package payment

import "context"

type notifyCtxKey int

const (
	notifyCtxKeyStars notifyCtxKey = iota + 1
	notifyCtxKeyCrypto
)

// StarsNotifyMeta — поля SuccessfulPayment (Stars), без записи в БД; только для группового уведомления.
type StarsNotifyMeta struct {
	TelegramPaymentChargeID string
	ProviderPaymentChargeID string
	TotalAmount             int
	Currency                string
}

// CryptoNotifyMeta — фрагмент ответа CryptoPay API по счёту; только для уведомления.
type CryptoNotifyMeta struct {
	Hash, Status, CurrencyType, Asset, PaidAsset, PaidAmount string
	PayUrl, BotInvoiceUrl, FeeAmount                         string
}

// WithStarsNotifyMeta кладёт метаданные Stars в ctx (перед ProcessPurchaseById).
func WithStarsNotifyMeta(ctx context.Context, m StarsNotifyMeta) context.Context {
	return context.WithValue(ctx, notifyCtxKeyStars, m)
}

// StarsNotifyMetaFromCtx читает метаданные Stars из ctx.
func StarsNotifyMetaFromCtx(ctx context.Context) (StarsNotifyMeta, bool) {
	v, ok := ctx.Value(notifyCtxKeyStars).(StarsNotifyMeta)
	return v, ok
}

// WithCryptoNotifyMeta кладёт метаданные CryptoPay в ctx (перед ProcessPurchaseById).
func WithCryptoNotifyMeta(ctx context.Context, m CryptoNotifyMeta) context.Context {
	return context.WithValue(ctx, notifyCtxKeyCrypto, m)
}

// CryptoNotifyMetaFromCtx читает метаданные CryptoPay из ctx.
func CryptoNotifyMetaFromCtx(ctx context.Context) (CryptoNotifyMeta, bool) {
	v, ok := ctx.Value(notifyCtxKeyCrypto).(CryptoNotifyMeta)
	return v, ok
}
