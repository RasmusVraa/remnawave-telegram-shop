//go:build integration

package database

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Жизненный цикл записи: заведена → счётчики по ходу → итог; запоздавшее
// обновление хода итог не откатывает.
func TestBroadcastHistory_Lifecycle(t *testing.T) {
	pool := partnerTestPool(t)
	ctx := context.Background()
	repo := NewBroadcastRepository(pool)

	tariff := int64(7)
	id, err := repo.Create(ctx, BroadcastRecord{
		Source:      BroadcastSourceCabinet,
		Audiences:   []string{BroadcastAudienceActiveAll, BroadcastAudienceInactivePaid},
		TariffID:    &tariff,
		Text:        "<b>Перебои</b>",
		ParseMode:   "HTML",
		MediaKind:   "photo",
		MediaFileID: "AgAC",
		MediaName:   "nl.png",
		Buttons:     json.RawMessage(`{"connect":true,"links":["tariffs"]}`),
		Total:       242,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM broadcast WHERE id = $1`, id) })

	if err := repo.UpdateProgress(ctx, id, BroadcastCounters{Total: 240, Sent: 100, Blocked: 3, Failed: 1}); err != nil {
		t.Fatalf("progress: %v", err)
	}
	got, err := repo.Get(ctx, id)
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.Status != BroadcastStatusRunning || got.Sent != 100 || got.Total != 240 || got.FinishedAt != nil {
		t.Fatalf("running record = %+v", got)
	}
	if len(got.Audiences) != 2 || got.TariffID == nil || *got.TariffID != 7 || got.MediaName != "nl.png" {
		t.Fatalf("stored fields = %+v", got)
	}
	var buttons map[string]any
	if err := json.Unmarshal(got.Buttons, &buttons); err != nil || buttons["connect"] != true {
		t.Fatalf("buttons = %s (%v)", got.Buttons, err)
	}

	final := BroadcastCounters{Total: 240, Sent: 229, Blocked: 9, Failed: 2}
	if err := repo.Finish(ctx, id, BroadcastStatusDone, final, ""); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if err := repo.UpdateProgress(ctx, id, BroadcastCounters{Total: 240, Sent: 150}); err != nil {
		t.Fatalf("late progress: %v", err)
	}
	got, _ = repo.Get(ctx, id)
	if got.Status != BroadcastStatusDone || got.Sent != 229 || got.Blocked != 9 || got.Failed != 2 || got.FinishedAt == nil {
		t.Fatalf("finished record = %+v", got)
	}

	missing, err := repo.Get(ctx, -1)
	if err != nil || missing != nil {
		t.Fatalf("missing record = %v, %v; want nil, nil", missing, err)
	}
}

// После перезапуска процесса «идущих» рассылок не остаётся: отправлять их
// некому, и карточка хода в кабинете не должна висеть вечно.
func TestBroadcastHistory_MarkInterrupted(t *testing.T) {
	pool := partnerTestPool(t)
	ctx := context.Background()
	repo := NewBroadcastRepository(pool)

	running, err := repo.Create(ctx, BroadcastRecord{Source: BroadcastSourceTelegram, Text: "a"})
	if err != nil {
		t.Fatalf("create running: %v", err)
	}
	done, err := repo.Create(ctx, BroadcastRecord{Source: BroadcastSourceTelegram, Text: "b"})
	if err != nil {
		t.Fatalf("create done: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM broadcast WHERE id = ANY($1)`, []int64{running, done})
	})
	if err := repo.Finish(ctx, done, BroadcastStatusDone, BroadcastCounters{Total: 1, Sent: 1}, ""); err != nil {
		t.Fatalf("finish: %v", err)
	}

	if _, err := repo.MarkInterrupted(ctx); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}
	r, _ := repo.Get(ctx, running)
	d, _ := repo.Get(ctx, done)
	if r.Status != BroadcastStatusInterrupted || r.FinishedAt == nil {
		t.Fatalf("running after restart = %+v", r)
	}
	if d.Status != BroadcastStatusDone {
		t.Fatalf("finished one changed: %+v", d)
	}

	items, total, err := repo.List(ctx, 50, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total < 2 || len(items) < 2 {
		t.Fatalf("list: total=%d len=%d", total, len(items))
	}
	for i := 1; i < len(items); i++ {
		if items[i].StartedAt.After(items[i-1].StartedAt) {
			t.Fatalf("list not newest first at %d", i)
		}
	}
}

// Откат истории с данными отказывается, без данных проходит.
func TestBroadcastHistoryDown(t *testing.T) {
	pool := partnerTestPool(t)
	err := execDownInTx(t, pool, "000056_broadcast_history.down.sql", func(tx pgx.Tx) {
		if _, err := tx.Exec(context.Background(),
			`INSERT INTO broadcast (source, status) VALUES ('cabinet', 'done')`); err != nil {
			t.Fatalf("insert: %v", err)
		}
	})
	if err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("down with rows: err = %v, want refusal", err)
	}
	err = execDownInTx(t, pool, "000056_broadcast_history.down.sql", func(tx pgx.Tx) {
		ctx := context.Background()
		// Тестовая база уже на последней миграции, а откат идёт по порядку:
		// сначала 59 — у уведомлений кабинета внешний ключ на broadcast.
		if _, err := tx.Exec(ctx, `DELETE FROM cabinet_notification`); err != nil {
			t.Fatalf("clear notifications: %v", err)
		}
		later, err := os.ReadFile(filepath.Join(partnerMigrationsDir(t), "000059_cabinet_notifications.down.sql"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, string(later)); err != nil {
			t.Fatalf("down 59: %v", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM broadcast`); err != nil {
			t.Fatalf("clear: %v", err)
		}
	})
	if err != nil {
		t.Fatalf("down on empty table: %v", err)
	}
}
