package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"
)

// ReviewTrialLabel — значение tariff_label у клиента на пробном периоде: фронт
// подставляет вместо него локализованное «Пробный».
const ReviewTrialLabel = "@trial"

// Причины, по которым писать пока нельзя (поле reason в /me/review).
const (
	reviewReasonVerify    = "verify"    // нет подтверждённой почты или Telegram
	reviewReasonNeedPaid  = "need_paid" // нужна хотя бы одна оплата
	reviewReasonNeedSub   = "need_sub"  // нужна подписка: пробная или оплаченная
	reviewReasonNoAccount = "no_customer"
)

// ReviewsHandler — публичные отзывы и отзыв самого клиента.
type ReviewsHandler struct {
	reviews   *repository.ReviewRepo
	links     *repository.AccountCustomerLinkRepo
	customers *database.CustomerRepository
	purchases *database.PurchaseRepository
	tariffs   *database.TariffRepository
	identity  reviewIdentity
	notifier  *reviewNotifier
}

// reviewIdentity — имя, аватар и скачивание аватара Telegram (реализует MeHandler).
type reviewIdentity interface {
	ReviewIdentity(ctx context.Context, accountID int64) reviewAuthor
	ReviewAvatar(ctx context.Context, telegramID int64) (body []byte, contentType, etag string, ok bool)
}

type reviewAuthor struct {
	Name       string
	AvatarKind string
	AvatarURL  string
	TelegramID *int64
	// PreviewURL — ссылка на аватар для предпросмотра в форме (с токеном, живёт недолго, в отзыв не попадает).
	PreviewURL string
}

func NewReviews(
	reviews *repository.ReviewRepo,
	links *repository.AccountCustomerLinkRepo,
	customers *database.CustomerRepository,
	purchases *database.PurchaseRepository,
	tariffs *database.TariffRepository,
	identity reviewIdentity,
) *ReviewsHandler {
	return &ReviewsHandler{
		reviews: reviews, links: links, customers: customers, purchases: purchases,
		tariffs: tariffs, identity: identity, notifier: newReviewNotifier(),
	}
}

type reviewReplyDTO struct {
	Body string `json:"body"`
	At   string `json:"at"`
}

type reviewPublicDTO struct {
	ID              int64           `json:"id"`
	Rating          int             `json:"rating"`
	Body            string          `json:"body"`
	AuthorName      *string         `json:"author_name"`
	AuthorAvatarURL *string         `json:"author_avatar_url"`
	ClientSince     *string         `json:"client_since"`
	TariffLabel     string          `json:"tariff_label"`
	IsTrial         bool            `json:"is_trial"`
	CreatedAt       string          `json:"created_at"`
	Reply           *reviewReplyDTO `json:"reply,omitempty"`
}

type reviewMineDTO struct {
	reviewPublicDTO
	Status      string `json:"status"`
	Visibility  string `json:"visibility"`
	DisplayName string `json:"display_name"`
}

type reviewMineResp struct {
	Enabled   bool   `json:"enabled"`
	Moderated bool   `json:"moderated"`
	CanWrite  bool   `json:"can_write"`
	Reason    string `json:"reason,omitempty"`
	Policy    string `json:"policy"`
	// Author — как клиент выглядит сейчас: имя и аватар для предпросмотра формы.
	Author *reviewAuthorDTO `json:"author,omitempty"`
	Review *reviewMineDTO   `json:"review"`
}

type reviewAuthorDTO struct {
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

type reviewUpsertReq struct {
	Rating      int    `json:"rating"`
	Body        string `json:"body"`
	Visibility  string `json:"visibility"`
	DisplayName string `json:"display_name"`
}

// List — GET /cabinet/api/public/reviews
func (h *ReviewsHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !config.ReviewsEnabled() {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "items": []reviewPublicDTO{}})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, err := h.reviews.ListPublished(r.Context(), limit, offset)
	if err != nil {
		slog.Error("reviews list", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// Без кэша браузера: клиент, отправивший отзыв в режиме «сразу», должен увидеть его на странице тут же.
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "items": toPublicReviews(items)})
}

