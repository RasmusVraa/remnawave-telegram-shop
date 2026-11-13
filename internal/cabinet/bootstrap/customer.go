// Package bootstrap — создание «теневых» ресурсов для нового кабинет-аккаунта.
//
// Сейчас здесь только CustomerBootstrap — гарантирует, что у каждого
// cabinet_account есть привязанный customer (web-only). Отдельный пакет, а не
// часть auth/service, чтобы:
//
//   - на Этапе 4 (merge) тот же сервис вызывался из link-сервиса;
//   - избежать цикла импорта auth/service ↔ bootstrap (в merge-сервис auth/service
//     импортировать не потребуется, если логика bootstrap'а живёт отдельно).
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"remnawave-tg-shop-bot/internal/adminevents"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"
)

// CustomerBootstrap — сервис, создающий web-only customer + link на аккаунт.
type CustomerBootstrap struct {
	customerRepo *database.CustomerRepository
	linkRepo     *repository.AccountCustomerLinkRepo
	accountRepo  *repository.AccountRepo      // опционально: предпроверка «аккаунт жив»
	referralRepo *database.ReferralRepository // опционально: регистрация по ?ref=
	partnerRepo  *database.PartnerRepository  // опционально: регистрация по ?p=
}

// NewCustomerBootstrap — конструктор. referralRepo может быть nil — тогда AttachReferralAfterWebRegister не создаёт строки.
// accountRepo тоже может быть nil: тогда удалённый аккаунт отловится не предпроверкой,
// а guarded-insert'ом в linkRepo.Create — ценой одной строки-сироты в customer.
func NewCustomerBootstrap(customerRepo *database.CustomerRepository, linkRepo *repository.AccountCustomerLinkRepo, accountRepo *repository.AccountRepo, referralRepo *database.ReferralRepository, partnerRepo *database.PartnerRepository) *CustomerBootstrap {
	return &CustomerBootstrap{customerRepo: customerRepo, linkRepo: linkRepo, accountRepo: accountRepo, referralRepo: referralRepo, partnerRepo: partnerRepo}
}

// ensureAccountAlive — предпроверка перед созданием customer'а. Возвращает
// ErrAccountGone, если строки cabinet_account уже нет. Ошибки самой проверки не
// фатальны: пропускаем дальше, guarded-insert в linkRepo.Create поймает удаление.
func (b *CustomerBootstrap) ensureAccountAlive(ctx context.Context, accountID int64) error {
	if b.accountRepo == nil {
		return nil
	}
	alive, err := b.accountRepo.Exists(ctx, accountID)
	if err != nil {
		slog.Warn("bootstrap: account exists check failed", "account_id", accountID, "error", err)
		return nil
	}
	if !alive {
		return ErrAccountGone
	}
	return nil
}

// mapLinkCreateErr переводит ErrAccountMissing из репозитория в ErrAccountGone,
// чтобы HTTP-слой отличал «аккаунт удалён» от прочих сбоев вставки.
func mapLinkCreateErr(err error) error {
	if errors.Is(err, repository.ErrAccountMissing) {
		return ErrAccountGone
	}
	return err
}

// EnsureForAccount гарантирует, что у данного cabinet_account есть link на
// customer с is_web_only=TRUE. Идемпотентно: при повторном вызове возвращает
// существующий link без побочных эффектов.
//
// Поведение при сбоях:
//
//   - если customer был создан, но link — нет (процесс упал между этими шагами),
//     следующий вызов ON CONFLICT вернёт тот же customer и создаст link;
//   - при двух одновременных вызовах (гонка Register/Login) UNIQUE account_id
//     на link'е вызовет конфликт во втором; мы ловим ошибку и перечитываем.
//
// telegram_id генерируется из utils.SyntheticTelegramID — это единственная
// точка генерации synthetic id в проекте (см. mvp-tz.md 7.3).
func (b *CustomerBootstrap) EnsureForAccount(ctx context.Context, accountID int64, language string) (*repository.AccountCustomerLink, error) {
	if accountID <= 0 {
		return nil, fmt.Errorf("bootstrap: invalid account_id %d", accountID)
	}

	if link, err := b.linkRepo.FindByAccountID(ctx, accountID); err == nil {
		return link, nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("bootstrap: find link: %w", err)
	}

	if err := b.ensureAccountAlive(ctx, accountID); err != nil {
		return nil, err
	}

	telegramID := utils.SyntheticTelegramID(accountID)
	customer, err := b.customerRepo.CreateWebOnly(ctx, telegramID, language)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: create customer: %w", err)
	}

	link, err := b.linkRepo.Create(ctx, accountID, customer.ID, repository.LinkStatusLinked)
	if err != nil {
		if link2, err2 := b.linkRepo.FindByAccountID(ctx, accountID); err2 == nil {
			return link2, nil
		}
		if mapped := mapLinkCreateErr(err); errors.Is(mapped, ErrAccountGone) {
			return nil, mapped
		}
		return nil, fmt.Errorf("bootstrap: create link: %w", err)
	}

	slog.Info("cabinet: web-only customer bootstrapped",
		"account_id", accountID,
		"customer_id", customer.ID,
		"telegram_id_masked", utils.MaskHalfInt64(telegramID),
	)
	return link, nil
}

