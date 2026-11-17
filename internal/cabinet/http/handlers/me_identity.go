package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/cabinet/avatartoken"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"
)

// identityProfile — «кто вошёл» для шапки профиля: имя, ник и картинка.
//
// Приоритет — Telegram: если он привязан, показываем его данные. К данным
// OAuth-провайдера спускаемся отдельно по каждому полю, а не по блоку целиком:
// когда Telegram привязан, но бот не смог отдать имя (человек не запускал
// бота), лучше подписать карточку именем из Google, чем «Аккаунтом №1274».
type identityProfile struct {
	DisplayName string `json:"display_name,omitempty"`
	// Username — ник Telegram без «@».
	Username string `json:"username,omitempty"`
	// AvatarURL — свой /cabinet/api/avatar для Telegram, прямая ссылка
	// провайдера для остальных. Пусто — рисуем инициалы.
	AvatarURL string `json:"avatar_url,omitempty"`
	// Provider — главный способ входа (telegram|google|yandex|vk): по нему UI
	// ставит бейдж на аватарке. Это именно способ входа, а не источник
	// картинки: они расходятся, когда Telegram привязан, но аватарки в нём нет.
	Provider string `json:"identity_provider,omitempty"`
}

// pickTelegramID — telegram id аккаунта. Источник истины для бота —
// customer.telegram_id, но подменять им первую identity можно только если
// такая привязка у аккаунта действительно есть: иначе после слияния аккаунтов
// показали бы чужой id.
func pickTelegramID(identityIDs []int64, cust *database.Customer) *int64 {
	var out *int64
	if len(identityIDs) > 0 {
		v := identityIDs[0]
		out = &v
	}
	if cust == nil || cust.IsWebOnly || utils.IsSyntheticTelegramID(cust.TelegramID) {
		return out
	}
	v := cust.TelegramID
	if out == nil {
		return &v
	}
	if *out == v {
		return out
	}
	for _, id := range identityIDs {
		if id == v {
			return &v
		}
	}
	return out
}

// ResolveForReview — имя и аватар для сохранения отзыва (снимок на момент отправки).
func (h *MeHandler) ResolveForReview(r *http.Request, accountID int64) (displayName, avatarURL string) {
	if h == nil || r == nil {
		return "", ""
	}
	ctx := r.Context()
	var ids []repository.Identity
	if h.ids != nil {
		if list, err := h.ids.ListByAccount(ctx, accountID); err == nil {
			ids = list
		}
	}
	var cust *database.Customer
	var telegramID *int64
	if h.links != nil && h.customers != nil {
		if link, err := h.links.FindByAccountID(ctx, accountID); err == nil && link != nil {
			if c, err := h.customers.FindById(ctx, link.CustomerID); err == nil {
				cust = c
				if c != nil && c.TelegramID != 0 {
					tid := c.TelegramID
					telegramID = &tid
				}
			}
		}
	}
	p := h.resolveIdentityProfile(ctx, accountID, ids, cust, telegramID)
	name := strings.TrimSpace(p.DisplayName)
	if name == "" {
		name = strings.TrimSpace(p.Username)
	}
	return name, strings.TrimSpace(p.AvatarURL)
}

// resolveIdentityProfile собирает шапку профиля. Ошибки Telegram сюда не
// протекают: не ответил — просто получится профиль победнее.
func (h *MeHandler) resolveIdentityProfile(
	ctx context.Context,
	accountID int64,
	ids []repository.Identity,
	cust *database.Customer,
	telegramID *int64,
) identityProfile {
	var out identityProfile

	if telegramID != nil {
		out.Provider = repository.ProviderTelegram
		if p, ok := h.tgProfiles.Profile(ctx, *telegramID); ok {
			out.DisplayName = p.DisplayName()
			out.Username = p.Username
			if p.HasPhoto() {
				out.AvatarURL = h.avatarURL(accountID)
			}
		}
		if out.Username == "" && cust != nil && cust.TelegramUsername != nil {
			// Бот пишет ник в customer при каждом /start — годится, когда
			// getChat не ответил.
			out.Username = strings.TrimPrefix(strings.TrimSpace(*cust.TelegramUsername), "@")
		}
		if out.DisplayName == "" {
			out.DisplayName = telegramNameFromIdentity(ids)
		}
		if out.DisplayName != "" && out.AvatarURL != "" {
			return out
		}
	}

	// Ниже — OAuth. Он же основной источник для тех, у кого Telegram не
	// привязан вовсе: имя и картинку провайдеры отдают в userinfo, и весь
	// ответ целиком лежит в raw_profile_json с момента входа.
	name, avatar, source := oauthNameAndAvatar(ids)
	if out.DisplayName == "" {
		out.DisplayName = name
	}
	if out.AvatarURL == "" {
		out.AvatarURL = avatar
	}
	if out.Provider == "" {
		out.Provider = source
	}
	return out
}

