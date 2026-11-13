// Package adminevents — журнал событий для страницы «Мониторинг» в админке и
// вкладки «История» в карточке клиента.
//
// Код бота и кабинета зовёт Emit там, где что-то произошло: оплата, вход,
// промокод, нода упала. Emit ничего не ждёт: событие уходит в очередь, фоновый
// писатель кладёт его в admin_event пачками. Если база тормозит и очередь полна,
// событие теряется с предупреждением в лог — оплата и бот из-за журнала не
// должны ни ждать, ни падать.
package adminevents

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"remnawave-tg-shop-bot/internal/config"
)

// Level — уровень события.
type Level int16

const (
	// LevelImportant — видно в ленте всегда.
	LevelImportant Level = 1
	// LevelBot — нажатия кнопок и команды в боте и прочие мелочи. Пишутся только
	// при MONITORING_BOT_ACTIONS=true и хранятся коротко.
	LevelBot Level = 2
)

// Источник события.
const (
	SourceBot     = "bot"
	SourceCabinet = "cabinet"
	SourcePanel   = "panel"
	SourceSystem  = "system"
)

// Типы событий. Строка уходит в БД и во фронт: переименование ломает историю.
const (
	UserRegistered = "user.registered"
	UserLogin      = "user.login"

	AccountLinked        = "account.linked"
	AccountMerged        = "account.merged"
	AccountDeleted       = "account.deleted"
	AccountEmailVerified = "account.email_verified"
	AccountPasswordReset = "account.password_reset"

	TrialActivated           = "trial.activated"
	PaymentSubscriptionNew   = "payment.subscription_new"
	PaymentSubscriptionRenew = "payment.subscription_renew"
	PaymentTariffUpgrade     = "payment.tariff_upgrade"
	PaymentExtraHwid         = "payment.extra_hwid"
	PaymentExtraTraffic      = "payment.extra_traffic"
	PaymentFailed            = "payment.failed"
	SubscriptionExpired      = "subscription.expired"
	SubscriptionLinkReset    = "subscription.link_reset"
	// SubscriptionExpiredDigest — сводка за сутки вместо россыпи «истекла» в общей ленте.
	SubscriptionExpiredDigest = "subscription.expired_digest"

	DeviceRemoved = "device.removed"
	TorrentReport = "torrent.report"

	PromoActivated       = "promo.activated"
	PromoInvalid         = "promo.invalid"
	FortuneSpin          = "fortune.spin"
	LoyaltyLevelUp       = "loyalty.level_up"
	ReferralBonus        = "referral.bonus"
	PartnerApplied       = "partner.applied"
	PartnerPayoutRequest = "partner.payout_requested"
	SupportMessage       = "support.message"
	SupportReply         = "support.reply"
	SupportClosed        = "support.closed"
	BotMessage           = "bot.message"
	BotCommand           = "bot.command"
	BotButton            = "bot.button"
	NodeDown             = "node.down"
	NodeUp               = "node.up"
	SystemBroadcast      = "system.broadcast"
	SystemSync           = "system.sync"
	AdminAction          = "admin.action"
)

// Категории — пункты фильтра ленты.
const (
	CategoryMoney   = "money"
	CategoryUsers   = "users"
	CategorySubs    = "subs"
	CategoryPromo   = "promo"
	CategorySupport = "support"
	CategoryNodes   = "nodes"
	CategoryAdmin   = "admin"
	// CategoryBot — нажатия в боте; в фильтре это не галочка, а отдельный переключатель.
	CategoryBot = "bot"
)

// Categories — порядок пунктов фильтра.
var Categories = []string{
	CategoryMoney, CategoryUsers, CategorySubs, CategoryPromo,
	CategorySupport, CategoryNodes, CategoryAdmin,
}

