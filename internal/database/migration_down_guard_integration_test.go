//go:build integration

package database

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// execDownInTx выполняет down-файл миграции в транзакции и откатывает её:
// схема и версия миграций в тестовой базе не меняются.
func execDownInTx(t *testing.T, pool *pgxpool.Pool, file string, prepare func(tx pgx.Tx)) error {
	t.Helper()
	ctx := context.Background()
	sqlText, err := os.ReadFile(filepath.Join(partnerMigrationsDir(t), file))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	prepare(tx)
	_, err = tx.Exec(ctx, string(sqlText))
	return err
}

// Откат журнала реферальных начислений с данными отказывается, а не стирает их.
func TestReferralLedgerDown_RefusesWithData(t *testing.T) {
	pool := partnerTestPool(t)
	err := execDownInTx(t, pool, "000043_referral_bonus_ledger.down.sql", func(tx pgx.Tx) {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO referral_bonus_ledger (referrer_telegram_id, referee_telegram_id, recipient_telegram_id, days, kind)
			VALUES (1, 2, 1, 7, 'manual')`); err != nil {
			t.Fatalf("insert ledger row: %v", err)
		}
	})
	if err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("down with ledger rows: err = %v, want refusal", err)
	}
}

// Без данных откаты 43 и 44 проходят как раньше.
func TestFinancialDowns_PassWhenEmpty(t *testing.T) {
	pool := partnerTestPool(t)
	empty := func(tables ...string) func(pgx.Tx) {
		return func(tx pgx.Tx) {
			for _, tbl := range tables {
				if _, err := tx.Exec(context.Background(), `DELETE FROM `+tbl); err != nil {
					t.Fatalf("clear %s: %v", tbl, err)
				}
			}
		}
	}
	if err := execDownInTx(t, pool, "000043_referral_bonus_ledger.down.sql", empty("referral_bonus_ledger")); err != nil {
		t.Fatalf("000043 down on empty ledger: %v", err)
	}
	if err := execDownInTx(t, pool, "000044_partner_program.down.sql", empty("partner_payout", "partner_earning")); err != nil {
		t.Fatalf("000044 down on empty partner tables: %v", err)
	}
}
