package yookasa

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/google/uuid"

	"remnawave-tg-shop-bot/internal/database"
)

type Payment struct {
	ID            uuid.UUID         `json:"id,omitempty"`
	Status        string            `json:"status,omitempty"`
	Paid          bool              `json:"paid,omitempty"`
	Amount        Amount            `json:"amount,omitempty"`
	Confirmation  ConfirmationType  `json:"confirmation,omitempty"`
	CreatedAt     time.Time         `json:"created_at,omitempty"`
	ExpiresAt     time.Time         `json:"expires_at,omitempty"`
	Description   string            `json:"description,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Recipient     RecipientType     `json:"recipient,omitempty"`
	PaymentMethod PaymentType       `json:"payment_method,omitempty"`
	Refundable    bool              `json:"refundable,omitempty"`
	Test          bool              `json:"test,omitempty"`
	RedirectURL   string            `json:"redirect_url,omitempty"`
}

func (p *Payment) IsCancelled() bool {
	return p.Status == "canceled"
}

// MatchesPurchase проверяет, что платёж выставлен именно этой покупке: тип
// счёта, id платежа и сумма в рублях. Покупка находится по purchaseId из
// metadata, поэтому без сверки чужой (или более дешёвый) платёж мог бы
// зачесться не той покупке.
func (p *Payment) MatchesPurchase(pu *database.Purchase) error {
	if pu.InvoiceType != database.InvoiceTypeYookasa {
		return fmt.Errorf("purchase invoice type is %q", pu.InvoiceType)
	}
	if pu.YookasaID == nil || *pu.YookasaID != p.ID {
		return fmt.Errorf("purchase yookasa_id %v differs from payment id %s", pu.YookasaID, p.ID)
	}
	if p.Amount.Currency != "RUB" {
		return fmt.Errorf("payment currency %q", p.Amount.Currency)
	}
	paid, err := strconv.ParseFloat(p.Amount.Value, 64)
	if err != nil {
		return fmt.Errorf("payment amount %q: %w", p.Amount.Value, err)
	}
	// Счёт создаётся на int(amount) — см. PaymentService.createYookasaInvoice.
	if math.Abs(paid-math.Trunc(pu.Amount)) >= 0.01 {
		return fmt.Errorf("payment amount %s differs from purchase amount %.2f", p.Amount.Value, pu.Amount)
	}
	return nil
}

type PaymentRequest struct {
	Amount            Amount             `json:"amount"`
	Confirmation      ConfirmationType   `json:"confirmation"`
	Capture           bool               `json:"capture"`
	Description       string             `json:"description,omitempty"`
	PaymentMethodData *PaymentMethodData `json:"payment_method_data,omitempty"`
	SavePaymentMethod bool               `json:"save_payment_method"`
	PaymentMethodID   *uuid.UUID         `json:"payment_method_id"`
	Receipt           *Receipt           `json:"receipt,omitempty"`
	Metadata          map[string]any     `json:"metadata,omitempty"`
}

func NewPaymentRequest(
	amount Amount,
	urlRedirect,
	description string,
	receipt *Receipt,
	metadata map[string]any) PaymentRequest {
	return PaymentRequest{
		Amount:   amount,
		Receipt:  receipt,
		Metadata: metadata,
		Confirmation: ConfirmationType{
			Type:      "redirect",
			ReturnURL: urlRedirect,
		},
		PaymentMethodData: nil,
		Capture:           true,
		Description:       description,
	}
}

type Receipt struct {
	Items    []Item    `json:"items"`
	Customer *Customer `json:"customer,omitempty"`
}

type Customer struct {
	Email string `json:"email"`
}

type Item struct {
	Description    string `json:"description"`
	Amount         Amount `json:"amount"`
	VatCode        int    `json:"vat_code"`
	Quantity       string `json:"quantity"`
	PaymentSubject string `json:"payment_subject,omitempty"`
	PaymentMode    string `json:"payment_mode,omitempty"`
}

type Amount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type PaymentMethodData struct {
	Type string `json:"type"`
}

type ConfirmationType struct {
	ReturnURL       string `json:"return_url,omitempty"`
	Type            string `json:"type,omitempty"`
	ConfirmationURL string `json:"confirmation_url,omitempty"`
}

type RecipientType struct {
	AccountID string `json:"account_id,omitempty"`
	GatewayID string `json:"gateway_id,omitempty"`
}

type PaymentType struct {
	Type  string    `json:"type,omitempty"`
	ID    uuid.UUID `json:"id,omitempty"`
	Saved bool      `json:"saved,omitempty"`
}
