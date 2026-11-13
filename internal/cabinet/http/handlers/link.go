package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode"

	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	"remnawave-tg-shop-bot/internal/cabinet/linking"
	cabmetrics "remnawave-tg-shop-bot/internal/cabinet/metrics"
)

// LinkHandler — эндпоинты /cabinet/api/link/*.
//
// POST /link/telegram/start    → nonce для Telegram Login Widget.
// POST /link/telegram/confirm  → валидация payload, сохранение claim.
// POST /link/merge/preview     → dry-run merge, preview изменений.
// POST /link/merge/confirm     → реальный merge (Idempotency-Key).
type LinkHandler struct {
	svc *linking.MergeService
}

// NewLink — конструктор.
func NewLink(svc *linking.MergeService) *LinkHandler {
	return &LinkHandler{svc: svc}
}

// ============================================================================
// POST /link/telegram/start
// ============================================================================

// TelegramStart генерирует nonce (TTL 10 мин) для использования в
// Telegram Login Widget в качестве параметра state/nonce.
//
// Response 200: { "nonce": "<hex>" }
func (h *LinkHandler) TelegramStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	nonce, err := h.svc.Start(r.Context(), claims.AccountID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"nonce": nonce})
}

// ============================================================================
// POST /link/telegram/confirm
// ============================================================================

// telegramConfirmRequest — тело POST /link/telegram/confirm.
type telegramConfirmRequest struct {
	// Общие.
	Source string `json:"source"` // "widget" | "miniapp"
	Nonce  string `json:"nonce"`

	// Widget-поля (заполняются если source=widget).
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	PhotoURL  string `json:"photo_url"`
	AuthDate  int64  `json:"auth_date"`
	Hash      string `json:"hash"`

	// MiniApp-поле (заполняется если source=miniapp).
	InitData string `json:"init_data"`
}

