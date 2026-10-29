// Package tokens — генерация и sha256-хеширование одноразовых токенов
// (refresh, email verification, password reset).
//
// Все эти токены — «capability»: их полный вид отправляется клиенту один раз,
// а в БД лежит только sha256. При утечке дампа БД злоумышленник не может
// предъявить токен в свой клиент (как и с bcrypt, но без соли — sha256 здесь
// достаточно: пространство ≥ 2^256, brute-force не имеет смысла).
package tokens

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
)

// DefaultRefreshBytes — 32 байта случайности для refresh-токена (256 бит).
const DefaultRefreshBytes = 32

// Generate возвращает случайный токен в base64url-без-паддинга формате и
// sha256-хеш от его байтового представления (до base64-кодирования).
//
// Почему sha256 от байтов, а не от строки: так унифицировано с тем, как токен
// приходит от клиента (мы декодируем base64url, получаем те же байты) —
// и хеш совпадёт. Это проще и безопаснее, чем дважды base64-энкодить.
func Generate(byteLen int) (token string, hash [32]byte, err error) {
	if byteLen <= 0 {
		byteLen = DefaultRefreshBytes
	}
	raw := make([]byte, byteLen)
	if _, err := rand.Read(raw); err != nil {
		return "", [32]byte{}, fmt.Errorf("tokens: read rand: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	hash = sha256.Sum256(raw)
	return token, hash, nil
}

// HashString считает sha256 от base64url-токена (как его прислал клиент).
// Используйте для lookup в БД: SELECT ... WHERE refresh_token_hash = $1.
// Возвращает ошибку, если строка не валидный base64url.
func HashString(token string) ([32]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return [32]byte{}, fmt.Errorf("tokens: decode: %w", err)
	}
	return sha256.Sum256(raw), nil
}

// EmailVerifyOTPLen — длина кода в письме подтверждения email (только цифры).
const EmailVerifyOTPLen = 6

// IsEmailVerifyOTP — ровно 6 ASCII-цифр (код из письма).
func IsEmailVerifyOTP(s string) bool {
	if len(s) != EmailVerifyOTPLen {
		return false
	}
	for i := 0; i < EmailVerifyOTPLen; i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// HashEmailVerifyOTP — sha256 кода без ключа. Так хэшировались коды до появления
// ключа; функция остаётся, чтобы принимать коды, выданные до обновления.
func HashEmailVerifyOTP(code string) [32]byte {
	return sha256.Sum256([]byte(code))
}

// emailVerifyOTPKeyContext разводит ключ кодов с другими ключами из секрета кабинета.
const emailVerifyOTPKeyContext = "remnawave-cabinet/email-verify-otp/v1\x00"

// EmailVerifyOTPKey выводит из секрета кабинета ключ для хэша кодов. У кода
// 10^6 вариантов: без ключа дамп БД раскрывал бы все живые коды перебором за
// доли секунды, с ключом — только вместе с секретом.
func EmailVerifyOTPKey(secret []byte) []byte {
	sum := sha256.Sum256(append([]byte(emailVerifyOTPKeyContext), secret...))
	return sum[:]
}

// HashEmailVerifyOTPKeyed — HMAC-SHA256 кода на ключе из EmailVerifyOTPKey.
func HashEmailVerifyOTPKeyed(key []byte, code string) [32]byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(code))
	var out [32]byte
	copy(out[:], mac.Sum(nil))
	return out
}

// GenerateEmailVerifyOTP возвращает 6-значный код и его HMAC для записи в БД.
// 20 бит энтропии хватает только вместе с ограничением попыток на код и
// поиском в пределах одного аккаунта (см. service.ConfirmEmail).
func GenerateEmailVerifyOTP(key []byte) (code string, hash [32]byte, err error) {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", [32]byte{}, fmt.Errorf("tokens: read rand: %w", err)
	}
	n := binary.BigEndian.Uint32(buf[:])
	code = fmt.Sprintf("%06d", n%1000000)
	hash = HashEmailVerifyOTPKeyed(key, code)
	return code, hash, nil
}
