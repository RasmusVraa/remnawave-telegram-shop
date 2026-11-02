//go:build integration

package database

import (
	"context"
	"testing"
)

// Важное у человека одно — последнее. Прежние важные в ленте становятся
// обычными, а скрытое крестиком свежее важное старое важным не возвращает.
func TestCabinetNotifications_SingleImportant(t *testing.T) {
	pool := partnerTestPool(t)
	ctx := context.Background()
	repo := NewCabinetNotificationRepository(pool)
	c := makeCustomer(t, pool)
	other := makeCustomer(t, pool)

	create := func(customerID int64, title string, important bool) int64 {
		t.Helper()
		id, created, err := repo.CreatePersonal(ctx, customerID, CabinetNotification{Kind: CabinetNotificationBroadcast, Title: title, Body: title, Important: important})
		if err != nil || !created {
			t.Fatalf("create %q: %v (created=%v)", title, err, created)
		}
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM cabinet_notification WHERE id = $1`, id) })
		return id
	}
	create(c.ID, "first", true)
	create(c.ID, "plain", false)
	second := create(c.ID, "second", true)
	last := create(c.ID, "last", true)
	otherOwn := create(other.ID, "other", true)

	items, total, err := repo.List(ctx, c.ID, 10, 0)
	if err != nil || total != 4 {
		t.Fatalf("list: total=%d err=%v", total, err)
	}
	if items[0].ID != last || !items[0].Important {
		t.Fatalf("pinned = %+v, want last important", items[0])
	}
	for _, it := range items[1:] {
		if it.Important {
			t.Fatalf("%q stays important next to the latest one", it.Title)
		}
	}
	u, err := repo.Unread(ctx, c.ID)
	if err != nil || u.Total != 4 || u.Important != 1 {
		t.Fatalf("unread = %+v (%v), want 4 total, 1 important", u, err)
	}
	if got, err := repo.LatestUnreadImportant(ctx, c.ID); err != nil || got == nil || got.ID != last {
		t.Fatalf("banner = %+v (%v), want last", got, err)
	}

	// Чужое важное своё у другого человека и не страдает от наших.
	if got, err := repo.LatestUnreadImportant(ctx, other.ID); err != nil || got == nil || got.ID != otherOwn {
		t.Fatalf("other banner = %+v (%v)", got, err)
	}

	// Скрыл свежее — плашки нет, прежнее важным не становится.
	if err := repo.Hide(ctx, c.ID, []int64{last}); err != nil {
		t.Fatalf("hide: %v", err)
	}
	if got, err := repo.LatestUnreadImportant(ctx, c.ID); err != nil || got != nil {
		t.Fatalf("banner after hide = %+v (%v), want none", got, err)
	}
	items, _, _ = repo.List(ctx, c.ID, 10, 0)
	for _, it := range items {
		if it.Important {
			t.Fatalf("%q became important after the latest was hidden", it.Title)
		}
	}

	// Админ снял важность с последнего — важным снова становится предыдущее.
	if _, err := pool.Exec(ctx, `UPDATE cabinet_notification SET important = false WHERE id = $1`, last); err != nil {
		t.Fatalf("demote: %v", err)
	}
	if got, err := repo.LatestUnreadImportant(ctx, c.ID); err != nil || got == nil || got.ID != second {
		t.Fatalf("banner after demote = %+v (%v), want second", got, err)
	}
}

// Важная рассылка в сервисе одна: новая снимает флаг с прежней, какой бы ни
// была аудитория, — это видно и в истории. Тестовая рассылка флаг не трогает,
// «Сделать важным» в истории снимает его с текущей важной.
func TestCabinetNotifications_ImportantBroadcastIsGlobal(t *testing.T) {
	pool := partnerTestPool(t)
	ctx := context.Background()
	repo := NewCabinetNotificationRepository(pool)
	bcRepo := NewBroadcastRepository(pool)
	admin := makeCustomer(t, pool)

	publish := func(text string, audience string) int64 {
		t.Helper()
		bid, err := bcRepo.Create(ctx, BroadcastRecord{Source: BroadcastSourceCabinet, Audiences: []string{audience}, Text: text, Total: 1})
		if err != nil {
			t.Fatalf("broadcast: %v", err)
		}
		nid, _, err := repo.CreateForAudience(ctx, CabinetNotification{Kind: CabinetNotificationBroadcast, BroadcastID: &bid, Important: true, Body: text},
			[]string{audience}, nil, admin.TelegramID)
		if err != nil || nid == 0 {
			t.Fatalf("publish %q: id=%d err=%v", text, nid, err)
		}
		t.Cleanup(func() {
			bg := context.Background()
			_, _ = pool.Exec(bg, `DELETE FROM cabinet_notification WHERE id = $1`, nid)
			_, _ = pool.Exec(bg, `DELETE FROM broadcast WHERE id = $1`, bid)
		})
		return bid
	}
	important := func(ids ...int64) map[int64]bool {
		t.Helper()
		stats, err := repo.BroadcastStats(ctx, ids)
		if err != nil {
			t.Fatalf("stats: %v", err)
		}
		out := map[int64]bool{}
		for _, id := range ids {
			out[id] = stats[id].Important
		}
		return out
	}

	forActive := publish("for active", BroadcastAudienceActiveAll)
	forExpired := publish("for expired", BroadcastAudienceInactivePaid)
	if got := important(forActive, forExpired); got[forActive] || !got[forExpired] {
		t.Fatalf("after second important: %v, want only the newer one", got)
	}

	test := publish("test", BroadcastAudienceTestBroadcast)
	if got := important(forExpired, test); !got[forExpired] {
		t.Fatalf("test broadcast demoted the real important one: %v", got)
	}

	if found, err := repo.SetImportantByBroadcast(ctx, forActive, true); err != nil || !found {
		t.Fatalf("make important: found=%v err=%v", found, err)
	}
	if got := important(forActive, forExpired); !got[forActive] || got[forExpired] {
		t.Fatalf("after make important: %v, want only the re-marked one", got)
	}
}
