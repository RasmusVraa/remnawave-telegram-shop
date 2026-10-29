// Package connectinvite выпускает и проверяет короткие подписанные токены
// приглашения «подключить ещё одно устройство».
//
// Зачем отдельный токен, а не сама ссылка подписки в URL: приглашение уезжает
// в мессенджер и попадает в QR. Ссылка подписки — секрет длиной ~60 символов,
// а зашифрованный deep link Happ (RSA-4096) — под 700; QR от такой строки
// вырождается в плотную сетку, которую камера читает через раз. Токен же —
// 34 символа, QR получается крупный и надёжный.
//
// Токен stateless: account_id и срок зашиты внутрь и закрыты HMAC, отдельной
// таблицы под приглашения нет. Отзыв конкретного приглашения поэтому
// невозможен — вместо него короткий TTL (см. DefaultTTL). Разом отзываются
// все приглашения аккаунта: при сбросе ссылки подписки (см. IssuedBefore).
//
// Модель угроз: токен обменивается на deep link подписки, то есть по сути
// равен самой подписке. Он и создан, чтобы им делились с другим устройством,
// поэтому «утечка» здесь не инцидент, а сценарий. Ограничивают ущерб лимит
// устройств (HWID) и срок жизни приглашения.
package connectinvite

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"time"
)

// DefaultTTL — срок жизни приглашения. Трое суток: получателю хватает
// поставить приложение не в тот же вечер, а забытая в переписке ссылка
// перестаёт работать без участия владельца. Чем короче срок, тем меньше
// стоит утечка приглашения — отзыва у stateless-токена нет.
const DefaultTTL = 3 * 24 * time.Hour

const (
	tokenVersion = 1

	macLen     = 12 // 96 бит: подделка нереальна, а токен остаётся коротким
	payloadLen = 1 + 8 + 4
	tokenLen   = payloadLen + macLen
)

// keyContext разводит ключ приглашений и ключ подписи JWT: секрет один, но
// токен приглашения не должен проверяться как access-токен и наоборот.
const keyContext = "cabinet-connect-invite-v1"

var (
	// ErrMalformed — строка не является токеном приглашения.
	ErrMalformed = errors.New("connectinvite: malformed token")
	// ErrSignature — подпись не сходится (подделка или смена секрета).
	ErrSignature = errors.New("connectinvite: bad signature")
	// ErrExpired — срок приглашения истёк.
	ErrExpired = errors.New("connectinvite: token expired")
)

var encoding = base64.RawURLEncoding

// deriveKey — HMAC-ключ приглашений, выведенный из секрета кабинета.
func deriveKey(secret []byte) []byte {
	sum := sha256.Sum256(append([]byte(keyContext), secret...))
	return sum[:]
}

// Issue выпускает токен приглашения для аккаунта. expiresAt возвращается,
// чтобы UI мог показать срок, не разбирая токен.
func Issue(secret []byte, accountID int64, ttl time.Duration, now time.Time) (token string, expiresAt time.Time, err error) {
	if len(secret) == 0 {
		return "", time.Time{}, errors.New("connectinvite: empty secret")
	}
	if accountID <= 0 {
		return "", time.Time{}, errors.New("connectinvite: invalid account id")
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	expiresAt = now.Add(ttl).Truncate(time.Second)
	// Unix-секунды в uint32 хватает до 2106 года; срок здесь — неделя.
	if expiresAt.Unix() <= 0 || expiresAt.Unix() > int64(^uint32(0)) {
		return "", time.Time{}, errors.New("connectinvite: expiry out of range")
	}

	raw := make([]byte, tokenLen)
	raw[0] = tokenVersion
	binary.BigEndian.PutUint64(raw[1:9], uint64(accountID))
	binary.BigEndian.PutUint32(raw[9:13], uint32(expiresAt.Unix()))
	copy(raw[payloadLen:], sign(secret, raw[:payloadLen]))

	return encoding.EncodeToString(raw), expiresAt, nil
}

// IssuedBefore сообщает, выпущено ли приглашение раньше момента t.
//
// Время выпуска в токене не хранится, но все приглашения выпускаются с
// DefaultTTL, поэтому оно равно сроку минус TTL. Срок округлён вниз до секунды,
// и t округляется так же: иначе токен, выпущенный в ту же секунду сразу после
// сброса, отклонялся бы.
func IssuedBefore(expiresAt, t time.Time) bool {
	return expiresAt.Add(-DefaultTTL).Before(t.Truncate(time.Second))
}

// Parse проверяет подпись и срок, возвращая account_id приглашения.
func Parse(secret []byte, token string, now time.Time) (int64, error) {
	accountID, _, err := ParseWithExpiry(secret, token, now)
	return accountID, err
}

// ParseWithExpiry — Parse, который отдаёт ещё и срок приглашения: по нему
// вызывающий отсекает приглашения, выпущенные до сброса ссылки подписки.
func ParseWithExpiry(secret []byte, token string, now time.Time) (int64, time.Time, error) {
	if len(secret) == 0 {
		return 0, time.Time{}, errors.New("connectinvite: empty secret")
	}
	raw, err := encoding.DecodeString(token)
	if err != nil || len(raw) != tokenLen || raw[0] != tokenVersion {
		return 0, time.Time{}, ErrMalformed
	}
	// Подпись проверяем до срока: иначе по разнице ответов «протух» и
	// «не сходится» можно отличить существующий токен от выдуманного.
	if !hmac.Equal(raw[payloadLen:], sign(secret, raw[:payloadLen])) {
		return 0, time.Time{}, ErrSignature
	}

	accountID := int64(binary.BigEndian.Uint64(raw[1:9]))
	if accountID <= 0 {
		return 0, time.Time{}, ErrMalformed
	}
	exp := time.Unix(int64(binary.BigEndian.Uint32(raw[9:13])), 0)
	if !now.Before(exp) {
		return 0, time.Time{}, ErrExpired
	}
	return accountID, exp, nil
}

func sign(secret, payload []byte) []byte {
	mac := hmac.New(sha256.New, deriveKey(secret))
	mac.Write(payload)
	return mac.Sum(nil)[:macLen]
}
