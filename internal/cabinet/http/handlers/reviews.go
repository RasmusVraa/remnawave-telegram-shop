package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/database"
)

// ReviewsHandler — публичный список отзывов и upsert своего отзыва.
type ReviewsHandler struct {
	reviews   *repository.ReviewRepo
	links     *repository.AccountCustomerLinkRepo
	customers *database.CustomerRepository
	tariffs   *database.TariffRepository
	identity  reviewIdentityResolver
}

type reviewIdentityResolver interface {
	ResolveForReview(r *http.Request, accountID int64) (displayName, avatarURL string)
}

func NewReviews(
	reviews *repository.ReviewRepo,
	links *repository.AccountCustomerLinkRepo,
	customers *database.CustomerRepository,
	tariffs *database.TariffRepository,
	identity reviewIdentityResolver,
) *ReviewsHandler {
	return &ReviewsHandler{
		reviews:   reviews,
		links:     links,
		customers: customers,
		tariffs:   tariffs,
		identity:  identity,
	}
}

type reviewPublicDTO struct {
	ID              int64    `json:"id"`
	Rating          float64  `json:"rating"`
	Body            string   `json:"body"`
	AuthorName      *string  `json:"author_name"`
	AuthorAvatarURL *string  `json:"author_avatar_url"`
	ClientSince     *string  `json:"client_since"`
	TariffName      string   `json:"tariff_name"`
	CreatedAt       string   `json:"created_at"`
	EditedAt        *string  `json:"edited_at,omitempty"`
	HasOriginal     bool     `json:"has_original"`
}

type reviewsListDTO struct {
	Items []reviewPublicDTO `json:"items"`
}

type reviewMineDTO struct {
	reviewPublicDTO
	Visibility  string `json:"visibility"`
	DisplayName string `json:"display_name,omitempty"`
	HasReview   bool   `json:"has_review"`
}

type reviewUpsertReq struct {
	Rating      float64 `json:"rating"`
	Body        string  `json:"body"`
	Visibility  string  `json:"visibility"`
	DisplayName string  `json:"display_name"`
}

type reviewOriginalDTO struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

// List — GET /cabinet/api/public/reviews
func (h *ReviewsHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, err := h.reviews.List(r.Context(), limit, offset)
	if err != nil {
		slog.Error("reviews list", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := make([]reviewPublicDTO, 0, len(items))
	for i := range items {
		out = append(out, toPublicReview(&items[i]))
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	writeJSON(w, http.StatusOK, reviewsListDTO{Items: out})
}

// Featured — GET /cabinet/api/public/reviews/featured
func (h *ReviewsHandler) Featured(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.reviews.Featured(r.Context(), limit)
	if err != nil {
		slog.Error("reviews featured", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := make([]reviewPublicDTO, 0, len(items))
	for i := range items {
		out = append(out, toPublicReview(&items[i]))
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, reviewsListDTO{Items: out})
}

// Original — GET /cabinet/api/public/reviews/{id}/original
func (h *ReviewsHandler) Original(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, ok := reviewIDFromPath(r.URL.Path)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rev, err := h.reviews.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		slog.Error("reviews original", "id", id, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if rev.EditedAt == nil || strings.TrimSpace(rev.OriginalBody) == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, reviewOriginalDTO{ID: rev.ID, Body: rev.OriginalBody})
}

// Mine — GET /cabinet/api/me/review
func (h *ReviewsHandler) Mine(w http.ResponseWriter, r *http.Request) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rev, err := h.reviews.FindByAccountID(r.Context(), claims.AccountID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeJSON(w, http.StatusOK, reviewMineDTO{HasReview: false})
			return
		}
		slog.Error("reviews mine", "account_id", claims.AccountID, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, toMineReview(rev))
}

// Upsert — PUT /cabinet/api/me/review
func (h *ReviewsHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req reviewUpsertReq
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
	avatarURL := ""
	if h.identity != nil {
		resolvedName, resolvedAvatar := h.identity.ResolveForReview(r, claims.AccountID)
		if displayName == "" {
			displayName = resolvedName
		}
		avatarURL = resolvedAvatar
	}
	if vis == repository.ReviewVisibilityAnonymous {
		displayName, avatarURL = "", ""
	} else if vis == repository.ReviewVisibilityName {
		avatarURL = ""
	}

	in := repository.ReviewUpsertInput{
		AccountID:   claims.AccountID,
		Rating:      req.Rating,
		Body:        body,
		Visibility:  vis,
		DisplayName: truncateRunes(displayName, 120),
		AvatarURL:   strings.TrimSpace(avatarURL),
	}

	if h.links != nil && h.customers != nil {
		if link, err := h.links.FindByAccountID(r.Context(), claims.AccountID); err == nil && link != nil {
			cid := link.CustomerID
			in.CustomerID = &cid
			if cust, err := h.customers.FindById(r.Context(), cid); err == nil && cust != nil {
				since := time.Date(cust.CreatedAt.UTC().Year(), cust.CreatedAt.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
				in.ClientSince = &since
				if cust.CurrentTariffID != nil && h.tariffs != nil {
					tid := *cust.CurrentTariffID
					in.TariffID = &tid
					if t, err := h.tariffs.GetByID(r.Context(), tid); err == nil && t != nil && t.Name != nil {
						in.TariffName = strings.TrimSpace(*t.Name)
					}
				}
			}
		}
	}

	rev, err := h.reviews.Upsert(r.Context(), in)
	if err != nil {
		if errors.Is(err, repository.ErrReviewValidation) {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		slog.Error("reviews upsert", "account_id", claims.AccountID, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, toMineReview(rev))
}

func toMineReview(rev *repository.Review) reviewMineDTO {
	dto := reviewMineDTO{
		reviewPublicDTO: toPublicReview(rev),
		Visibility:      rev.Visibility,
		DisplayName:     rev.DisplayName,
		HasReview:       true,
	}
	if rev.DisplayName != "" {
		name := rev.DisplayName
		dto.AuthorName = &name
	}
	if rev.AvatarURL != "" {
		av := rev.AvatarURL
		dto.AuthorAvatarURL = &av
	}
	return dto
}

func toPublicReview(rev *repository.Review) reviewPublicDTO {
	dto := reviewPublicDTO{
		ID:          rev.ID,
		Rating:      rev.Rating,
		Body:        rev.Body,
		TariffName:  rev.TariffName,
		CreatedAt:   rev.CreatedAt.UTC().Format(time.RFC3339),
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

func reviewIDFromPath(path string) (int64, bool) {
	// /cabinet/api/public/reviews/12/original
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

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}
