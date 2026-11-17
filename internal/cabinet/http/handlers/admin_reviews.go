package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"remnawave-tg-shop-bot/internal/cabinet/repository"
)

// AdminReviewsHandler — список / скрытое редактирование / удаление отзывов в админке.
type AdminReviewsHandler struct {
	reviews *repository.ReviewRepo
}

func NewAdminReviews(reviews *repository.ReviewRepo) *AdminReviewsHandler {
	return &AdminReviewsHandler{reviews: reviews}
}

type adminReviewItemDTO struct {
	ID              int64    `json:"id"`
	AccountID       int64    `json:"account_id"`
	CustomerID      *int64   `json:"customer_id,omitempty"`
	Rating          float64  `json:"rating"`
	Body            string   `json:"body"`
	Visibility      string   `json:"visibility"`
	DisplayName     string   `json:"display_name"`
	AvatarURL       string   `json:"avatar_url,omitempty"`
	AuthorName      *string  `json:"author_name,omitempty"`
	AuthorAvatarURL *string  `json:"author_avatar_url,omitempty"`
	ClientSince     *string  `json:"client_since,omitempty"`
	TariffName      string   `json:"tariff_name"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	EditedAt        *string  `json:"edited_at,omitempty"`
	HasOriginal     bool     `json:"has_original"`
}

type adminReviewsListDTO struct {
	Items  []adminReviewItemDTO `json:"items"`
	Total  int64                `json:"total"`
	Limit  int                  `json:"limit"`
	Offset int                  `json:"offset"`
}

type adminReviewUpdateReq struct {
	Rating      float64 `json:"rating"`
	Body        string  `json:"body"`
	Visibility  string  `json:"visibility"`
	DisplayName string  `json:"display_name"`
	AvatarURL   string  `json:"avatar_url"`
	TariffName  string  `json:"tariff_name"`
}

// List — GET /cabinet/api/admin/reviews
func (h *AdminReviewsHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.reviews == nil {
		http.Error(w, "reviews unavailable", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	total, err := h.reviews.Count(r.Context())
	if err != nil {
		slog.Error("admin reviews count", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	items, err := h.reviews.List(r.Context(), limit, offset)
	if err != nil {
		slog.Error("admin reviews list", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := make([]adminReviewItemDTO, 0, len(items))
	for i := range items {
		out = append(out, toAdminReviewItem(&items[i]))
	}
	writeJSON(w, http.StatusOK, adminReviewsListDTO{Items: out, Total: total, Limit: limit, Offset: offset})
}

// HandleByID — /cabinet/api/admin/reviews/{id}
func (h *AdminReviewsHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	id, ok := adminReviewIDFromPath(r.URL.Path)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodPut:
		h.updateSilent(w, r, id)
	case http.MethodDelete:
		h.delete(w, r, id)
	default:
		w.Header().Set("Allow", http.MethodPut+", "+http.MethodDelete)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *AdminReviewsHandler) updateSilent(w http.ResponseWriter, r *http.Request, id int64) {
	if h.reviews == nil {
		http.Error(w, "reviews unavailable", http.StatusServiceUnavailable)
		return
	}
	var req adminReviewUpdateReq
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
	if vis == repository.ReviewVisibilityAnonymous {
		displayName, avatarURL = "", ""
	} else if vis == repository.ReviewVisibilityName {
		avatarURL = ""
	}
	in := repository.ReviewUpsertInput{
		Rating:      req.Rating,
		Body:        body,
		Visibility:  vis,
		DisplayName: truncateRunes(displayName, 120),
		AvatarURL:   avatarURL,
		TariffName:  truncateRunes(strings.TrimSpace(req.TariffName), 255),
	}
	rev, err := h.reviews.AdminUpdateSilent(r.Context(), id, in)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, repository.ErrReviewValidation) {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		slog.Error("admin reviews update", "id", id, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, toAdminReviewItem(rev))
}

func (h *AdminReviewsHandler) delete(w http.ResponseWriter, r *http.Request, id int64) {
	if h.reviews == nil {
		http.Error(w, "reviews unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := h.reviews.Delete(r.Context(), id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		slog.Error("admin reviews delete", "id", id, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toAdminReviewItem(rev *repository.Review) adminReviewItemDTO {
	dto := adminReviewItemDTO{
		ID:          rev.ID,
		AccountID:   rev.AccountID,
		CustomerID:  rev.CustomerID,
		Rating:      rev.Rating,
		Body:        rev.Body,
		Visibility:  rev.Visibility,
		DisplayName: rev.DisplayName,
		AvatarURL:   rev.AvatarURL,
		TariffName:  rev.TariffName,
		CreatedAt:   rev.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   rev.UpdatedAt.UTC().Format(time.RFC3339),
		HasOriginal: rev.EditedAt != nil && strings.TrimSpace(rev.OriginalBody) != "",
	}
	if rev.EditedAt != nil {
		s := rev.EditedAt.UTC().Format(time.RFC3339)
		dto.EditedAt = &s
	}
	switch rev.Visibility {
	case repository.ReviewVisibilityName, repository.ReviewVisibilityNameAvatar:
		if name := strings.TrimSpace(rev.DisplayName); name != "" {
			n := name
			dto.AuthorName = &n
		}
		if rev.Visibility == repository.ReviewVisibilityNameAvatar {
			if av := strings.TrimSpace(rev.AvatarURL); av != "" {
				a := av
				dto.AuthorAvatarURL = &a
			}
		}
	}
	if rev.ClientSince != nil {
		s := rev.ClientSince.UTC().Format("2006-01")
		dto.ClientSince = &s
	}
	return dto
}

func adminReviewIDFromPath(path string) (int64, bool) {
	// /cabinet/api/admin/reviews/12
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == "reviews" && i+1 < len(parts) {
			id, err := strconv.ParseInt(parts[i+1], 10, 64)
			if err == nil && id > 0 {
				return id, true
			}
		}
	}
	return 0, false
}
