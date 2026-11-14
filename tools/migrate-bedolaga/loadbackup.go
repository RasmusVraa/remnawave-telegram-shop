package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// backupTables — таблицы Bedolaga, которые читает мигратор. Остальные (аудит, кнопки,
// рассылки и т.д. — десятки тысяч строк) в рабочую базу не загружаем.
var backupTables = []string{"users", "subscriptions", "tariffs", "transactions", "referral_earnings"}

// loadBackup разворачивает «бекап из админки Bedolaga» (tar.gz с database.json или сам
// database.json, формат orm-1.0) в Postgres по dsn. Типы колонок в файле не хранятся —
// выводим из значений. Таблицы создаются заново (DROP IF EXISTS), так что dsn должен
// указывать на временную базу, а не на рабочую.
func loadBackup(ctx context.Context, file, dsn string) (err error) {
	raw, closeFn, err := openBackupJSON(file)
	if err != nil {
		return err
	}
	defer closeFn()

	dec := json.NewDecoder(raw)
	dec.UseNumber()
	var doc struct {
		Metadata map[string]any              `json:"metadata"`
		Data     map[string][]map[string]any `json:"data"`
	}
	if err := dec.Decode(&doc); err != nil {
		return fmt.Errorf("разбор database.json: %w", err)
	}
	if len(doc.Data) == 0 {
		return fmt.Errorf("в файле нет секции data — это не бекап Bedolaga")
	}
	if _, ok := doc.Data["users"]; !ok {
		return fmt.Errorf("в бекапе нет таблицы users — это не бекап Bedolaga")
	}
	slog.Info("backup opened", "format", doc.Metadata["version"], "created", doc.Metadata["timestamp"], "tables", len(doc.Data))

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("подключение к временной базе: %w", err)
	}
	// Ошибка закрытия значит, что сервер мог не дописать последние данные: если загрузка
	// иначе прошла, о ней надо сказать, а не вернуть «успех».
	defer func() {
		if cerr := conn.Close(ctx); cerr != nil && err == nil {
			err = fmt.Errorf("закрытие подключения к временной базе: %w", cerr)
		}
	}()

	for _, table := range backupTables {
		rows, ok := doc.Data[table]
		if !ok {
			slog.Warn("table missing in backup, skipped", "table", table)
			continue
		}
		if err := loadBackupTable(ctx, conn, table, rows); err != nil {
			return fmt.Errorf("таблица %s: %w", table, err)
		}
		slog.Info("table loaded", "table", table, "rows", len(rows))
	}
	return nil
}

func openBackupJSON(file string) (io.Reader, func(), error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}
	lower := strings.ToLower(file)
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		gz, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return nil, nil, fmt.Errorf("%s: не gzip: %w", file, err)
		}
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				f.Close()
				return nil, nil, err
			}
			if path.Base(h.Name) == "database.json" {
				return tr, func() { f.Close() }, nil
			}
		}
		f.Close()
		return nil, nil, fmt.Errorf("в архиве %s нет database.json (бекап должен быть из админки Bedolaga)", file)
	case strings.HasSuffix(lower, ".json"):
		return f, func() { f.Close() }, nil
	default:
		f.Close()
		return nil, nil, fmt.Errorf("%s: ожидается .tar.gz или database.json", file)
	}
}

type colKind int

const (
	kindNull colKind = iota
	kindBool
	kindInt
	kindFloat
	kindTime
	kindJSON
	kindText
)

func (k colKind) sql() string {
	switch k {
	case kindBool:
		return "boolean"
	case kindInt:
		return "bigint"
	case kindFloat:
		return "double precision"
	case kindTime:
		return "timestamptz"
	case kindJSON:
		return "jsonb"
	default:
		return "text"
	}
}

func classify(v any) colKind {
	switch x := v.(type) {
	case nil:
		return kindNull
	case bool:
		return kindBool
	case json.Number:
		if strings.ContainsAny(x.String(), ".eE") {
			return kindFloat
		}
		return kindInt
	case string:
		if _, err := time.Parse(time.RFC3339Nano, x); err == nil && len(x) >= 19 {
			return kindTime
		}
		return kindText
	default:
		return kindJSON
	}
}

func mergeKind(a, b colKind) colKind {
	switch {
	case a == kindNull:
		return b
	case b == kindNull || a == b:
		return a
	case (a == kindInt && b == kindFloat) || (a == kindFloat && b == kindInt):
		return kindFloat
	default:
		return kindText
	}
}

func convertValue(v any, k colKind) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch k {
	case kindBool:
		return v.(bool), nil
	case kindInt:
		return v.(json.Number).Int64()
	case kindFloat:
		return v.(json.Number).Float64()
	case kindTime:
		return time.Parse(time.RFC3339Nano, v.(string))
	case kindJSON:
		b, err := json.Marshal(v)
		return string(b), err
	default:
		switch x := v.(type) {
		case string:
			return x, nil
		case json.Number:
			return x.String(), nil
		case bool:
			if x {
				return "true", nil
			}
			return "false", nil
		default:
			b, err := json.Marshal(v)
			return string(b), err
		}
	}
}

func loadBackupTable(ctx context.Context, conn *pgx.Conn, table string, rows []map[string]any) error {
	kinds := map[string]colKind{}
	for _, r := range rows {
		for c, v := range r {
			kinds[c] = mergeKind(kinds[c], classify(v))
		}
	}
	cols := make([]string, 0, len(kinds))
	for c := range kinds {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	if len(cols) == 0 {
		cols = []string{"id"}
		kinds["id"] = kindInt
	}

	defs := make([]string, len(cols))
	for i, c := range cols {
		defs[i] = pgx.Identifier{c}.Sanitize() + " " + kinds[c].sql()
	}
	tbl := pgx.Identifier{table}.Sanitize()
	if _, err := conn.Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE"); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, "CREATE TABLE "+tbl+" ("+strings.Join(defs, ", ")+")"); err != nil {
		return err
	}

	data := make([][]any, 0, len(rows))
	for _, r := range rows {
		row := make([]any, len(cols))
		for i, c := range cols {
			v, err := convertValue(r[c], kinds[c])
			if err != nil {
				return fmt.Errorf("колонка %s: %w", c, err)
			}
			row[i] = v
		}
		data = append(data, row)
	}
	_, err := conn.CopyFrom(ctx, pgx.Identifier{table}, cols, pgx.CopyFromRows(data))
	return err
}
