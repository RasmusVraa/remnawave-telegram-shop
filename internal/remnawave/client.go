package remnawave

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/utils"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned when the API responds with 404.
var ErrNotFound = errors.New("not found")

// ErrUserNotFound — в панели нет пользователя с данным telegram_id (пустой ответ поиска).
var ErrUserNotFound = errors.New("user not found")

// ctxKey is an unexported type for context keys in this package.
type ctxKey string

// CtxKeyUsername is the context key used to pass the Telegram username.
const CtxKeyUsername ctxKey = "username"

// CtxKeyPanelUsername is an optional explicit username for Remnawave panel user.
const CtxKeyPanelUsername ctxKey = "panel_username"

type Client struct {
	httpClient *http.Client
	baseURL    string
	hwidPin    HwidLimitPinLookup
}

// HwidLimitPinLookup возвращает лимит устройств, закреплённый админом за
// клиентом; ok=false — не закреплён. customerID может быть 0 — тогда ищут по
// telegramID.
type HwidLimitPinLookup func(ctx context.Context, customerID, telegramID int64) (limit int, ok bool, err error)

// SetHwidLimitPin подключает источник закреплённых лимитов. Лимит в панель
// пишут оплата, докупка, промокоды, смена тарифа и очистка протухших слотов —
// подмена здесь, в единственной точке записи, держит закреплённое число при
// любом из них и при любом будущем месте записи.
func (r *Client) SetHwidLimitPin(lookup HwidLimitPinLookup) {
	r.hwidPin = lookup
}

// pinnedHwidLimit подменяет расчётный лимит закреплённым. Ошибка чтения из БД
// не останавливает запись: оплата важнее, а закреплённое число вернётся при
// следующей записи лимита.
func (r *Client) pinnedHwidLimit(ctx context.Context, customerID, telegramID int64, computed int) int {
	if r.hwidPin == nil {
		return computed
	}
	limit, ok, err := r.hwidPin(ctx, customerID, telegramID)
	if err != nil {
		slog.Error("hwid limit pin lookup failed, using computed limit", "error", err)
		return computed
	}
	if !ok || limit <= 0 {
		return computed
	}
	if limit != computed {
		slog.Info("hwid limit pinned by admin", "computed", computed, "pinned", limit)
	}
	return limit
}

// pinHwidLimitField подставляет закреплённый лимит в запрос. Если запрос лимит
// не трогал (nil), а закрепление есть, лимит всё равно выставляется — так
// заново созданный профиль панели сразу получает закреплённое число.
func (r *Client) pinHwidLimitField(ctx context.Context, customerID, telegramID int64, field **int) {
	computed := 0
	if *field != nil {
		computed = **field
	}
	if v := r.pinnedHwidLimit(ctx, customerID, telegramID, computed); v > 0 && v != computed {
		*field = &v
	}
}

type headerTransport struct {
	base       http.RoundTripper
	headers    map[string]string
	forceLocal bool
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())

	host := r.URL.Hostname()
	if t.forceLocal || strings.HasPrefix(host, "remnawave") || host == "127.0.0.1" || host == "localhost" {
		r.Header.Set("x-forwarded-for", "127.0.0.1")
		r.Header.Set("x-forwarded-proto", "https")
	}

	for key, value := range t.headers {
		r.Header.Set(key, value)
	}

	return t.base.RoundTrip(r)
}

func NewClient(baseURL, token, mode string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	headers := config.RemnawaveHeaders()
	if headers == nil {
		headers = make(map[string]string)
	}
	headers["Authorization"] = "Bearer " + token
	forceLocal := mode == "local"

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &headerTransport{
			base:       http.DefaultTransport,
			headers:    headers,
			forceLocal: forceLocal,
		},
	}

	return &Client{
		httpClient: client,
		baseURL:    baseURL,
	}
}

// ---------------------------------------------------------------------------
// Generic HTTP helpers
// ---------------------------------------------------------------------------

