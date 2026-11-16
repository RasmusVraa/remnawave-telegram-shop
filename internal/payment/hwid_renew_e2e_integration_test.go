//go:build integration

package payment

// Продление подписки без тарифа в счёте (Tribute, бот без активных тарифов —
// ветка classic в applyExtraAfterSubscription) у клиента с действующими доп. HWID.
// Запуск — как у yookassa_webhook_e2e_integration_test.go.

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"remnawave-tg-shop-bot/internal/yookasa"
)

func (p *fakePanel) setDeviceLimit(telegramID int64, limit int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, u := range p.users {
		if u.TelegramID != nil && *u.TelegramID == telegramID {
			l := limit
			u.HwidDeviceLimit = &l
		}
	}
}

// hwidRenewSetup: база 2 (HWID_FALLBACK_DEVICE_LIMIT по умолчанию) + 2 доп. места до конца подписки.
func hwidRenewSetup(t *testing.T, e *e2eEnv) (int64, int64, time.Time) {
	t.Helper()
	tgID, _ := e2eTelegramIDs()
	customerID := e.customer(t, tgID)
	expire := time.Now().UTC().Add(5 * 24 * time.Hour).Truncate(time.Second)
	if _, err := e.pool.Exec(context.Background(),
		`UPDATE customer SET expire_at = $2, extra_hwid = 2, extra_hwid_expires_at = $2 WHERE id = $1`, customerID, expire); err != nil {
		t.Fatalf("seed extra hwid: %v", err)
	}
	e.panel.addUser(tgID, expire)
	e.panel.setDeviceLimit(tgID, 4)
	return tgID, customerID, expire
}

// hwidRenewInvoice — счёт за месяц без tariff_id с extraHwid доп. мест.
func hwidRenewInvoice(t *testing.T, e *e2eEnv, customerID int64, extraHwid int) uuid.UUID {
	t.Helper()
	paymentID := uuid.New()
	var purchaseID int64
	if err := e.pool.QueryRow(context.Background(), `
		INSERT INTO purchase (amount, customer_id, month, extra_hwid, currency, status, invoice_type, yookasa_id)
		VALUES (199, $1, 1, $2, 'RUB', 'pending', 'yookasa', $3) RETURNING id`, customerID, extraHwid, paymentID).Scan(&purchaseID); err != nil {
		t.Fatalf("insert purchase: %v", err)
	}
	e.kassa.set(yookasa.Payment{
		ID:       paymentID,
		Status:   "succeeded",
		Paid:     true,
		Amount:   yookasa.Amount{Value: "199.00", Currency: "RUB"},
		Metadata: map[string]string{"purchaseId": strconv.FormatInt(purchaseID, 10)},
	})
	return paymentID
}

func hwidRenewState(t *testing.T, e *e2eEnv, customerID int64) (int, *time.Time) {
	t.Helper()
	var extra int
	var exp *time.Time
	if err := e.pool.QueryRow(context.Background(),
		`SELECT extra_hwid, extra_hwid_expires_at FROM customer WHERE id = $1`, customerID).Scan(&extra, &exp); err != nil {
		t.Fatalf("customer extra hwid: %v", err)
	}
	return extra, exp
}

// Клиент нажал «Не продлевать» доп. устройства: места не оплачены — лимит в панели
// возвращается к базе, а не остаётся повышенным без срока.
func TestYookassaWebhookE2E_RenewWithoutExtraHwidDropsSlots(t *testing.T) {
	e := newE2EEnv(t)
	tgID, customerID, _ := hwidRenewSetup(t, e)

	if code := e.notify(t, hwidRenewInvoice(t, e, customerID, 0)); code != http.StatusOK {
		t.Fatalf("webhook: status %d, want 200", code)
	}
	extra, exp := hwidRenewState(t, e, customerID)
	if extra != 0 || exp != nil {
		t.Fatalf("db extra_hwid=%d expires_at=%v, want 0 and nil", extra, exp)
	}
	if l := e.panel.user(tgID).HwidDeviceLimit; l == nil || *l != 2 {
		t.Fatalf("panel device limit %v, want 2 (base without unpaid extra)", deref(l))
	}
}

// Продление вместе с доп. устройствами: лимит base+extra, срок мест — новый конец подписки.
func TestYookassaWebhookE2E_RenewWithExtraHwidKeepsSlots(t *testing.T) {
	e := newE2EEnv(t)
	tgID, customerID, expire := hwidRenewSetup(t, e)

	if code := e.notify(t, hwidRenewInvoice(t, e, customerID, 2)); code != http.StatusOK {
		t.Fatalf("webhook: status %d, want 200", code)
	}
	wantExpire := expire.AddDate(0, 0, e2eDaysInMonth)
	extra, exp := hwidRenewState(t, e, customerID)
	if extra != 2 || exp == nil || !sameDay(*exp, wantExpire) {
		t.Fatalf("db extra_hwid=%d expires_at=%v, want 2 and %v", extra, exp, wantExpire)
	}
	if l := e.panel.user(tgID).HwidDeviceLimit; l == nil || *l != 4 {
		t.Fatalf("panel device limit %v, want 4", deref(l))
	}
}

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
