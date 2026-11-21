package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"
)

// AdminReviewsHandler — модерация отзывов: список, правка, ответ, удаление,
// отзыв от имени клиента.
type AdminReviewsHandler struct {
	reviews   *repository.ReviewRepo
	links     *repository.AccountCustomerLinkRepo
	customers *database.CustomerRepository
	user      *ReviewsHandler // сборка отзыва тем же кодом, что у клиента
	notifier  *reviewNotifier
}

func NewAdminReviews(
	reviews *repository.ReviewRepo,
	links *repository.AccountCustomerLinkRepo,
	customers *database.CustomerRepository,
	user *ReviewsHandler,
) *AdminReviewsHandler {
	return &AdminReviewsHandler{reviews: reviews, links: links, customers: customers, user: user, notifier: newReviewNotifier()}
}

type adminReviewDTO struct {
	reviewPublicDTO
	Status       string `json:"status"`
	Visibility   string `json:"visibility"`
	DisplayName  string `json:"display_name"`
	OriginalBody string `json:"original_body,omitempty"`
	ByAdmin      bool   `json:"by_admin"`
	CustomerID   *int64 `json:"customer_id"`
	TelegramID   *int64 `json:"telegram_id"`
	UpdatedAt    string `json:"updated_at"`
	// PublishedAt — дата публикации (RFC3339) или пусто, если не опубликован.
	PublishedAt *string `json:"published_at"`
}

type adminReviewCounts struct {
	All       int64 `json:"all"`
	Pending   int64 `json:"pending"`
	Published int64 `json:"published"`
	Hidden    int64 `json:"hidden"`
}

type adminReviewUpdateReq struct {
	Rating      *int    `json:"rating"`
	Body        *string `json:"body"`
	Visibility  *string `json:"visibility"`
	DisplayName *string `json:"display_name"`
	Status      *string `json:"status"`
	Reply       *string `json:"reply"`
	// PublishedAt — дата отзыва «ГГГГ-ММ-ДД»; пусто — не менять.
	PublishedAt *string `json:"published_at"`
}

