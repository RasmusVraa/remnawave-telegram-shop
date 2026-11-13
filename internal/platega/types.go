package platega

type PaymentMethod int

// Коды способов оплаты — как в OpenAPI Platega (https://docs.platega.io): 2 СБП, 3 ЕРИП, 11 карточный эквайринг, 12 международная, 13 крипта.
// Раньше в форках использовалось 10 «CardsRUB» — текущий API его не принимает (400 VAL_0001).
const (
	PaymentMethodSBPQR                  PaymentMethod = 2
	PaymentMethodERIP                   PaymentMethod = 3
	PaymentMethodCardAcquiring          PaymentMethod = 11
	PaymentMethodInternationalAcquiring PaymentMethod = 12
	PaymentMethodCrypto                 PaymentMethod = 13
)

func (m PaymentMethod) String() string {
	switch m {
	case PaymentMethodSBPQR:
		return "SBPQR"
	case PaymentMethodERIP:
		return "ERIP"
	case PaymentMethodCardAcquiring:
		return "CardAcquiring"
	case PaymentMethodInternationalAcquiring:
		return "InternationalAcquiring"
	case PaymentMethodCrypto:
		return "Crypto"
	default:
		return "Unknown"
	}
}

type PaymentStatus string

const (
	StatusPending      PaymentStatus = "PENDING"
	StatusCanceled     PaymentStatus = "CANCELED"
	StatusConfirmed    PaymentStatus = "CONFIRMED"
	StatusChargebacked PaymentStatus = "CHARGEBACKED"
)

func (s PaymentStatus) IsTerminal() bool {
	return s == StatusCanceled || s == StatusConfirmed || s == StatusChargebacked
}

func (s PaymentStatus) IsSuccess() bool {
	return s == StatusConfirmed
}

type PaymentDetails struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

type CreateTransactionRequest struct {
	// PaymentMethod 0 не отправляется: общая форма, способ выбирает плательщик.
	PaymentMethod  PaymentMethod  `json:"paymentMethod,omitempty"`
	PaymentDetails PaymentDetails `json:"paymentDetails"`
	Description    string         `json:"description"`
	Return         string         `json:"return"`
	FailedUrl      string         `json:"failedUrl"`
	Payload        string         `json:"payload,omitempty"`
}

type CreateTransactionResponse struct {
	PaymentMethod  string        `json:"paymentMethod"`
	TransactionId  string        `json:"transactionId"`
	Redirect       string        `json:"redirect"`
	URL            string        `json:"url"`
	Return         string        `json:"return"`
	PaymentDetails any           `json:"paymentDetails"`
	Status         PaymentStatus `json:"status"`
	ExpiresIn      string        `json:"expiresIn"`
	MerchantId     string        `json:"merchantId"`
	UsdtRate       float64       `json:"usdtRate,omitempty"`
}

// PayURL — ссылка на оплату: v2 отдаёт её в url, прежняя версия — в redirect.
func (r *CreateTransactionResponse) PayURL() string {
	if r.URL != "" {
		return r.URL
	}
	return r.Redirect
}

type TransactionStatusResponse struct {
	Id                string         `json:"id"`
	Status            PaymentStatus  `json:"status"`
	PaymentDetails    PaymentDetails `json:"paymentDetails"`
	MerchantName      string         `json:"merchantName"`
	MerchantId        string         `json:"mechantId"`
	Commission        float64        `json:"comission"`
	PaymentMethod     string         `json:"paymentMethod"`
	ExpiresIn         string         `json:"expiresIn"`
	Return            string         `json:"return"`
	CommissionUsdt    float64        `json:"comissionUsdt"`
	AmountUsdt        float64        `json:"amountUsdt"`
	QR                string         `json:"qr"`
	PayformSuccessUrl string         `json:"payformSuccessUrl"`
	Payload           string         `json:"payload"`
	CommissionType    int            `json:"comissionType"`
	ExternalId        string         `json:"externalId"`
	Description       string         `json:"description"`
}

type CallbackPayload struct {
	Id            string        `json:"id"`
	Amount        float64       `json:"amount"`
	Currency      string        `json:"currency"`
	Status        PaymentStatus `json:"status"`
	PaymentMethod int           `json:"paymentMethod"`
	Payload       string        `json:"payload"`
}