// Featured — GET /cabinet/api/public/reviews/featured: случайные для лендинга.
// Меньше трёх опубликованных — пусто: одинокая карточка выглядит как пустота.
func (h *ReviewsHandler) Featured(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	out := []reviewPublicDTO{}
	if limit := config.ReviewsLandingLimit(); config.ReviewsEnabled() && limit > 0 {
		if n, err := h.reviews.CountPublished(r.Context()); err == nil && n >= 3 {
			items, err := h.reviews.Featured(r.Context(), limit)
			if err != nil {
				slog.Error("reviews featured", "error", err.Error())
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			out = toPublicReviews(items)
		}
	}
	// Случайная выборка: кэш короткий, иначе «случайные» застынут надолго.
	w.Header().Set("Cache-Control", "public, max-age=20")
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// Avatar — GET /cabinet/api/public/reviews/{id}/avatar: аватар Telegram автора.
// Ссылку с токеном в отзыве не храним (она протухала через неделю): картинку
// достаём по id отзыва, и только у опубликованных с режимом «имя и аватар».
func (h *ReviewsHandler) Avatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	id, ok := reviewIDFromPath(r.URL.Path, "avatar")
	if !ok || !config.ReviewsEnabled() {
		http.NotFound(w, r)
		return
	}
	rev, err := h.reviews.FindByID(r.Context(), id)
	if err != nil || rev.Status != repository.ReviewStatusPublished ||
		rev.Visibility != repository.ReviewVisibilityNameAvatar ||
		rev.AvatarKind != repository.ReviewAvatarTelegram || rev.TelegramID == nil || h.identity == nil {
		http.NotFound(w, r)
		return
	}
	body, ctype, etag, found := h.identity.ReviewAvatar(r.Context(), *rev.TelegramID)
	// Публичная отдача: только настоящие картинки, иначе HTML из чужого ответа ушёл бы как есть.
	if !found || !strings.HasPrefix(strings.ToLower(ctype), "image/") {
		http.NotFound(w, r)
		return
	}
	hd := w.Header()
	hd.Set("ETag", etag)
	// Недолго: скрытый или анонимизированный отзыв не должен висеть в кэшах часами.
	hd.Set("Cache-Control", "public, max-age=300")
	if matchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	hd.Set("Content-Type", ctype)
	hd.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body) //nolint:gosec // G705: Content-Type проверен выше — только image/*, nosniff выставлен глобально
}

// Mine — GET /cabinet/api/me/review: свой отзыв и можно ли писать.
func (h *ReviewsHandler) Mine(w http.ResponseWriter, r *http.Request) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	resp := reviewMineResp{
		Enabled:   config.ReviewsEnabled(),
		Moderated: config.ReviewsModerated(),
		Policy:    config.ReviewsWritePolicy(),
	}
	if resp.Enabled {
		resp.CanWrite, resp.Reason = h.canWrite(r.Context(), claims.AccountID, claims.EmailVerified)
		if h.identity != nil {
			if a := h.identity.ReviewIdentity(r.Context(), claims.AccountID); a.Name != "" || a.PreviewURL != "" {
				resp.Author = &reviewAuthorDTO{Name: a.Name, AvatarURL: a.PreviewURL}
			}
		}
	}
	if rev, err := h.reviews.FindByAccountID(r.Context(), claims.AccountID); err == nil {
		dto := toMineReview(rev)
		resp.Review = &dto
	} else if !errors.Is(err, repository.ErrNotFound) {
		slog.Error("reviews mine", "account_id", claims.AccountID, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, resp)
}