type adminReviewCreateReq struct {
	CustomerID  int64  `json:"customer_id"`
	Rating      int    `json:"rating"`
	Body        string `json:"body"`
	Visibility  string `json:"visibility"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
	// PublishedAt — дата отзыва «ГГГГ-ММ-ДД» (старые отзывы из канала); пусто — сегодня.
	PublishedAt string `json:"published_at"`
}

func toAdminReview(rev *repository.Review) adminReviewDTO {
	pub := toPublicReview(rev)
	// В админке дата — всегда создания: у «на проверке» нет даты публикации.
	pub.CreatedAt = rev.CreatedAt.UTC().Format(time.RFC3339)
	dto := adminReviewDTO{
		reviewPublicDTO: pub, Status: rev.Status, Visibility: rev.Visibility, DisplayName: rev.DisplayName,
		OriginalBody: rev.OriginalBody, ByAdmin: rev.ByAdmin, CustomerID: rev.CustomerID, TelegramID: rev.TelegramID,
		UpdatedAt: rev.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if rev.PublishedAt != nil {
		p := rev.PublishedAt.UTC().Format(time.RFC3339)
		dto.PublishedAt = &p
	}
	return dto
}

// parseReviewDate — «ГГГГ-ММ-ДД» → полдень UTC: так день не «съезжает» от часового пояса.
func parseReviewDate(raw string) (*time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, false
	}
	d = time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, time.UTC)
	return &d, true
}

// List — GET /cabinet/api/admin/reviews?status=&q=&limit=&offset=
func (h *AdminReviewsHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	items, total, err := h.reviews.AdminList(r.Context(), repository.ReviewAdminFilter{
		Status: strings.TrimSpace(q.Get("status")), Search: q.Get("q"),
	}, limit, offset)
	if err != nil {
		if errors.Is(err, repository.ErrReviewValidation) {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		slog.Error("admin reviews list", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	counts, err := h.reviews.Counts(r.Context())
	if err != nil {
		slog.Error("admin reviews counts", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := make([]adminReviewDTO, 0, len(items))
	for i := range items {
		out = append(out, toAdminReview(&items[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": config.ReviewsEnabled(),
		"items":   out,
		"total":   total,
		"counts":  adminReviewCounts{All: counts.All, Pending: counts.Pending, Published: counts.Published, Hidden: counts.Hidden},
	})
}

// Create — POST /cabinet/api/admin/reviews: отзыв от имени клиента.
func (h *AdminReviewsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var req adminReviewCreateReq
	if !decodeJSON(w, r, &req) {
		return
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = repository.ReviewStatusPublished
	}
	when, dateOK := parseReviewDate(req.PublishedAt)
	if !dateOK {
		http.Error(w, "invalid published_at", http.StatusBadRequest)
		return
	}
	cust, cerr := h.customers.FindById(r.Context(), req.CustomerID)
	if cerr != nil || cust == nil {
		http.Error(w, "customer not found", http.StatusNotFound)
		return
	}
	// У клиента может не быть аккаунта кабинета (бот-пользователь): тогда отзыв
	// привязывается прямо к клиенту, автор — Telegram-профиль из бота.
	var accountID int64
	if link, lerr := h.links.FindByCustomerID(r.Context(), cust.ID); lerr == nil && link != nil {
		accountID = link.AccountID
	}
	if existing, ferr := h.findExisting(r, cust.ID, accountID); ferr == nil {
		// Отзыв уже есть: правим его через AdminUpdate, чтобы не потерять оригинал текста.
		vis, dn := strings.TrimSpace(req.Visibility), strings.TrimSpace(req.DisplayName)
		rev, _, uerr := h.reviews.AdminUpdate(r.Context(), existing.ID, repository.ReviewAdminUpdate{
			Rating: &req.Rating, Body: &req.Body, Visibility: &vis, DisplayName: &dn, Status: &status, PublishedAt: when,
		})
		if uerr != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, toAdminReview(rev))
		return
	}
	in, ok := h.user.buildInput(r.Context(), accountID, reviewUpsertReq{
		Rating: req.Rating, Body: req.Body, Visibility: req.Visibility, DisplayName: req.DisplayName,
	})
	if !ok {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if accountID == 0 {
		h.user.fillBotAuthor(r.Context(), &in, cust)
	}
	in.PublishedAt = when
	rev, err := h.reviews.UpsertByAdmin(r.Context(), in, status)
	if err != nil {
		if errors.Is(err, repository.ErrReviewValidation) {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		slog.Error("admin review create", "customer_id", req.CustomerID, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, toAdminReview(rev))
}

// HandleByID — /cabinet/api/admin/reviews/{id} (PUT, DELETE) и
// /cabinet/api/admin/reviews/customer/{customer_id} (GET).
func (h *AdminReviewsHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/cabinet/api/admin/reviews/"), "/")
	if rest == "customers" {
		h.searchCustomers(w, r)
		return
	}
	if cid, ok := strings.CutPrefix(rest, "customer/"); ok {
		h.byCustomer(w, r, cid)
		return
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodPut:
		h.update(w, r, id)
	case http.MethodDelete:
		h.delete(w, r, id)
	default:
		w.Header().Set("Allow", http.MethodPut+", "+http.MethodDelete)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// findExisting — отзыв клиента: по аккаунту кабинета, а без него — по самому клиенту.
func (h *AdminReviewsHandler) findExisting(r *http.Request, customerID, accountID int64) (*repository.Review, error) {
	if accountID > 0 {
		return h.reviews.FindByAccountID(r.Context(), accountID)
	}
	return h.reviews.FindByCustomerNoAccount(r.Context(), customerID)
}

// byCustomer — отзыв клиента для окна «от имени клиента» в карточке пользователя.
func (h *AdminReviewsHandler) byCustomer(w http.ResponseWriter, r *http.Request, rawID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	cid, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || cid <= 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	resp := map[string]any{"customer_found": false, "has_account": false, "review": nil}
	if cust, cerr := h.customers.FindById(r.Context(), cid); cerr == nil && cust != nil {
		resp["customer_found"] = true
		var accountID int64
		if link, lerr := h.links.FindByCustomerID(r.Context(), cid); lerr == nil && link != nil {
			accountID = link.AccountID
			resp["has_account"] = true
		}
		if rev, ferr := h.findExisting(r, cid, accountID); ferr == nil {
			resp["review"] = toAdminReview(rev)
		} else if !errors.Is(ferr, repository.ErrNotFound) {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AdminReviewsHandler) update(w http.ResponseWriter, r *http.Request, id int64) {
	var req adminReviewUpdateReq
	if !decodeJSON(w, r, &req) {
		return
	}
	var when *time.Time
	if req.PublishedAt != nil {
		var ok bool
		if when, ok = parseReviewDate(*req.PublishedAt); !ok {
			http.Error(w, "invalid published_at", http.StatusBadRequest)
			return
		}
	}
	rev, replyChanged, err := h.reviews.AdminUpdate(r.Context(), id, repository.ReviewAdminUpdate{
		Rating: req.Rating, Body: req.Body, Visibility: req.Visibility, DisplayName: req.DisplayName,
		Status: req.Status, ReplyBody: req.Reply, PublishedAt: when,
	})
	switch {
	case errors.Is(err, repository.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case errors.Is(err, repository.ErrReviewValidation):
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	case err != nil:
		slog.Error("admin review update", "id", id, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if replyChanged && rev.CustomerID != nil {
		h.notifyReply(r, rev)
	}
	writeJSON(w, http.StatusOK, toAdminReview(rev))
}

func (h *AdminReviewsHandler) notifyReply(r *http.Request, rev *repository.Review) {
	lang := ""
	if cust, err := h.customers.FindById(r.Context(), *rev.CustomerID); err == nil && cust != nil {
		lang = cust.Language
	}
	h.notifier.notifyReply(r.Context(), *rev.CustomerID, lang, rev)
}

func (h *AdminReviewsHandler) delete(w http.ResponseWriter, r *http.Request, id int64) {
	if err := h.reviews.Delete(r.Context(), id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		slog.Error("admin review delete", "id", id, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type adminReviewCustomerDTO struct {
	ID               int64  `json:"id"`
	TelegramUsername string `json:"telegram_username,omitempty"`
	// TelegramID — настоящий Telegram ID; у web-клиентов пусто.
	TelegramID string `json:"telegram_id,omitempty"`
	Email      string `json:"email,omitempty"`
	HasAccount bool   `json:"has_account"`
}

// searchCustomers — GET /cabinet/api/admin/reviews/customers?q= : подсказки для окна
// «от имени клиента». Ищет по ID клиента (точно), @нику, Telegram ID и почте. Поиск
// «Пользователей» по ID не умеет, а в этом окне ID — самое частое, что вводят.
func (h *AdminReviewsHandler) searchCustomers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	needle := strings.TrimPrefix(strings.TrimSpace(r.URL.Query().Get("q")), "@")
	out := []adminReviewCustomerDTO{}
	if needle == "" {
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
		return
	}
	const limit = 8
	seen := map[int64]bool{}
	add := func(c *database.Customer) {
		if c == nil || seen[c.ID] || len(out) >= limit {
			return
		}
		seen[c.ID] = true
		dto := adminReviewCustomerDTO{ID: c.ID}
		if c.TelegramUsername != nil {
			dto.TelegramUsername = strings.TrimPrefix(strings.TrimSpace(*c.TelegramUsername), "@")
		}
		if !c.IsWebOnly && !utils.IsSyntheticTelegramID(c.TelegramID) {
			dto.TelegramID = strconv.FormatInt(c.TelegramID, 10)
		}
		dto.Email, dto.HasAccount = h.reviews.CustomerEmail(r.Context(), c.ID)
		out = append(out, dto)
	}
	// Точное совпадение по ID клиента — первым.
	if n, err := strconv.ParseInt(needle, 10, 64); err == nil && n > 0 {
		if c, cerr := h.customers.FindById(r.Context(), n); cerr == nil {
			add(c)
		}
	}
	if rows, err := h.customers.SearchForAdmin(r.Context(), needle, limit); err == nil {
		for i := range rows {
			add(&rows[i])
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}