// EnsureForAccountTelegram — как EnsureForAccount, но при входе через Telegram:
// если в БД бота уже есть customer с этим реальным telegram_id и без связи с
// кабинетом — создаём link на него (подписка/покупки из бота становятся видны).
// Если у аккаунта уже висит web-only synthetic customer — перепривязываем link
// на ботового customer (тот же сценарий после ошибочного bootstrap).
func (b *CustomerBootstrap) EnsureForAccountTelegram(ctx context.Context, accountID int64, telegramUserID int64, language string) (*repository.AccountCustomerLink, error) {
	if accountID <= 0 {
		return nil, fmt.Errorf("bootstrap: invalid account_id %d", accountID)
	}
	if telegramUserID <= 0 || utils.IsSyntheticTelegramID(telegramUserID) {
		return b.EnsureForAccount(ctx, accountID, language)
	}

	link, err := b.linkRepo.FindByAccountID(ctx, accountID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("bootstrap: find link: %w", err)
	}
	if err == nil {
		return b.reconcileTelegramLink(ctx, accountID, telegramUserID, link)
	}

	if err := b.ensureAccountAlive(ctx, accountID); err != nil {
		return nil, err
	}

	// Нет link: попробовать привязать существующего ботового customer.
	botCust, err := b.customerRepo.FindByTelegramId(ctx, telegramUserID)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: find by telegram: %w", err)
	}
	if botCust == nil {
		// Telegram-first login: создаём customer сразу с реальным telegram_id,
		// чтобы не оставлять synthetic id для живого Telegram-пользователя.
		created, err := b.customerRepo.Create(adminevents.WithSource(ctx, adminevents.SourceCabinet), &database.Customer{
			TelegramID: telegramUserID,
			Language:   language,
		})
		if err != nil {
			return nil, fmt.Errorf("bootstrap: create telegram customer: %w", err)
		}
		newLink, err := b.linkRepo.Create(ctx, accountID, created.ID, repository.LinkStatusLinked)
		if err != nil {
			if link2, err2 := b.linkRepo.FindByAccountID(ctx, accountID); err2 == nil {
				return link2, nil
			}
			if mapped := mapLinkCreateErr(err); errors.Is(mapped, ErrAccountGone) {
				return nil, mapped
			}
			return nil, fmt.Errorf("bootstrap: create link to telegram customer: %w", err)
		}
		slog.Info("cabinet: created telegram customer for cabinet account",
			"account_id", accountID,
			"customer_id", created.ID,
			"telegram_id_masked", utils.MaskHalfInt64(telegramUserID),
		)
		return newLink, nil
	}
	other, err := b.linkRepo.FindByCustomerID(ctx, botCust.ID)
	if err == nil && other != nil {
		if other.AccountID == accountID {
			return other, nil
		}
		return nil, ErrTelegramCustomerLinkedElsewhere
	}
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("bootstrap: find link by customer: %w", err)
	}

	newLink, err := b.linkRepo.Create(ctx, accountID, botCust.ID, repository.LinkStatusLinked)
	if err != nil {
		if link2, err2 := b.linkRepo.FindByAccountID(ctx, accountID); err2 == nil {
			return link2, nil
		}
		if mapped := mapLinkCreateErr(err); errors.Is(mapped, ErrAccountGone) {
			return nil, mapped
		}
		return nil, fmt.Errorf("bootstrap: create link to bot customer: %w", err)
	}
	slog.Info("cabinet: linked existing bot customer to cabinet account",
		"account_id", accountID,
		"customer_id", botCust.ID,
		"telegram_id_masked", utils.MaskHalfInt64(telegramUserID),
	)
	return newLink, nil
}

func (b *CustomerBootstrap) reconcileTelegramLink(
	ctx context.Context,
	accountID int64,
	telegramUserID int64,
	link *repository.AccountCustomerLink,
) (*repository.AccountCustomerLink, error) {
	cur, err := b.customerRepo.FindById(ctx, link.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: load linked customer: %w", err)
	}
	if cur == nil {
		return nil, fmt.Errorf("bootstrap: linked customer %d missing", link.CustomerID)
	}
	if cur.TelegramID == telegramUserID {
		return link, nil
	}

	// Уже привязан «живой» другой telegram_id — не перетираем молча.
	if !cur.IsWebOnly || !utils.IsSyntheticTelegramID(cur.TelegramID) {
		return link, nil
	}

	botCust, err := b.customerRepo.FindByTelegramId(ctx, telegramUserID)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: find bot customer: %w", err)
	}
	if botCust == nil {
		return link, nil
	}
	other, err := b.linkRepo.FindByCustomerID(ctx, botCust.ID)
	if err == nil && other != nil {
		if other.AccountID != accountID {
			return nil, ErrTelegramCustomerLinkedElsewhere
		}
		return link, nil
	}
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("bootstrap: find link for bot customer: %w", err)
	}

	if err := b.linkRepo.UpdateCustomerID(ctx, accountID, botCust.ID); err != nil {
		return nil, fmt.Errorf("bootstrap: relink synthetic to bot customer: %w", err)
	}
	slog.Info("cabinet: relinked cabinet account from synthetic to bot customer",
		"account_id", accountID,
		"old_customer_id", cur.ID,
		"new_customer_id", botCust.ID,
		"telegram_id_masked", utils.MaskHalfInt64(telegramUserID),
	)
	return b.linkRepo.FindByAccountID(ctx, accountID)
}
