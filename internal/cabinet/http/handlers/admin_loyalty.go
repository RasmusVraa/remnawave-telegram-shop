package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/loyalty"
	"remnawave-tg-shop-bot/utils"
)

var recalcRunning int32

func extractLoyaltyTierID(path string) (int64, bool) {
	s := strings.TrimPrefix(path, "/cabinet/api/admin/loyalty/tiers/")
	s = strings.TrimRight(s, "/")
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

type AdminLoyaltyHandler struct {
	tiers     *database.LoyaltyTierRepository
	customers *database.CustomerRepository
	purchases *database.PurchaseRepository
}

func NewAdminLoyalty(
	tiers *database.LoyaltyTierRepository,
	customers *database.CustomerRepository,
	purchases *database.PurchaseRepository,
) *AdminLoyaltyHandler {
	return &AdminLoyaltyHandler{
		tiers:     tiers,
		customers: customers,
		purchases: purchases,
	}
}

type loyaltyTierDTO struct {
	ID              int64   `json:"id"`
	SortOrder       int     `json:"sort_order"`
	XpMin           int64   `json:"xp_min"`
	DiscountPercent int     `json:"discount_percent"`
	DisplayName     *string `json:"display_name"`
}

func tierToDTO(t *database.LoyaltyTier) loyaltyTierDTO {
	return loyaltyTierDTO{
		ID: t.ID, SortOrder: t.SortOrder, XpMin: t.XpMin,
		DiscountPercent: t.DiscountPercent, DisplayName: t.DisplayName,
	}
}

// ListTiers — GET /cabinet/api/admin/loyalty/tiers
func (h *AdminLoyaltyHandler) ListTiers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tiers, err := h.tiers.ListAllOrderedByXpMinAsc(r.Context())
	if err != nil {
		slog.Error("admin loyalty list tiers", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	dtos := make([]loyaltyTierDTO, 0, len(tiers))
	for i := range tiers {
		dtos = append(dtos, tierToDTO(&tiers[i]))
	}
	writeJSON(w, http.StatusOK, dtos)
}

type createTierReq struct {
	XpMin           int64   `json:"xp_min"`
	DiscountPercent int     `json:"discount_percent"`
	DisplayName     *string `json:"display_name"`
}

// CreateTier — POST /cabinet/api/admin/loyalty/tiers
func (h *AdminLoyaltyHandler) CreateTier(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req createTierReq
	if !decodeJSON(w, r, &req) {
		return
	}

	maxSort, err := h.tiers.MaxSortOrder(r.Context())
	if err != nil {
		slog.Error("admin loyalty max sort order", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	id, err := h.tiers.Insert(r.Context(), maxSort+1, req.XpMin, req.DiscountPercent, req.DisplayName)
	if err != nil {
		slog.Error("admin loyalty create tier", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	tier, err := h.tiers.GetByID(r.Context(), id)
	if err != nil {
		slog.Error("admin loyalty get after create", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	dto := tierToDTO(tier)
	writeJSON(w, http.StatusCreated, dto)
}

// UpdateTier — PATCH /cabinet/api/admin/loyalty/tiers/{id}
func (h *AdminLoyaltyHandler) UpdateTier(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := extractLoyaltyTierID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	existing, err := h.tiers.GetByID(r.Context(), id)
	if err != nil {
		slog.Error("admin loyalty get tier for update", "error", err.Error())
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var raw map[string]json.RawMessage
	if !decodeJSON(w, r, &raw) {
		return
	}

	sortOrder := existing.SortOrder
	xpMin := existing.XpMin
	discountPct := existing.DiscountPercent
	displayName := existing.DisplayName

	if v, has := raw["sort_order"]; has {
		var n int
		if err := json.Unmarshal(v, &n); err == nil {
			sortOrder = n
		}
	}
	if v, has := raw["xp_min"]; has {
		var n int64
		if err := json.Unmarshal(v, &n); err == nil {
			xpMin = n
		}
	}
	if v, has := raw["discount_percent"]; has {
		var n int
		if err := json.Unmarshal(v, &n); err == nil {
			discountPct = n
		}
	}
	if v, has := raw["display_name"]; has {
		var s *string
		if err := json.Unmarshal(v, &s); err == nil {
			displayName = s
		}
	}

	if err := h.tiers.Update(r.Context(), id, sortOrder, xpMin, discountPct, displayName); err != nil {
		slog.Error("admin loyalty update tier", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	tier, err := h.tiers.GetByID(r.Context(), id)
	if err != nil {
		slog.Error("admin loyalty get after update", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	dto := tierToDTO(tier)
	writeJSON(w, http.StatusOK, dto)
}

// DeleteTier — DELETE /cabinet/api/admin/loyalty/tiers/{id}
func (h *AdminLoyaltyHandler) DeleteTier(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := extractLoyaltyTierID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	existing, err := h.tiers.GetByID(r.Context(), id)
	if err != nil {
		slog.Error("admin loyalty get tier for delete", "error", err.Error())
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if existing.SortOrder == 0 {
		http.Error(w, "cannot delete base tier (sort_order=0)", http.StatusBadRequest)
		return
	}

	if err := h.tiers.Delete(r.Context(), id); err != nil {
		slog.Error("admin loyalty delete tier", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// TriggerRecalc — POST /cabinet/api/admin/loyalty/recalc
func (h *AdminLoyaltyHandler) TriggerRecalc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !atomic.CompareAndSwapInt32(&recalcRunning, 0, 1) {
		http.Error(w, "recalc already in progress", http.StatusConflict)
		return
	}

	utils.Go("loyalty recalc", func() {
		defer atomic.StoreInt32(&recalcRunning, 0)

		ctx := context.Background()
		allPurchases, err := h.purchases.ListAllPaidForLoyaltyBackfill(ctx)
		if err != nil {
			slog.Error("admin loyalty recalc: list purchases", "error", err.Error())
			return
		}

		sums := loyalty.BuildCustomerXPSumsFromPaidPurchases(allPurchases)

		if err := h.customers.ApplyLoyaltyXPFullRecalc(ctx, sums); err != nil {
			slog.Error("admin loyalty recalc: apply xp", "error", err.Error())
			return
		}
		slog.Info("admin loyalty recalc completed", "customers", len(sums))
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// Handle dispatches /cabinet/api/admin/loyalty/tiers (no trailing path).
func (h *AdminLoyaltyHandler) Handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.ListTiers(w, r)
	case http.MethodPost:
		h.CreateTier(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleByID dispatches /cabinet/api/admin/loyalty/tiers/{id}.
func (h *AdminLoyaltyHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPatch:
		h.UpdateTier(w, r)
	case http.MethodDelete:
		h.DeleteTier(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
