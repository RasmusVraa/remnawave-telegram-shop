package platega

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

type PurchaseProcessor interface {
	ProcessPurchaseById(ctx context.Context, purchaseId int64) error
}

// TransactionGetter — перезапрос транзакции в API Platega.
type TransactionGetter interface {
	GetTransaction(ctx context.Context, id string) (*TransactionStatusResponse, error)
}

type WebhookHandler struct {
	purchaseRepo       *database.PurchaseRepository
	processor          PurchaseProcessor
	client             TransactionGetter
	expectedMerchantID string
	expectedSecret     string
}

func NewWebhookHandler(
	purchaseRepo *database.PurchaseRepository,
	processor PurchaseProcessor,
	client TransactionGetter,
	merchantID, secret string,
) *WebhookHandler {
	return &WebhookHandler{
		purchaseRepo:       purchaseRepo,
		processor:          processor,
		client:             client,
		expectedMerchantID: merchantID,
		expectedSecret:     secret,
	}
}

// constantTimeEqual сравнивает секреты за время, не зависящее от того, на каком
// символе они расходятся: обычное != позволяло подбирать секрет по задержке.
func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// CheckTransaction сверяет транзакцию из API Platega с покупкой: тип счёта, id
// транзакции и сумму. Сумма проверяется только снизу: оплачено не меньше счёта
// (с запасом в рубль на округление), — комиссия провайдера её не уменьшает.
func CheckTransaction(tx *TransactionStatusResponse, pu *database.Purchase) error {
	isPlatega := false
	for _, t := range database.PlategaInvoiceTypes() {
		if pu.InvoiceType == t {
			isPlatega = true
			break
		}
	}
	if !isPlatega {
		return fmt.Errorf("purchase invoice type is %q", pu.InvoiceType)
	}
	if pu.PlategaID == nil || strings.TrimSpace(*pu.PlategaID) != strings.TrimSpace(tx.Id) {
		return fmt.Errorf("purchase platega_id %v differs from transaction %q", pu.PlategaID, tx.Id)
	}
	d := tx.PaymentDetails
	if (d.Currency == "" || strings.EqualFold(d.Currency, "RUB")) && d.Amount > 0 && d.Amount+1 < pu.Amount {
		return fmt.Errorf("transaction amount %.2f is less than purchase amount %.2f", d.Amount, pu.Amount)
	}
	return nil
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if h.expectedMerchantID == "" || h.expectedSecret == "" {
		http.Error(w, "platega not configured", http.StatusServiceUnavailable)
		return
	}

	merchantID := r.Header.Get("X-MerchantId")
	secretHeader := r.Header.Get("X-Secret")
	// Оба сравнения выполняются всегда: короткое замыкание выдавало бы по
	// времени ответа, совпал ли merchant id.
	merchantOK := constantTimeEqual(merchantID, h.expectedMerchantID)
	secretOK := constantTimeEqual(secretHeader, h.expectedSecret)
	if !merchantOK || !secretOK {
		slog.Warn("platega webhook: invalid credentials", "received_merchant_id", merchantID)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Уведомление Platega — несколько килобайт. Без лимита любой мог
	// прислать гигабайтное тело и держать его в памяти целиком.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		slog.Error("platega webhook: read body error", "error", err)
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	var payload CallbackPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		slog.Error("platega webhook: unmarshal error", "error", err, "payload", string(body))
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	slog.Info("platega webhook received",
		"transaction_id", payload.Id,
		"status", payload.Status,
		"amount", payload.Amount,
		"payment_method", payload.PaymentMethod,
	)

	purchaseID, ok := parsePurchaseID(payload.Payload)
	if !ok {
		slog.Warn("platega webhook: missing purchaseId in payload", "transaction_id", payload.Id, "payload", payload.Payload)
		w.WriteHeader(http.StatusOK)
		return
	}

	purchase, err := h.purchaseRepo.FindById(ctx, purchaseID)
	if err != nil {
		slog.Error("platega webhook: find purchase failed", "purchase_id", purchaseID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if purchase == nil {
		slog.Warn("platega webhook: purchase not found", "purchase_id", purchaseID)
		w.WriteHeader(http.StatusOK)
		return
	}

	if purchase.Status == database.PurchaseStatusPaid || purchase.Status == database.PurchaseStatusCancel {
		if purchase.Status == database.PurchaseStatusPaid && payload.Status == StatusChargebacked {
			// Подписка уже выдана, а деньги вернули через банк. Автоматически
			// ничего не отзываем — решение за админом, но молча это терять нельзя.
			slog.Error("platega webhook: CHARGEBACK on paid purchase — check manually",
				"purchase_id", purchaseID, "transaction_id", payload.Id, "customer_id", purchase.CustomerID)
		} else {
			slog.Info("platega webhook: purchase already finalized", "purchase_id", purchaseID, "status", purchase.Status)
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	// Статус из тела не используем: перезапрашиваем транзакцию этой покупки в
	// API Platega и сверяем её с покупкой. Тело вебхука — только повод проверить.
	if purchase.PlategaID == nil || strings.TrimSpace(*purchase.PlategaID) == "" {
		slog.Warn("platega webhook: purchase has no platega_id", "purchase_id", purchaseID)
		w.WriteHeader(http.StatusOK)
		return
	}
	tx, err := h.client.GetTransaction(ctx, strings.TrimSpace(*purchase.PlategaID))
	if err != nil {
		slog.Error("platega webhook: get transaction failed", "purchase_id", purchaseID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := CheckTransaction(tx, purchase); err != nil {
		slog.Error("platega webhook: transaction does not match purchase", "purchase_id", purchaseID, "error", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	if tx.Id != payload.Id {
		slog.Warn("platega webhook: body transaction id differs from purchase", "purchase_id", purchaseID, "body_id", payload.Id, "transaction_id", tx.Id)
	}

	switch tx.Status {
	case StatusConfirmed:
		if err := h.processor.ProcessPurchaseById(ctx, purchaseID); err != nil {
			slog.Error("platega webhook: process purchase failed", "purchase_id", purchaseID, "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	case StatusCanceled, StatusChargebacked:
		if _, err := h.purchaseRepo.CancelIfOpen(ctx, purchaseID); err != nil {
			slog.Error("platega webhook: cancel purchase failed", "purchase_id", purchaseID, "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	default:
		slog.Debug("platega webhook: status ignored", "purchase_id", purchaseID, "status", tx.Status)
	}

	w.WriteHeader(http.StatusOK)
}

func parsePurchaseID(payload string) (int64, bool) {
	for _, part := range strings.Split(payload, "&") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		if kv[0] == "purchaseId" {
			id, err := strconv.ParseInt(kv[1], 10, 64)
			if err != nil {
				return 0, false
			}
			return id, true
		}
	}
	return 0, false
}