// TelegramConfirm валидирует Telegram payload + nonce, сохраняет claim для merge.
//
// Response 200: { "telegram_id": 123, "customer_tg_id": 456, "has_merge_candidate": true }
func (h *LinkHandler) TelegramConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req telegramConfirmRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Nonce == "" {
		http.Error(w, "nonce is required", http.StatusBadRequest)
		return
	}
	if req.Source != "widget" && req.Source != "miniapp" {
		http.Error(w, `source must be "widget" or "miniapp"`, http.StatusBadRequest)
		return
	}

	in := linking.ConfirmInput{
		Source:    req.Source,
		Nonce:     req.Nonce,
		UserAgent: r.UserAgent(),
		IP:        middleware.ClientIP(r),
		ID:        req.ID,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Username:  req.Username,
		PhotoURL:  req.PhotoURL,
		AuthDate:  req.AuthDate,
		Hash:      req.Hash,
		InitData:  req.InitData,
	}

	claim, err := h.svc.Confirm(r.Context(), claims.AccountID, in)
	if err != nil {
		switch {
		case errors.Is(err, linking.ErrNonceInvalid):
			http.Error(w, "nonce invalid or expired; call /link/telegram/start again", http.StatusUnprocessableEntity)
		case errors.Is(err, linking.ErrTelegramDisabled):
			http.Error(w, "telegram not configured", http.StatusNotImplemented)
		case errors.Is(err, linking.ErrTelegramAlreadyLinked):
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"telegram_already_linked","message":"this telegram is already linked to another cabinet account"}`))
		default:
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		}
		return
	}

	resp := map[string]any{
		"telegram_id":         claim.TelegramID,
		"telegram_username":   claim.TelegramUsername,
		"has_merge_candidate": claim.CustomerTgID != nil,
	}
	if claim.CustomerTgID != nil {
		resp["customer_tg_id"] = *claim.CustomerTgID
	}
	writeJSON(w, http.StatusOK, resp)
}

// ============================================================================
// POST /link/merge/preview
// ============================================================================

// MergePreview — dry-run merge. Транзакция откатывается, БД не меняется.
//
// Response 200: MergePreviewResponse
func (h *LinkHandler) MergePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	preview, err := h.svc.Preview(r.Context(), claims.AccountID)
	if err != nil {
		switch {
		case errors.Is(err, linking.ErrNoClaimFound):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"status":      "error",
				"reason_code": "merge_claim_missing",
				"message":     "no merge claim: confirm telegram or verify email merge first",
			})
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, mergePreviewToResponse(preview))
}

// ============================================================================
// POST /link/merge/confirm
// ============================================================================

// mergeConfirmRequest — тело POST /link/merge/confirm.
type mergeConfirmRequest struct {
	Force bool `json:"force"`
	// KeepSubscription — при двух подписках: "web" (текущий customer кабинета) или "tg" (найденный по Telegram).
	KeepSubscription string `json:"keep_subscription"`
}

// MergeConfirm — выполняет реальный merge.
// Заголовок Idempotency-Key обязателен.
//
// Response 200: { "result": "merged|linked|noop", "customer_id": 123, "purchases_moved": 2 }
// Response 202: { "result": "already_done", ... } — если ключ уже использован.
// Response 409: { "error": "dangerous_conflict", "preview": {...} } — если force нужен.
func (h *LinkHandler) MergeConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ikey := r.Header.Get("Idempotency-Key")
	if !validIdempotencyKey(ikey) {
		cabmetrics.RecordMerge("client_error")
		http.Error(w, "Idempotency-Key must be 16-64 chars [A-Za-z0-9._:-]", http.StatusBadRequest)
		return
	}

	var req mergeConfirmRequest
	// DisallowUnknownFields отключаем для force — тело может быть пустым.
	_ = decodeJSON(w, r, &req)

	result, err := h.svc.Merge(r.Context(), claims.AccountID, ikey, req.Force, req.KeepSubscription)

	if err != nil {
		switch {
		case errors.Is(err, linking.ErrSubscriptionChoiceRequired):
			cabmetrics.RecordMerge("client_error")
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"status":          "merge_blocked",
				"reason_code":     "double_active_subscription_conflict",
				"message":         "pass keep_subscription web or tg when both accounts have a subscription history",
				"requires_choice": true,
			})
			return
		case errors.Is(err, linking.ErrMergeAlreadyDone):
			cabmetrics.RecordMerge("already_done")
			// Идемпотентный повтор — возвращаем 202 с закешированным результатом.
			if result != nil {
				writeJSON(w, http.StatusAccepted, map[string]any{
					"status":      "already_done",
					"reason_code": "idempotency_key_reused",
					"result":      "already_done",
					"customer_id": result.CustomerID,
				})
			} else {
				writeJSON(w, http.StatusAccepted, map[string]string{
					"status":      "already_done",
					"reason_code": "idempotency_key_reused",
					"result":      "already_done",
				})
			}
			return
		case errors.Is(err, linking.ErrNoClaimFound):
			cabmetrics.RecordMerge("client_error")
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"status":      "error",
				"reason_code": "merge_claim_missing",
				"message":     "no merge claim: confirm telegram or verify email merge first",
			})
			return
		default:
			slog.Error("cabinet merge confirm failed",
				"account_id", claims.AccountID,
				"error", err,
				"keep_subscription", req.KeepSubscription,
				"force", req.Force,
			)
			cabmetrics.RecordMerge("server_error")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	switch result.Result {
	case "noop":
		cabmetrics.RecordMerge("noop")
	default:
		cabmetrics.RecordMerge("success")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":          result.Result,
		"reason_code":     "merge_committed",
		"result":          result.Result,
		"customer_id":     result.CustomerID,
		"purchases_moved": result.PurchasesMoved,
		"referrals_moved": result.ReferralsMoved,
	})
}

// ============================================================================
// Helpers
// ============================================================================

type mergeCustomerResp struct {
	ID              int64  `json:"id"`
	ExpireAt        any    `json:"expire_at"`
	LoyaltyXP       int64  `json:"loyalty_xp"`
	ExtraHwid       int    `json:"extra_hwid"`
	IsWebOnly       bool   `json:"is_web_only"`
	HasSubscription bool   `json:"has_subscription"`
	CurrentTariffID *int64 `json:"current_tariff_id,omitempty"`
}

type mergePreviewResp struct {
	CustomerWeb *mergeCustomerResp `json:"customer_web,omitempty"`
	CustomerTg  *mergeCustomerResp `json:"customer_tg,omitempty"`
	// UISwapSides — см. linking.MergePreview.UISwapSides (email-peer merge + Telegram).
	UISwapSides     bool   `json:"ui_swap_sides,omitempty"`
	MergedExpireAt  any    `json:"merged_expire_at"`
	MergedLoyaltyXP int64  `json:"merged_loyalty_xp"`
	MergedExtraHwid int    `json:"merged_extra_hwid"`
	PurchasesMoved  int    `json:"purchases_moved"`
	ReferralsMoved  int    `json:"referrals_moved"`
	IsNoop          bool   `json:"is_noop"`
	IsDangerous     bool   `json:"is_dangerous"`
	DangerReason    string `json:"danger_reason,omitempty"`
	// RequiresSubscriptionChoice — оба customer с expire_at; нужен keep_subscription в confirm.
	RequiresSubscriptionChoice bool `json:"requires_subscription_choice"`
	ClaimExpiresAt             any  `json:"claim_expires_at,omitempty"`
}

func mergePreviewToResponse(p *linking.MergePreview) mergePreviewResp {
	resp := mergePreviewResp{
		MergedLoyaltyXP:            p.MergedLoyaltyXP,
		MergedExtraHwid:            p.MergedExtraHwid,
		PurchasesMoved:             p.PurchasesMoved,
		ReferralsMoved:             p.ReferralsMoved,
		IsNoop:                     p.IsNoop,
		IsDangerous:                p.IsDangerous,
		DangerReason:               p.DangerReason,
		RequiresSubscriptionChoice: p.RequiresSubscriptionChoice,
		UISwapSides:                p.UISwapSides,
	}
	if p.ClaimExpiresAt != nil {
		resp.ClaimExpiresAt = p.ClaimExpiresAt.UTC()
	}
	if p.MergedExpireAt != nil {
		resp.MergedExpireAt = p.MergedExpireAt.UTC()
	}
	if p.CustomerWeb != nil {
		resp.CustomerWeb = snapshotToResp(p.CustomerWeb)
	}
	if p.CustomerTg != nil {
		resp.CustomerTg = snapshotToResp(p.CustomerTg)
	}
	return resp
}

func snapshotToResp(s *linking.CustomerSnapshot) *mergeCustomerResp {
	r := &mergeCustomerResp{
		ID:              s.ID,
		LoyaltyXP:       s.LoyaltyXP,
		ExtraHwid:       s.ExtraHwid,
		IsWebOnly:       s.IsWebOnly,
		CurrentTariffID: s.CurrentTariffID,
	}
	if s.ExpireAt != nil {
		r.ExpireAt = s.ExpireAt.UTC()
		r.HasSubscription = true
	}
	return r
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func validIdempotencyKey(v string) bool {
	v = strings.TrimSpace(v)
	if len(v) < 16 || len(v) > 64 {
		return false
	}
	for _, r := range v {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '.', '_', ':', '-':
			continue
		default:
			return false
		}
	}
	return true
}
