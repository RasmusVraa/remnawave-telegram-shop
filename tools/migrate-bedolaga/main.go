// Command migrate-bedolaga imports users/tariffs from a Bedolaga Postgres
// into this shop. Prefer the interactive wrapper:
//
//	./scripts/meows-bedolaga-migrate.sh
//
// Direct usage (from repo root):
//
//	go run ./tools/migrate-bedolaga -config migrate.yaml -dry-run -step all
//	go run ./tools/migrate-bedolaga -config migrate.yaml -apply -step tariffs
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

func main() {
	configPath := flag.String("config", "migrate.yaml", "path to migrate.yaml")
	step := flag.String("step", "all", "tariffs|customers|balance|referrals|history|all")
	dryRun := flag.Bool("dry-run", true, "plan only (default true)")
	apply := flag.Bool("apply", false, "write changes (overrides -dry-run)")
	writeExample := flag.Bool("write-example-config", false, "write migrate.yaml.example to stdout or -config path")
	loadBackupPath := flag.String("load-backup", "", "развернуть бекап из админки Bedolaga (.tar.gz / database.json) во временный Postgres и выйти")
	loadDSN := flag.String("load-dsn", "", "DSN временной базы для -load-backup (таблицы пересоздаются!)")
	flag.Parse()

	if *loadBackupPath != "" {
		if *loadDSN == "" {
			fmt.Fprintln(os.Stderr, "migrate-bedolaga: для -load-backup нужен -load-dsn")
			os.Exit(1)
		}
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if err := loadBackup(ctx, *loadBackupPath, *loadDSN); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-bedolaga: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Бекап развёрнут во временную базу.")
		return
	}

	if *writeExample {
		path := *configPath
		if path == "" || path == "migrate.yaml" {
			path = "migrate.yaml.example"
		}
		if err := os.WriteFile(path, []byte(DefaultConfigTemplate()), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-bedolaga: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", path)
		return
	}

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate-bedolaga: %v\n", err)
		os.Exit(1)
	}

	doDry := *dryRun
	if *apply {
		doDry = false
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	slog.Info("migrate-bedolaga starting",
		"config", *configPath,
		"step", *step,
		"dry_run", doDry,
		"report_dir", cfg.Reporting.Dir,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	m, err := newMigrator(cfg, doDry, strings.TrimSpace(*step))
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate-bedolaga: %v\n", err)
		os.Exit(1)
	}
	if err := m.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "migrate-bedolaga: %v\n", err)
		os.Exit(1)
	}
	slog.Info("done", "report_dir", cfg.Reporting.Dir)
	fmt.Printf("Reports: %s\n", cfg.Reporting.Dir)
}
