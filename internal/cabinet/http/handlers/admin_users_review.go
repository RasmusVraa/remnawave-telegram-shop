package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"remnawave-tg-shop-bot/internal/cabinet/repository"
)

// SetReviews подключает репозиторий отзывов к карточке пользователя.
func (h *AdminUsersHandler) SetReviews(reviews *repository.ReviewRepo) {
	h.reviews = reviews
}

type adminUserReviewResp struct {
	HasReview       bool     `json:"has_review"`
	CanEdit         bool     `json:"can_edit"`
	ID              int64    `json:"id,omitempty"`
	Rating          float64  `json:"rating,omitempty"`
	Body            string   `json:"body,omitempty"`
	Visibility      string   `json:"visibility,omitempty"`
	DisplayName     string   `json:"display_name,omitempty"`
	AvatarURL       string   `json:"avatar_url,omitempty"`
	AuthorName      *string  `json:"author_name,omitempty"`
	AuthorAvatarURL *string  `json:"author_avatar_url,omitempty"`
	ClientSince     *string  `json:"client_since,omitempty"`
	TariffName      string   `json:"tariff_name,omitempty"`
	CreatedAt       string   `json:"created_at,omitempty"`
	EditedAt        *string  `json:"edited_at,omitempty"`
	HasOriginal     bool     `json:"has_original,omitempty"`
}

type adminUserReviewUpsertReq struct {
	Rating      float64 `json:"rating"`
	Body        string  `json:"body"`
	Visibility  string  `json:"visibility"`
	DisplayName string  `json:"display_name"`
	AvatarURL   string  `json:"avatar_url"`
}

// GetReview — GET /cabinet/api/admin/users/{id}/review
func (h *AdminUsersHandler) GetReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, ok := adminUsersExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	cust, err := h.customers.FindById(r.Context(), id)
	if err != nil || cust == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	ci, _ := h.customers.CabinetIdentityByCustomerID(r.Context(), cust.ID)
	canEdit := ci != nil && ci.AccountID > 0
	out := adminUserReviewResp{CanEdit: canEdit}
	if h.reviews == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	rev, err := h.reviews.FindByCustomerID(r.Context(), cust.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeJSON(w, http.StatusOK, out)
			return
		}
		slog.Error("admin review get", "customer_id", id, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out = adminReviewToResp(rev, canEdit)
	writeJSON(w, http.StatusOK, out)
}

// PutReview — PUT /cabinet/api/admin/users/{id}/review — отзыв от имени клиента.
func (h *AdminUsersHandler) PutReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.reviews == nil {
		http.Error(w, "reviews unavailable", http.StatusServiceUnavailable)
		return
	}
	id, ok := adminUsersExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	cust, err := h.customers.FindById(r.Context(), id)
	if err != nil || cust == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	ci, err := h.customers.CabinetIdentityByCustomerID(r.Context(), cust.ID)
	if err != nil {
		slog.Error("admin review cabinet", "customer_id", id, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if ci == nil || ci.AccountID <= 0 {
		http.Error(w, "cabinet account required", http.StatusBadRequest)
		return
	}

	var req adminUserReviewUpsertReq
	if !decodeJSON(w, r, &req) {
		return
	}
	body := strings.TrimSpace(req.Body)
	if utf8.RuneCountInString(body) < 10 || utf8.RuneCountInString(body) > 2000 {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	vis := strings.TrimSpace(req.Visibility)
	switch vis {
	case repository.ReviewVisibilityAnonymous, repository.ReviewVisibilityName, repository.ReviewVisibilityNameAvatar:
	default:
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	displayName := strings.TrimSpace(req.DisplayName)
	avatarURL := strings.TrimSpace(req.AvatarURL)
	if displayName == "" {
		if cust.TelegramUsername != nil {
			displayName = strings.TrimPrefix(strings.TrimSpace(*cust.TelegramUsername), "@")
		}
		if displayName == "" {
			displayName = strings.TrimSpace(ci.Email)
		}
	}
	if avatarURL == "" {
		avatarURL = h.avatarURL(cust)
	}
	if vis == repository.ReviewVisibilityAnonymous {
		displayName, avatarURL = "", ""
	} else if vis == repository.ReviewVisibilityName {
		avatarURL = ""
	}

	in := repository.ReviewUpsertInput{
		AccountID:   ci.AccountID,
		CustomerID:  &cust.ID,
		Rating:      req.Rating,
		Body:        body,
		Visibility:  vis,
		DisplayName: truncateRunes(displayName, 120),
		AvatarURL:   avatarURL,
	}
	since := time.Date(cust.CreatedAt.UTC().Year(), cust.CreatedAt.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	in.ClientSince = &since
	if cust.CurrentTariffID != nil && h.tariffs != nil {
		tid := *cust.CurrentTariffID
		in.TariffID = &tid
		if t, err := h.tariffs.GetByID(r.Context(), tid); err == nil && t != nil && t.Name != nil {
			in.TariffName = strings.TrimSpace(*t.Name)
		}
	}

	rev, err := h.reviews.Upsert(r.Context(), in)
	if err != nil {
		if errors.Is(err, repository.ErrReviewValidation) {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		slog.Error("admin review upsert", "customer_id", id, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	slog.Info("admin: upsert review", "customer_id", id, "review_id", rev.ID, "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, adminReviewToResp(rev, true))
}

func adminReviewToResp(rev *repository.Review, canEdit bool) adminUserReviewResp {
	pub := toPublicReview(rev)
	return adminUserReviewResp{
		HasReview:       true,
		CanEdit:         canEdit,
		ID:              rev.ID,
		Rating:          rev.Rating,
		Body:            rev.Body,
		Visibility:      rev.Visibility,
		DisplayName:     rev.DisplayName,
		AvatarURL:       rev.AvatarURL,
		AuthorName:      pub.AuthorName,
		AuthorAvatarURL: pub.AuthorAvatarURL,
		ClientSince:     pub.ClientSince,
		TariffName:      rev.TariffName,
		CreatedAt:       pub.CreatedAt,
		EditedAt:        pub.EditedAt,
		HasOriginal:     pub.HasOriginal,
	}
}