// Upsert — PUT /cabinet/api/me/review.
func (h *ReviewsHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !config.ReviewsEnabled() {
		http.Error(w, "reviews disabled", http.StatusForbidden)
		return
	}
	var req reviewUpsertReq
	if !decodeJSON(w, r, &req) {
		return
	}
	// Уже есть отзыв — править можно всегда (право писать проверяем на первом).
	existing, ferr := h.reviews.FindByAccountID(r.Context(), claims.AccountID)
	if ferr != nil && !errors.Is(ferr, repository.ErrNotFound) {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if existing == nil {
		if ok, reason := h.canWrite(r.Context(), claims.AccountID, claims.EmailVerified); !ok {
			http.Error(w, "review not allowed: "+reason, http.StatusForbidden)
			return
		}
	}
	in, ok := h.buildInput(r.Context(), claims.AccountID, req)
	if !ok {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	rev, created, err := h.reviews.UpsertClient(r.Context(), in, config.ReviewsModerated())
	if err != nil {
		if errors.Is(err, repository.ErrReviewValidation) {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		slog.Error("reviews upsert", "account_id", claims.AccountID, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.notifier.notifyAdmin(r.Context(), created, rev)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, toMineReview(rev))
}

// buildInput собирает ReviewInput: имя и аватар — снимок на момент отправки.
func (h *ReviewsHandler) buildInput(ctx context.Context, accountID int64, req reviewUpsertReq) (repository.ReviewInput, bool) {
	body := strings.TrimSpace(req.Body)
	vis := strings.TrimSpace(req.Visibility)
	if req.Rating < 1 || req.Rating > 5 || utf8.RuneCountInString(body) < repository.ReviewBodyMin ||
		utf8.RuneCountInString(body) > repository.ReviewBodyMax {
		return repository.ReviewInput{}, false
	}
	switch vis {
	case repository.ReviewVisibilityAnonymous, repository.ReviewVisibilityName, repository.ReviewVisibilityNameAvatar:
	default:
		return repository.ReviewInput{}, false
	}
	name := strings.TrimSpace(req.DisplayName)
	if utf8.RuneCountInString(name) > 120 {
		return repository.ReviewInput{}, false
	}
	in := repository.ReviewInput{AccountID: accountID, Rating: req.Rating, Body: body, Visibility: vis}

	var author reviewAuthor
	if h.identity != nil {
		author = h.identity.ReviewIdentity(ctx, accountID)
	}
	if vis != repository.ReviewVisibilityAnonymous {
		in.DisplayName = name
		if in.DisplayName == "" {
			in.DisplayName = author.Name
		}
	}
	if vis == repository.ReviewVisibilityNameAvatar {
		in.AvatarKind, in.AvatarURL, in.TelegramID = author.AvatarKind, author.AvatarURL, author.TelegramID
	}
	h.fillCustomer(ctx, accountID, &in)
	return in, true
}

// fillCustomer — «клиент с месяца» и подпись тарифа (или «Пробный»).
func (h *ReviewsHandler) fillCustomer(ctx context.Context, accountID int64, in *repository.ReviewInput) {
	h.fillFromCustomer(ctx, h.customerFor(ctx, accountID), in)
}

// fillFromCustomer — id клиента, «клиент с» и подпись тарифа (или «Пробный»).
func (h *ReviewsHandler) fillFromCustomer(ctx context.Context, cust *database.Customer, in *repository.ReviewInput) {
	if cust == nil {
		return
	}
	cid := cust.ID
	in.CustomerID = &cid
	created := cust.CreatedAt.UTC()
	since := time.Date(created.Year(), created.Month(), created.Day(), 0, 0, 0, 0, time.UTC)
	in.ClientSince = &since
	paid, _ := h.purchases.HasPaidSubscription(ctx, cust.ID)
	switch {
	case paid && cust.CurrentTariffID != nil && h.tariffs != nil:
		if t, err := h.tariffs.GetByID(ctx, *cust.CurrentTariffID); err == nil && t != nil && t.Name != nil {
			in.TariffLabel = strings.TrimSpace(*t.Name)
		}
	case !paid && hasSubscription(cust):
		in.TariffLabel = ReviewTrialLabel
	}
}

func (h *ReviewsHandler) customerFor(ctx context.Context, accountID int64) *database.Customer {
	if h.links == nil || h.customers == nil {
		return nil
	}
	link, err := h.links.FindByAccountID(ctx, accountID)
	if err != nil || link == nil {
		return nil
	}
	cust, err := h.customers.FindById(ctx, link.CustomerID)
	if err != nil {
		return nil
	}
	return cust
}

func hasSubscription(c *database.Customer) bool {
	return c != nil && c.SubscriptionLink != nil && strings.TrimSpace(*c.SubscriptionLink) != ""
}

// canWrite — может ли аккаунт оставить первый отзыв по правилу из настроек.
func (h *ReviewsHandler) canWrite(ctx context.Context, accountID int64, verified bool) (bool, string) {
	if !verified {
		return false, reviewReasonVerify
	}
	policy := config.ReviewsWritePolicy()
	if policy == config.ReviewsWriteVerified {
		return true, ""
	}
	cust := h.customerFor(ctx, accountID)
	if cust == nil {
		return false, reviewReasonNoAccount
	}
	paid, err := h.purchases.HasPaidSubscription(ctx, cust.ID)
	if err != nil {
		slog.Warn("reviews: paid check failed", "customer_id", cust.ID, "error", err.Error())
		return false, reviewReasonNeedPaid
	}
	if paid {
		return true, ""
	}
	if policy == config.ReviewsWriteTrialPaid {
		if hasSubscription(cust) {
			return true, ""
		}
		return false, reviewReasonNeedSub
	}
	return false, reviewReasonNeedPaid
}

func toPublicReviews(items []repository.Review) []reviewPublicDTO {
	out := make([]reviewPublicDTO, 0, len(items))
	for i := range items {
		out = append(out, toPublicReview(&items[i]))
	}
	return out
}

func toPublicReview(rev *repository.Review) reviewPublicDTO {
	created := rev.CreatedAt
	if rev.PublishedAt != nil {
		created = *rev.PublishedAt
	}
	dto := reviewPublicDTO{
		ID: rev.ID, Rating: rev.Rating, Body: rev.Body,
		TariffLabel: rev.TariffLabel, IsTrial: rev.TariffLabel == ReviewTrialLabel,
		CreatedAt: created.UTC().Format(time.RFC3339),
	}
	if dto.IsTrial {
		dto.TariffLabel = ""
	}
	if rev.Visibility != repository.ReviewVisibilityAnonymous {
		if name := strings.TrimSpace(rev.DisplayName); name != "" {
			dto.AuthorName = &name
		}
	}
	if rev.Visibility == repository.ReviewVisibilityNameAvatar {
		switch rev.AvatarKind {
		case repository.ReviewAvatarTelegram:
			u := "/cabinet/api/public/reviews/" + strconv.FormatInt(rev.ID, 10) + "/avatar?v=" + strconv.FormatInt(rev.UpdatedAt.Unix(), 10)
			dto.AuthorAvatarURL = &u
		case repository.ReviewAvatarURL:
			if u := strings.TrimSpace(rev.AvatarURL); u != "" {
				dto.AuthorAvatarURL = &u
			}
		}
	}
	if rev.ClientSince != nil {
		s := rev.ClientSince.UTC().Format("2006-01-02")
		dto.ClientSince = &s
	}
	if rev.ReplyBody != "" && rev.ReplyAt != nil {
		dto.Reply = &reviewReplyDTO{Body: rev.ReplyBody, At: rev.ReplyAt.UTC().Format(time.RFC3339)}
	}
	return dto
}

func toMineReview(rev *repository.Review) reviewMineDTO {
	dto := reviewMineDTO{
		reviewPublicDTO: toPublicReview(rev),
		Status:          rev.Status,
		Visibility:      rev.Visibility,
		DisplayName:     rev.DisplayName,
	}
	// Свой отзыв всегда показываем с датой создания, а не публикации.
	dto.CreatedAt = rev.CreatedAt.UTC().Format(time.RFC3339)
	return dto
}

// reviewIDFromPath достаёт id из /cabinet/api/public/reviews/{id}/{suffix}.
func reviewIDFromPath(path, suffix string) (int64, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == "reviews" && i+1 < len(parts) {
			if suffix != "" && (i+2 >= len(parts) || parts[i+2] != suffix) {
				return 0, false
			}
			id, err := strconv.ParseInt(parts[i+1], 10, 64)
			if err == nil && id > 0 {
				return id, true
			}
		}
	}
	return 0, false
}

// fillBotAuthor дополняет отзыв от имени клиента, у которого нет аккаунта кабинета
// (пользователь бота): данные о клиенте, имя из Telegram-ника и аватар Telegram.
func (h *ReviewsHandler) fillBotAuthor(ctx context.Context, in *repository.ReviewInput, cust *database.Customer) {
	h.fillFromCustomer(ctx, cust, in)
	if in.Visibility == repository.ReviewVisibilityAnonymous {
		return
	}
	if in.DisplayName == "" && cust.TelegramUsername != nil {
		if u := strings.TrimPrefix(strings.TrimSpace(*cust.TelegramUsername), "@"); u != "" {
			in.DisplayName = u
		}
	}
	if in.Visibility == repository.ReviewVisibilityNameAvatar && !cust.IsWebOnly && !utils.IsSyntheticTelegramID(cust.TelegramID) {
		tid := cust.TelegramID
		in.AvatarKind, in.TelegramID = repository.ReviewAvatarTelegram, &tid
	}
}