// avatarURL — подписанная ссылка на свою же отдачу аватарки.
func (h *MeHandler) avatarURL(accountID int64) string {
	if len(h.avatarSecret) == 0 {
		return ""
	}
	token, err := avatartoken.Issue(h.avatarSecret, accountID, avatartoken.DefaultTTL, time.Now())
	if err != nil {
		return ""
	}
	return "/cabinet/api/avatar?t=" + url.QueryEscape(token)
}

// telegramNameFromIdentity — имя из сохранённого payload логина. Раньше при
// создании identity писались только id и username, так что у старых аккаунтов
// поля не будет; это запасной путь, а не основной.
func telegramNameFromIdentity(ids []repository.Identity) string {
	for _, id := range ids {
		if id.Provider != repository.ProviderTelegram || len(id.RawProfileJSON) == 0 {
			continue
		}
		var raw struct {
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
		}
		if err := json.Unmarshal(id.RawProfileJSON, &raw); err != nil {
			continue
		}
		if name := strings.TrimSpace(raw.FirstName + " " + raw.LastName); name != "" {
			return name
		}
	}
	return ""
}

// oauthNameAndAvatar — имя и аватарка из userinfo провайдера. Порядок фиксиран
// (Google, Яндекс, VK), чтобы у аккаунта с несколькими привязками картинка не
// прыгала от запроса к запросу.
func oauthNameAndAvatar(ids []repository.Identity) (name, avatar, source string) {
	for _, provider := range []string{repository.ProviderGoogle, repository.ProviderYandex, repository.ProviderVK} {
		for _, id := range ids {
			if id.Provider != provider || len(id.RawProfileJSON) == 0 {
				continue
			}
			n, a := parseOAuthProfile(provider, id.RawProfileJSON)
			if n == "" && a == "" {
				continue
			}
			return n, a, provider
		}
	}
	return "", "", ""
}

func parseOAuthProfile(provider string, rawJSON []byte) (name, avatar string) {
	var raw struct {
		Name        string `json:"name"`         // google
		Picture     string `json:"picture"`      // google
		DisplayName string `json:"display_name"` // yandex
		RealName    string `json:"real_name"`    // yandex
		AvatarID    string `json:"default_avatar_id"`
		FirstName   string `json:"first_name"` // vk
		LastName    string `json:"last_name"`  // vk
		Photo200    string `json:"photo_200"`  // vk
	}
	if err := json.Unmarshal(rawJSON, &raw); err != nil {
		return "", ""
	}
	switch provider {
	case repository.ProviderGoogle:
		return strings.TrimSpace(raw.Name), httpsOnly(raw.Picture)
	case repository.ProviderYandex:
		name = strings.TrimSpace(raw.DisplayName)
		if name == "" {
			name = strings.TrimSpace(raw.RealName)
		}
		if id := strings.TrimSpace(raw.AvatarID); id != "" && !strings.ContainsAny(id, "/?&#") {
			// Яндекс отдаёт не ссылку, а идентификатор картинки; islands-200 —
			// квадрат 200×200, ровно под аватарку.
			avatar = "https://avatars.yandex.net/get-yapic/" + id + "/islands-200"
		}
		return name, avatar
	case repository.ProviderVK:
		return strings.TrimSpace(raw.FirstName + " " + raw.LastName), httpsOnly(raw.Photo200)
	}
	return "", ""
}

// httpsOnly отсеивает всё, что не является https-ссылкой: значение приезжает
// от стороннего сервиса и попадает в src картинки.
func httpsOnly(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	return u.String()
}
