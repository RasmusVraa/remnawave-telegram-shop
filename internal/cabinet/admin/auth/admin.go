// Package adminauth проверяет, является ли аккаунт кабинета администратором.
//
// Логика: cabinet_identity(provider=telegram, unlinked_at IS NULL) →
// provider_user_id == ADMIN_TELEGRAM_ID. Без новых env — привязан ли Telegram
// аккаунта к тому же ID, что и в боте.
package adminauth

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/config"
)

// Checker проверяет admin-статус аккаунта.
type Checker struct {
	ids      *repository.IdentityRepo
	accounts *repository.AccountRepo
}

// NewChecker создаёт Checker.
// accounts может быть nil: тогда почтовый список не проверяется.
func NewChecker(ids *repository.IdentityRepo, accounts *repository.AccountRepo) *Checker {
	return &Checker{ids: ids, accounts: accounts}
}

// IsAdmin возвращает true, если у аккаунта есть привязанный Telegram identity
// с provider_user_id == ADMIN_TELEGRAM_ID.
func (c *Checker) IsAdmin(ctx context.Context, accountID int64) bool {
	if c.emailAdmin(ctx, accountID) {
		return true
	}
	_, ok := c.AdminTelegramID(ctx, accountID)
	return ok
}

// AdminTelegramID — Telegram id, под которым аккаунт признан админом. Нужен
// журналу «Мониторинга»: видно, кто из админов что сделал.
func (c *Checker) AdminTelegramID(ctx context.Context, accountID int64) (int64, bool) {
	if c == nil || c.ids == nil || len(config.GetAdminTelegramIds()) == 0 {
		return 0, false
	}

	ids, err := c.ids.ListLinkedByAccount(ctx, accountID)
	if err != nil {
		slog.Warn("admin checker: list identities", "account_id", accountID, "error", err)
		return 0, false
	}

	for _, id := range ids {
		if id.Provider != repository.ProviderTelegram {
			continue
		}
		s := strings.TrimSpace(id.ProviderUserID)
		if s == "" {
			continue
		}
		v, perr := strconv.ParseInt(s, 10, 64)
		if perr != nil {
			continue
		}
		if config.IsAdminTelegramID(v) {
			return v, true
		}
	}
	return 0, false
}

// emailAdmin — аккаунт с почтой из CABINET_ADMIN_EMAILS.
func (c *Checker) emailAdmin(ctx context.Context, accountID int64) bool {
	if c == nil || c.accounts == nil || accountID <= 0 {
		return false
	}
	allow := config.CabinetAdminEmails()
	if len(allow) == 0 {
		return false
	}
	acc, err := c.accounts.FindByID(ctx, accountID)
	if err != nil || acc == nil || acc.Email == nil {
		return false
	}
	email := strings.ToLower(strings.TrimSpace(*acc.Email))
	if email == "" {
		return false
	}
	for _, allowed := range allow {
		if email == allowed {
			return true
		}
	}
	return false
}