var typeCategory = map[string]string{
	UserRegistered: CategoryUsers, UserLogin: CategoryUsers,
	AccountLinked: CategoryUsers, AccountMerged: CategoryUsers, AccountDeleted: CategoryUsers,
	AccountEmailVerified: CategoryUsers, AccountPasswordReset: CategoryUsers,

	PaymentSubscriptionNew: CategoryMoney, PaymentSubscriptionRenew: CategoryMoney,
	PaymentTariffUpgrade: CategoryMoney, PaymentExtraHwid: CategoryMoney,
	PaymentExtraTraffic: CategoryMoney, PaymentFailed: CategoryMoney,

	TrialActivated: CategorySubs, SubscriptionExpired: CategorySubs, SubscriptionLinkReset: CategorySubs,
	SubscriptionExpiredDigest: CategorySubs,
	DeviceRemoved:             CategorySubs, TorrentReport: CategorySubs,

	PromoActivated: CategoryPromo, PromoInvalid: CategoryPromo, FortuneSpin: CategoryPromo,
	LoyaltyLevelUp: CategoryPromo, ReferralBonus: CategoryPromo,
	PartnerApplied: CategoryPromo, PartnerPayoutRequest: CategoryPromo,

	SupportMessage: CategorySupport, SupportReply: CategorySupport, SupportClosed: CategorySupport,
	BotMessage: CategorySupport,

	NodeDown: CategoryNodes, NodeUp: CategoryNodes,

	SystemBroadcast: CategoryAdmin, SystemSync: CategoryAdmin, AdminAction: CategoryAdmin,

	BotCommand: CategoryBot, BotButton: CategoryBot,
}

// CategoryOf — категория типа. Неизвестный тип — «admin»: так новое событие,
// которому забыли завести категорию, всё равно видно в ленте.
func CategoryOf(eventType string) string {
	if c, ok := typeCategory[eventType]; ok {
		return c
	}
	return CategoryAdmin
}

// TypesOf — все типы категории.
func TypesOf(category string) []string {
	var out []string
	for t, c := range typeCategory {
		if c == category {
			out = append(out, t)
		}
	}
	return out
}

// noisyTypes — частые события, которые хранятся MONITORING_RETENTION_NOISY_DAYS.
var noisyTypes = []string{UserLogin, SupportMessage, SupportReply, SupportClosed, BotMessage, TorrentReport}

// PaidTypes — успешные оплаты: по ним считается «Оплаты» в счётчиках.
var PaidTypes = []string{
	PaymentSubscriptionNew, PaymentSubscriptionRenew, PaymentTariffUpgrade,
	PaymentExtraHwid, PaymentExtraTraffic,
}

// Event — одно событие. Payload — то, что фронт покажет в строке; без секретов,
// ссылок подписки и полного текста сообщений (только Preview).
type Event struct {
	Type       string
	Level      Level
	CustomerID int64 // 0 — событие не про клиента
	// AccountID — аккаунт кабинета, если клиент по нему ещё не известен вызывающему.
	// Клиента найдёт запись по cabinet_account_customer_link.
	AccountID int64
	// TelegramID — клиент по telegram id: бот знает его, не загружая клиента.
	TelegramID int64
	ActorTgID  int64 // кто из админов; 0 — не админ
	Source     string
	Payload    map[string]any
	At         time.Time
}

// maxPayloadBytes — потолок JSON payload. Больше — значит, кто-то положил туда
// лишнего; строку всё равно пишем, но без содержимого.
const maxPayloadBytes = 2048

func (e Event) payloadJSON() []byte {
	if len(e.Payload) == 0 {
		return []byte("{}")
	}
	b, err := json.Marshal(e.Payload)
	if err != nil || len(b) > maxPayloadBytes {
		return []byte(`{"truncated":true}`)
	}
	// Postgres не принимает \u0000 в jsonb, и одна такая строка уронила бы всю
	// пачку — вместе с оплатами. Нулевой символ в тексте смысла не несёт.
	return bytes.ReplaceAll(b, []byte(`\u0000`), nil)
}

// Preview обрезает текст до n символов и схлопывает переводы строк — для превью
// сообщений в ленте.
func Preview(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n])) + "…"
}

var defaultWriter atomic.Pointer[Writer]

// SetDefault делает писателя общим для процесса. Без него Emit ничего не делает —
// так живут тесты и утилиты.
func SetDefault(w *Writer) { defaultWriter.Store(w) }

// Emit отправляет событие в журнал, не дожидаясь записи.
func Emit(e Event) {
	w := defaultWriter.Load()
	if w == nil {
		return
	}
	w.Emit(e)
}

type sourceKey struct{}

// WithSource помечает контекст источником события — для мест, где одна и та же
// функция вызывается и из бота, и из кабинета (создание клиента).
func WithSource(ctx context.Context, source string) context.Context {
	return context.WithValue(ctx, sourceKey{}, source)
}

// SourceFrom — источник из контекста или def.
func SourceFrom(ctx context.Context, def string) string {
	if v, ok := ctx.Value(sourceKey{}).(string); ok && v != "" {
		return v
	}
	return def
}

// botActionsEnabled — подменяется в тестах.
var botActionsEnabled = config.MonitoringBotActions