func (r *Client) doRequest(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, r.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("execute request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return respBody, resp.StatusCode, ErrNotFound
	}

	if resp.StatusCode >= 400 {
		var apiErr apiErrorResponse
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Message != "" {
			return respBody, resp.StatusCode, fmt.Errorf("API error %d: %s (code: %s)", resp.StatusCode, apiErr.Message, apiErr.ErrorCode)
		}
		return respBody, resp.StatusCode, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, resp.StatusCode, nil
}

func (r *Client) doJSON(ctx context.Context, method, path string, body, result any) error {
	respBody, status, err := r.doRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	// Remnawave 3.0.0: DELETE отвечает 204, фоновые операции — 202, и оба без тела.
	// Такие вызовы передают result == nil и выходят здесь.
	if result == nil {
		return nil
	}
	// Если тело ждали, а его нет — это ошибка, а не успех. Молча вернуть нулевую
	// структуру нельзя: вызывающий записал бы в БД пустой subscription_link
	// и expire_at = 0001-01-01, то есть тихо испортил бы подписку клиента.
	if len(bytes.TrimSpace(respBody)) == 0 {
		return fmt.Errorf("empty response body for %s %s (status %d)", method, path, status)
	}
	if err := json.Unmarshal(respBody, result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Ping
// ---------------------------------------------------------------------------

func (r *Client) Ping(ctx context.Context) error {
	path := fmt.Sprintf("/api/users?size=%d&start=%d", 1, 0)
	return r.doJSON(ctx, http.MethodGet, path, nil, nil)
}

// ---------------------------------------------------------------------------
// Users — list
// ---------------------------------------------------------------------------

// GetUsers выгружает всех пользователей панели.
//
// Идёт через /api/users/stream, а не через offset-пагинацию /api/users:
// одна кодовая ветка пагинации на весь клиент и никакого пропуска записей,
// если список меняется между страницами.
func (r *Client) GetUsers(ctx context.Context) ([]User, error) {
	return r.streamUsers(ctx, "", 0)
}

func matchUserAdminSearch(u User, rawNeedle, needleLower string) bool {
	if needleLower == "" {
		return false
	}
	if strings.Contains(strings.ToLower(u.Username), needleLower) {
		return true
	}
	if u.Description != nil {
		desc := strings.TrimSpace(*u.Description)
		if desc != "" && strings.Contains(strings.ToLower(desc), needleLower) {
			return true
		}
	}
	if u.Tag != nil {
		tag := strings.TrimSpace(*u.Tag)
		if tag != "" && strings.Contains(strings.ToLower(tag), needleLower) {
			return true
		}
	}
	if u.TelegramID != nil {
		idStr := strconv.FormatInt(*u.TelegramID, 10)
		if strings.Contains(idStr, rawNeedle) {
			return true
		}
	}
	return false
}

// FindUsersMatchingAdminSearch фильтрует загруженный список пользователей панели по подстроке (username, описание, тег, telegram id как текст).
func (r *Client) FindUsersMatchingAdminSearch(ctx context.Context, needle string) ([]User, error) {
	raw := strings.TrimSpace(needle)
	raw = strings.TrimPrefix(raw, "@")
	if raw == "" {
		return nil, nil
	}
	nLow := strings.ToLower(raw)
	all, err := r.GetUsers(ctx)
	if err != nil {
		return nil, err
	}
	var out []User
	for _, u := range all {
		if matchUserAdminSearch(u, raw, nLow) {
			out = append(out, u)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Users — get by Telegram ID
// ---------------------------------------------------------------------------

// streamUsers постранично забирает пользователей через GET /api/users/stream.
//
// Remnawave 3.0.0 удалил точечные lookup-эндпоинты (by-telegram-id, by-email,
// by-tag, by-id) и заменил их одним stream с фильтрами. filters — уже готовые
// query-параметры (например "telegramId=123"); пустая строка = выгрузить всех.
//
// Пагинация курсорная и асимметричная по типам: в запрос cursor уходит как
// число, а в ответе nextCursor — строка. Поэтому курсор здесь строка и
// подставляется как есть.
func (r *Client) streamUsers(ctx context.Context, filter string, limit int) ([]User, error) {
	const pageSize = 250
	// Потолок страниц: 250 000 пользователей — заведомо больше любой реальной панели.
	// Нужен на случай, если панель перестанет двигать курсор (например, не поймёт
	// его формат) и будет отдавать первую страницу с hasMore=true: без потолка
	// цикл копил бы страницы до OOM. Лучше упасть с внятной ошибкой.
	const maxPages = 1000

	var (
		users  []User
		cursor *string
		seen   = make(map[string]struct{}, 8)
	)

	for page := 0; ; page++ {
		if page >= maxPages {
			return nil, fmt.Errorf("stream users (%s): превышен потолок в %d страниц — панель не двигает курсор", filter, maxPages)
		}
		path := fmt.Sprintf("/api/users/stream?size=%d", pageSize)
		if filter != "" {
			path += "&" + filter
		}
		if cursor != nil && *cursor != "" {
			path += "&cursor=" + url.QueryEscape(*cursor)
		}

		var page apiResponse[usersStreamBody]
		if err := r.doJSON(ctx, http.MethodGet, path, nil, &page); err != nil {
			return nil, fmt.Errorf("stream users (%s): %w", filter, err)
		}

		users = append(users, page.Response.Users...)

		if limit > 0 && len(users) >= limit {
			return users[:limit], nil
		}
		// hasMore — основной признак конца; nextCursor страхует от зацикливания,
		// если панель однажды вернёт hasMore=true без курсора.
		if !page.Response.HasMore || page.Response.NextCursor == nil || *page.Response.NextCursor == "" {
			return users, nil
		}
		// Курсор обязан двигаться. Повтор уже виденного значения означает, что
		// панель его не приняла, — дальше был бы бесконечный цикл.
		next := *page.Response.NextCursor
		if _, repeated := seen[next]; repeated {
			return nil, fmt.Errorf("stream users (%s): курсор %q повторился — панель его не принимает", filter, next)
		}
		seen[next] = struct{}{}
		cursor = page.Response.NextCursor
	}
}

// getUsersByTelegramID ищет пользователей панели по Telegram ID.
// До 3.0.0 это был GET /api/users/by-telegram-id/{id}; теперь — фильтр stream.
//
// Результат дополнительно фильтруется на нашей стороне. Раньше отбор гарантировал
// сам эндпоинт — он по определению возвращал только нужного пользователя.
// Теперь это query-параметр, и если панель его однажды проигнорирует, сюда придут
// ВСЕ пользователи, а findUserBySuffix отдаст первого попавшегося: продление,
// смена лимита и удаление ушли бы на чужой аккаунт. Цена проверки — один проход
// по списку, поэтому доверять фильтру на слово не будем.
func (r *Client) getUsersByTelegramID(ctx context.Context, telegramID int64) ([]User, error) {
	users, err := r.streamUsers(ctx, "telegramId="+strconv.FormatInt(telegramID, 10), 0)
	if err != nil {
		return nil, err
	}
	filtered := make([]User, 0, len(users))
	for i := range users {
		if users[i].TelegramID != nil && *users[i].TelegramID == telegramID {
			filtered = append(filtered, users[i])
		}
	}
	if len(filtered) != len(users) {
		slog.Warn("remnawave: stream вернул чужие профили при фильтре по telegramId",
			"requested", len(users), "matched", len(filtered))
	}
	return filtered, nil
}

// ---------------------------------------------------------------------------
// Internal squads
// ---------------------------------------------------------------------------

func (r *Client) getInternalSquads(ctx context.Context) ([]internalSquadItem, error) {
	var resp apiResponse[internalSquadsResponse]
	if err := r.doJSON(ctx, http.MethodGet, "/api/internal-squads", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Response.InternalSquads, nil
}

// InternalSquad — internal squad для выбора в админке.
type InternalSquad struct {
	UUID uuid.UUID
	Name string
}

// ListInternalSquads возвращает internal squads с панели Remnawave.
func (r *Client) ListInternalSquads(ctx context.Context) ([]InternalSquad, error) {
	items, err := r.getInternalSquads(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]InternalSquad, len(items))
	for i, s := range items {
		out[i] = InternalSquad(s)
	}
	return out, nil
}

func filterSquadsBySelection(allSquads []internalSquadItem, selected map[uuid.UUID]uuid.UUID) []uuid.UUID {
	if len(selected) == 0 {
		result := make([]uuid.UUID, 0, len(allSquads))
		for _, s := range allSquads {
			result = append(result, s.UUID)
		}
		return result
	}
	result := make([]uuid.UUID, 0, len(selected))
	for _, s := range allSquads {
		if _, ok := selected[s.UUID]; ok {
			result = append(result, s.UUID)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// DecreaseSubscription
// ---------------------------------------------------------------------------

func (r *Client) DecreaseSubscription(ctx context.Context, telegramId int64, trafficLimit int, days int) (*time.Time, error) {
	users, err := r.getUsersByTelegramID(ctx, telegramId)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("user with telegramId %d not found", telegramId)
	}

	existingUser := findUserBySuffix(users, telegramId)

	updated, err := r.updateUser(ctx, 0, telegramId, existingUser, trafficLimit, days, false)
	if err != nil {
		return nil, err
	}

	return &updated.ExpireAt, nil
}

// ---------------------------------------------------------------------------
// CreateOrUpdateUser
// ---------------------------------------------------------------------------

func (r *Client) CreateOrUpdateUser(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, days int, isTrialUser bool) (*User, error) {
	existingUser, err := r.findExistingUserForCustomer(ctx, customerId, telegramId)
	if err != nil {
		return nil, err
	}
	if existingUser == nil {
		return r.createUser(ctx, customerId, telegramId, trafficLimit, days, isTrialUser)
	}
	return r.updateUser(ctx, customerId, telegramId, existingUser, trafficLimit, days, isTrialUser)
}

// ExtendSubscriptionByDaysPreserveSquads продлевает только expire_at (рефералка, промо-дни и т.п.):
// не трогает internal squads, external squad, tag, лимит трафика и стратегию — в отличие от CreateOrUpdateUser.
// Если пользователя в Remnawave ещё нет — создаётся как при обычной первой выдаче (createUser).
func (r *Client) ExtendSubscriptionByDaysPreserveSquads(ctx context.Context, customerID int64, telegramID int64, days int) (*User, error) {
	if days <= 0 {
		return nil, fmt.Errorf("invalid days: %d", days)
	}
	existingUser, err := r.findExistingUserForCustomer(ctx, customerID, telegramID)
	if err != nil {
		return nil, err
	}
	if existingUser == nil {
		return r.createUser(ctx, customerID, telegramID, config.TrafficLimit(), days, false)
	}
	// Дату считает панель (POST /actions/extend, 3.0.0+), а не мы: это убирает
	// гонку read-modify-write, когда бот и кабинет продлевают одного клиента
	// одновременно. Семантика проверена на стенде 3.3.2 и совпадает с
	// getNewExpire: истёкшему считается от now, активному — от expireAt.
	updated, err := r.ExtendUserDays(ctx, existingUser.ID, days)
	if err != nil {
		return nil, err
	}

	// Но статус extend чинит не всегда: EXPIRED он переводит в ACTIVE, а
	// DISABLED и LIMITED оставляет как есть (проверено на стенде). Без этого
	// клиент с превышенным трафиком или заблокированный админом оплатил бы
	// продление и остался отрезанным. Патчим только статус, не трогая expireAt,
	// поэтому гонка по дате не возвращается.
	if updated.Status != "ACTIVE" {
		patched, perr := r.PatchUser(ctx, &UpdateUserRequest{ID: &updated.ID, Status: "ACTIVE"})
		if perr != nil {
			// Ошибку наверх НЕ поднимаем осознанно: дни панель уже начислила.
			// Вызывающие (промокоды, рефералка, фортуна, админ-выдача) на ошибке
			// не сохраняют новый expire_at и могут повторить операцию — тогда
			// клиент получит дни второй раз. Двойное начисление необратимо,
			// а неснятый статус чинится следующей операцией или админом.
			slog.Error("extend succeeded but reactivation failed: user keeps non-active status",
				"user_id", updated.ID, "status", updated.Status, "error", perr)
		} else {
			updated = patched
		}
	}

	tgid := ""
	if existingUser.TelegramID != nil {
		tgid = strconv.FormatInt(*existingUser.TelegramID, 10)
	}
	slog.Info("extended subscription (expire only)", "telegramId", utils.MaskHalf(tgid), "days", days)
	return updated, nil
}

// ShrinkSubscriptionByDaysPreserveSquads уменьшает expire_at на days (положительное число дней),
// не трогая squads/tag/лимиты — зеркало ExtendSubscriptionByDaysPreserveSquads для списания (колесо фортуны).
func (r *Client) ShrinkSubscriptionByDaysPreserveSquads(ctx context.Context, customerID int64, telegramID int64, days int) (*User, error) {
	if days <= 0 {
		return nil, fmt.Errorf("shrink days must be positive, got %d", days)
	}
	existingUser, err := r.findExistingUserForCustomer(ctx, customerID, telegramID)
	if err != nil {
		return nil, err
	}
	if existingUser == nil {
		return nil, fmt.Errorf("remnawave: user not found for shrink")
	}
	newExpire := getNewExpire(-days, existingUser.ExpireAt)
	userUpdate := &UpdateUserRequest{
		ID:       &existingUser.ID,
		Status:   "ACTIVE",
		ExpireAt: &newExpire,
	}
	var resp apiResponse[User]
	if err := r.doJSON(ctx, http.MethodPatch, "/api/users", userUpdate, &resp); err != nil {
		return nil, err
	}
	tgid := ""
	if existingUser.TelegramID != nil {
		tgid = strconv.FormatInt(*existingUser.TelegramID, 10)
	}
	slog.Info("shrunk subscription (expire only)", "telegramId", utils.MaskHalf(tgid), "days", days)
	return &resp.Response, nil
}

// CreateOrUpdateUserFromNow обновляет подписку, считая срок от текущего времени.
func (r *Client) CreateOrUpdateUserFromNow(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, days int, isTrialUser bool) (*User, error) {
	existingUser, err := r.findExistingUserForCustomer(ctx, customerId, telegramId)
	if err != nil {
		return nil, err
	}
	if existingUser == nil {
		return r.createUser(ctx, customerId, telegramId, trafficLimit, days, isTrialUser)
	}
	base := time.Now().UTC().Add(-time.Second)
	return r.updateUserWithBase(ctx, customerId, telegramId, existingUser, trafficLimit, days, isTrialUser, &base)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func findUserBySuffix(users []User, telegramId int64) *User {
	suffix := fmt.Sprintf("_%d", telegramId)
	for i := range users {
		if strings.Contains(users[i].Username, suffix) {
			return &users[i]
		}
	}
	return &users[0]
}

// UsernameFromCtx извлекает Telegram username из контекста (CtxKeyUsername).
func UsernameFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeyUsername).(string); ok {
		return v
	}
	return ""
}

func panelUsernameFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeyPanelUsername).(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// findExistingUserForCustomer находит RW-профиль для customer.
// Для обычных TG клиентов ищем по telegram_id.
// Для web-only/synthetic fallback: exact panel_username (если передан) и префикс "<customer_id>_".
func (r *Client) findExistingUserForCustomer(ctx context.Context, customerID int64, telegramID int64) (*User, error) {
	users, err := r.getUsersByTelegramID(ctx, telegramID)
	if err != nil {
		return nil, err
	}
	if len(users) > 0 {
		return findUserBySuffix(users, telegramID), nil
	}

	panelUsername := panelUsernameFromCtx(ctx)
	if panelUsername != "" {
		exact, err := r.findUserByUsername(ctx, panelUsername)
		if err != nil {
			return nil, err
		}
		if exact != nil {
			return exact, nil
		}
	}

	if !utils.IsSyntheticTelegramID(telegramID) {
		return nil, nil
	}

	all, err := r.GetUsers(ctx)
	if err != nil {
		return nil, err
	}
	prefix := fmt.Sprintf("%d_", customerID)
	for i := range all {
		if strings.HasPrefix(strings.TrimSpace(all[i].Username), prefix) {
			return &all[i], nil
		}
	}
	return nil, nil
}

func (r *Client) updateUser(ctx context.Context, customerID, telegramID int64, existingUser *User, trafficLimit int, days int, isTrialUser bool) (*User, error) {
	return r.updateUserWithBase(ctx, customerID, telegramID, existingUser, trafficLimit, days, isTrialUser, nil)
}

func (r *Client) updateUserWithBase(ctx context.Context, customerID, telegramID int64, existingUser *User, trafficLimit int, days int, isTrialUser bool, baseExpire *time.Time) (*User, error) {
	expireBase := existingUser.ExpireAt
	if baseExpire != nil {
		expireBase = *baseExpire
	}
	newExpire := getNewExpire(days, expireBase)

	squads, err := r.getInternalSquads(ctx)
	if err != nil {
		return nil, err
	}

	selectedSquads := config.SquadUUIDs()
	if isTrialUser {
		selectedSquads = config.TrialInternalSquads()
	}
	squadIds := filterSquadsBySelection(squads, selectedSquads)

	strategy := config.TrafficLimitResetStrategy()
	if isTrialUser {
		strategy = config.TrialTrafficLimitResetStrategy()
	}

	tl := int64(trafficLimit)
	squadsCopy := append([]uuid.UUID(nil), squadIds...)
	userUpdate := &UpdateUserRequest{
		ID:                   &existingUser.ID,
		ExpireAt:             &newExpire,
		Status:               "ACTIVE",
		TrafficLimitBytes:    &tl,
		ActiveInternalSquads: &squadsCopy,
		TrafficLimitStrategy: normalizeStrategy(strategy),
	}

	if isTrialUser {
		trialLimit := config.TrialHwidLimit()
		if trialLimit > 0 {
			userUpdate.HwidDeviceLimit = &trialLimit
		}
	}
	r.pinHwidLimitField(ctx, customerID, telegramID, &userUpdate.HwidDeviceLimit)

	externalSquad := config.ExternalSquadUUID()
	if isTrialUser {
		externalSquad = config.TrialExternalSquadUUID()
	}
	if externalSquad != uuid.Nil {
		userUpdate.ExternalSquadUuid = &externalSquad
	}

	tag := config.RemnawaveTag()
	if isTrialUser {
		tag = config.TrialRemnawaveTag()
	}
	if isValidTag(tag) {
		userUpdate.Tag = &tag
	}

	username := UsernameFromCtx(ctx)
	if username != "" {
		userUpdate.Description = &username
	}

	var resp apiResponse[User]
	if err := r.doJSON(ctx, http.MethodPatch, "/api/users", userUpdate, &resp); err != nil {
		return nil, err
	}

	tgid := ""
	if existingUser.TelegramID != nil {
		tgid = strconv.FormatInt(*existingUser.TelegramID, 10)
	}
	slog.Info("updated user", "telegramId", utils.MaskHalf(tgid), "username", utils.MaskHalf(username), "days", days)
	return &resp.Response, nil
}

// TODO(cabinet-web-only): после Этапа 3 (CustomerBootstrapService) для клиентов
// с customer.IsWebOnly=true username должен формироваться как
// `cabinet_<cabinet_account_id>` (а не `<customer_id>_<synthetic_tg_id>`),
// а поле createReq.TelegramID должно оставаться nil.
// См. docs/cabinet/audit-telegram-id.md, разделы 3.1 и 3.2.
func (r *Client) createUser(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, days int, isTrialUser bool) (*User, error) {
	expireAt := time.Now().UTC().AddDate(0, 0, days)
	username := panelUsernameFromCtx(ctx)
	if username == "" {
		username = generateUsername(customerId, telegramId)
	}

	squads, err := r.getInternalSquads(ctx)
	if err != nil {
		return nil, err
	}

	selectedSquads := config.SquadUUIDs()
	if isTrialUser {
		selectedSquads = config.TrialInternalSquads()
	}
	squadIds := filterSquadsBySelection(squads, selectedSquads)

	externalSquad := config.ExternalSquadUUID()
	if isTrialUser {
		externalSquad = config.TrialExternalSquadUUID()
	}

	strategy := config.TrafficLimitResetStrategy()
	if isTrialUser {
		strategy = config.TrialTrafficLimitResetStrategy()
	}

	tl := int64(trafficLimit)
	squadsCreate := append([]uuid.UUID(nil), squadIds...)
	createReq := &CreateUserRequest{
		Username:             username,
		ActiveInternalSquads: &squadsCreate,
		Status:               "ACTIVE",
		ExpireAt:             expireAt,
		TrafficLimitStrategy: normalizeStrategy(strategy),
		TrafficLimitBytes:    &tl,
	}
	if !utils.IsSyntheticTelegramID(telegramId) {
		tid := telegramId
		createReq.TelegramID = &tid
	}
	if isTrialUser {
		trialLimit := config.TrialHwidLimit()
		if trialLimit > 0 {
			createReq.HwidDeviceLimit = &trialLimit
		}
	}
	r.pinHwidLimitField(ctx, customerId, telegramId, &createReq.HwidDeviceLimit)
	if externalSquad != uuid.Nil {
		createReq.ExternalSquadUuid = &externalSquad
	}
	tag := config.RemnawaveTag()
	if isTrialUser {
		tag = config.TrialRemnawaveTag()
	}
	if isValidTag(tag) {
		createReq.Tag = &tag
	}

	tgUsername := UsernameFromCtx(ctx)
	if tgUsername != "" {
		createReq.Description = &tgUsername
	}

	var resp apiResponse[User]
	if err := r.doJSON(ctx, http.MethodPost, "/api/users", createReq, &resp); err != nil {
		return nil, err
	}
	slog.Info("created user", "telegramId", utils.MaskHalf(strconv.FormatInt(telegramId, 10)), "username", utils.MaskHalf(tgUsername), "days", days)
	return &resp.Response, nil
}

// ---------------------------------------------------------------------------
// User info & devices
// ---------------------------------------------------------------------------

// GetUserInfo возвращает id пользователя панели и его лимит устройств.
// До 3.0.0 первым значением был uuid-строка; теперь числовой id.
func (r *Client) GetUserInfo(ctx context.Context, telegramId int64) (int64, int, error) {
	users, err := r.getUsersByTelegramID(ctx, telegramId)
	if err != nil {
		return 0, 0, err
	}
	if len(users) == 0 {
		return 0, 0, ErrUserNotFound
	}

	user := findUserBySuffix(users, telegramId)
	deviceLimit := 0
	if user.HwidDeviceLimit != nil {
		deviceLimit = *user.HwidDeviceLimit
	}

	return user.ID, deviceLimit, nil
}

func (r *Client) GetUserTrafficInfo(ctx context.Context, telegramId int64) (*User, error) {
	users, err := r.getUsersByTelegramID(ctx, telegramId)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, ErrUserNotFound
	}

	user := findUserBySuffix(users, telegramId)
	return user, nil
}

// GetUserByID возвращает полную карточку пользователя панели GET /api/users/{userId}.
func (r *Client) GetUserByID(ctx context.Context, userID int64) (*User, error) {
	if userID <= 0 {
		return nil, ErrUserNotFound
	}
	var resp apiResponse[User]
	path := "/api/users/" + strconv.FormatInt(userID, 10)
	if err := r.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Response, nil
}

// ExtendUserDays продлевает подписку на N дней силами панели
// (POST /api/users/{userId}/actions/extend, появился в 3.0.0).
//
// Отличие от PATCH-пути: панель считает новый expireAt сама, поэтому нет гонки
// read-modify-write, когда бот и кабинет продлевают одного клиента одновременно.
func (r *Client) ExtendUserDays(ctx context.Context, userID int64, days int) (*User, error) {
	if userID <= 0 {
		return nil, ErrUserNotFound
	}
	if days <= 0 {
		return nil, fmt.Errorf("invalid days: %d", days)
	}
	var resp apiResponse[User]
	path := fmt.Sprintf("/api/users/%d/actions/extend", userID)
	if err := r.doJSON(ctx, http.MethodPost, path, &extendUserRequest{Days: days}, &resp); err != nil {
		return nil, err
	}
	return &resp.Response, nil
}

// sanitizePatchStatus приводит статус к тому, что принимает PATCH /api/users.
//
// В Remnawave 3.x ответ панели может содержать ACTIVE, DISABLED, LIMITED или
// EXPIRED, но на запись принимаются только первые два: LIMITED и EXPIRED панель
// вычисляет сама и на PATCH отвечает 400 Validation failed.
//
// Вызывающие (админка бота и кабинета) часто читают карточку и отправляют её
// обратно вместе с правкой тега/лимитов/сквадов — вместе с текущим статусом.
// Без этой нормализации любая такая правка падала бы у клиента с превышенным
// трафиком или истёкшей подпиской, то есть ровно у тех, кого правят чаще всего.
//
// Невалидный статус вычищается, а не подменяется на ACTIVE: смена тега не должна
// молча снимать блокировку. Пустое поле не уходит в JSON (omitempty) —
// статус остаётся тем, что вычислила панель.
func sanitizePatchStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "ACTIVE":
		return "ACTIVE"
	case "DISABLED":
		return "DISABLED"
	default:
		return ""
	}
}

// PatchUser применяет PATCH /api/users (тело UpdateUserRequest).
func (r *Client) PatchUser(ctx context.Context, req *UpdateUserRequest) (*User, error) {
	if req != nil {
		req.Status = sanitizePatchStatus(req.Status)
	}
	var resp apiResponse[User]
	if err := r.doJSON(ctx, http.MethodPatch, "/api/users", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Response, nil
}

// DeleteUser удаляет пользователя в панели DELETE /api/users/{userId}.
// В 3.0.0 эндпоинт отвечает 204 без тела — это обрабатывает doJSON.
func (r *Client) DeleteUser(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return errors.New("invalid user id")
	}
	return r.doJSON(ctx, http.MethodDelete, "/api/users/"+strconv.FormatInt(userID, 10), nil, nil)
}

// GetUserDevices возвращает HWID-устройства пользователя
// GET /api/hwid/devices/{userId} (был /{userUuid} до 3.0.0).
func (r *Client) GetUserDevices(ctx context.Context, userID int64) ([]Device, error) {
	if userID <= 0 {
		return nil, ErrUserNotFound
	}
	var resp getUserDevicesResponse
	path := "/api/hwid/devices/" + strconv.FormatInt(userID, 10)
	if err := r.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Response.Devices, nil
}

// DeleteUserDevice отвязывает устройство POST /api/hwid/devices/delete.
// Тело в 3.0.0 принимает userId вместо userUuid.
func (r *Client) DeleteUserDevice(ctx context.Context, userID int64, hwid string) error {
	if userID <= 0 {
		return ErrUserNotFound
	}
	req := &deleteUserDeviceRequest{
		Hwid:   hwid,
		UserID: userID,
	}
	return r.doJSON(ctx, http.MethodPost, "/api/hwid/devices/delete", req, nil)
}

// RevokeUserSubscription выдаёт пользователю новую ссылку подписки
// POST /api/users/{userId}/actions/revoke.
//
// Тело пустое намеренно: по умолчанию панель меняет и shortUuid (саму ссылку),
// и пароли/UUID внутри конфигов. Одной новой ссылки мало — у того, кто уже
// импортировал конфиги, они продолжали бы работать. Свой shortUuid не
// передаём: документация панели прямо советует отдать его генерацию ей.
func (r *Client) RevokeUserSubscription(ctx context.Context, userID int64) (*User, error) {
	if userID <= 0 {
		return nil, ErrUserNotFound
	}
	var resp apiResponse[User]
	path := fmt.Sprintf("/api/users/%d/actions/revoke", userID)
	if err := r.doJSON(ctx, http.MethodPost, path, &revokeUserSubscriptionRequest{}, &resp); err != nil {
		return nil, err
	}
	return &resp.Response, nil
}

// DeleteAllUserDevices отвязывает все HWID-устройства пользователя
// POST /api/hwid/devices/delete-all.
func (r *Client) DeleteAllUserDevices(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return ErrUserNotFound
	}
	return r.doJSON(ctx, http.MethodPost, "/api/hwid/devices/delete-all", &deleteAllUserDevicesRequest{UserID: userID}, nil)
}

// ResetUserTraffic обнуляет накопленный расход трафика у пользователя в панели; лимиты и стратегия сброса не меняются.
// POST /api/users/{userId}/actions/reset-traffic
func (r *Client) ResetUserTraffic(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return nil
	}
	path := fmt.Sprintf("/api/users/%d/actions/reset-traffic", userID)
	return r.doJSON(ctx, http.MethodPost, path, nil, nil)
}

func (r *Client) UpdateUserDeviceLimit(ctx context.Context, telegramId int64, newLimit int) (*User, error) {
	users, err := r.getUsersByTelegramID(ctx, telegramId)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, ErrUserNotFound
	}
	user := findUserBySuffix(users, telegramId)
	newLimit = r.pinnedHwidLimit(ctx, 0, telegramId, newLimit)
	if newLimit <= 0 {
		return nil, fmt.Errorf("invalid device limit: %d", newLimit)
	}

	req := &UpdateUserRequest{
		ID:              &user.ID,
		Status:          "ACTIVE",
		HwidDeviceLimit: &newLimit,
	}

	var resp apiResponse[User]
	if err := r.doJSON(ctx, http.MethodPatch, "/api/users", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Response, nil
}

// UpdateUserDeviceLimitByCustomer обновляет лимит устройств с учётом web-only fallback поиска.
func (r *Client) UpdateUserDeviceLimitByCustomer(ctx context.Context, customerID, telegramID int64, newLimit int) (*User, error) {
	user, err := r.findExistingUserForCustomer(ctx, customerID, telegramID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	newLimit = r.pinnedHwidLimit(ctx, customerID, telegramID, newLimit)
	if newLimit <= 0 {
		return nil, fmt.Errorf("invalid device limit: %d", newLimit)
	}

	req := &UpdateUserRequest{
		ID:              &user.ID,
		Status:          "ACTIVE",
		HwidDeviceLimit: &newLimit,
	}

	var resp apiResponse[User]
	if err := r.doJSON(ctx, http.MethodPatch, "/api/users", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Response, nil
}

// ---------------------------------------------------------------------------
// Utility functions
// ---------------------------------------------------------------------------

// Spec CreateUserRequestDto.username: pattern ^[a-zA-Z0-9_-]+$, 3..36 chars.
func generateUsername(customerId int64, telegramId int64) string {
	u := fmt.Sprintf("%d_%d", customerId, telegramId)
	if len(u) <= 36 {
		return u
	}
	h := sha256.Sum256([]byte(u))
	return "u_" + hex.EncodeToString(h[:16])
}

// isValidTag проверяет, соответствует ли тег формату ^[A-Z0-9_]+$
func isValidTag(tag string) bool {
	if tag == "" {
		return false
	}
	for _, char := range tag {
		if !((char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_') {
			return false
		}
	}
	return true
}

func getNewExpire(daysToAdd int, currentExpire time.Time) time.Time {
	if daysToAdd <= 0 {
		if currentExpire.AddDate(0, 0, daysToAdd).Before(time.Now()) {
			return time.Now().UTC().AddDate(0, 0, 1)
		}
		return currentExpire.AddDate(0, 0, daysToAdd)
	}

	if currentExpire.Before(time.Now().UTC()) || currentExpire.IsZero() {
		return time.Now().UTC().AddDate(0, 0, daysToAdd)
	}

	return currentExpire.AddDate(0, 0, daysToAdd)
}

func normalizeStrategy(s string) string {
	upper := strings.ToUpper(s)
	switch upper {
	case "DAY", "WEEK", "MONTH", "MONTH_ROLLING", "NO_RESET":
		return upper
	case "NEVER":
		return "NO_RESET"
	default:
		return "MONTH"
	}
}
