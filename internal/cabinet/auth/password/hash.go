// Package password — хеширование паролей Argon2id и сравнение в constant time.
//
// Формат хеша — стандартный PHC: "$argon2id$v=19$m=<mem>,t=<iter>,p=<par>$<salt>$<hash>"
// (salt и hash — base64 без паддинга). Такой формат хранит параметры в самом хеше,
// что позволяет поднять cost без инвалидации старых паролей и поддерживать cost-drift
// (пере-хеш старых паролей на логине, если текущие параметры сильнее).
//
// Параметры по умолчанию взяты из mvp-tz.md раздел 8.1:
// memory=64 MiB, iterations=3, parallelism=2, saltLen=16, keyLen=32.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Params задаёт параметры Argon2id. Zero-value — не валиден, используйте DefaultParams().
type Params struct {
	Memory      uint32 // в KiB
	Iterations  uint32
	Parallelism uint8
	SaltLen     uint32 // в байтах
	KeyLen      uint32 // в байтах
}

// DefaultParams возвращает параметры из ТЗ 8.1 (64 MiB / 3 / 2 / salt=16 / key=32).
// Это консервативный baseline; можно поднимать Memory/Iterations при апгрейде железа.
func DefaultParams() Params {
	return Params{
		Memory:      64 * 1024,
		Iterations:  3,
		Parallelism: 2,
		SaltLen:     16,
		KeyLen:      32,
	}
}

// slots ограничивает число одновременных вычислений Argon2id. Каждое берёт
// Memory (64 MiB по умолчанию): сотня параллельных попыток входа съедала бы
// несколько гигабайт, и процесс — вместе с ботом и приёмом оплат — убивал OOM.
// С ограничением лишние запросы ждут очереди: вход под атакой медленнее,
// но магазин жив. Ожидающий запрос памяти почти не держит.
var slots = make(chan struct{}, slotCount(runtime.GOMAXPROCS(0)))

// slotCount — половина ядер (у Argon2id parallelism=2), но от 2 до 4:
// на 4 слотах пик памяти — 256 MiB.
func slotCount(procs int) int {
	return min(max(procs/2, 2), 4)
}

func idKey(plain, salt []byte, p Params, keyLen uint32) []byte {
	slots <- struct{}{}
	defer func() { <-slots }()
	return argon2.IDKey(plain, salt, p.Iterations, p.Memory, p.Parallelism, keyLen)
}

// ErrInvalidHashFormat возвращается при попытке сравнить пароль с неразборчивым хешем.
var ErrInvalidHashFormat = errors.New("password: invalid hash format")

// ErrIncompatibleVersion — если хеш был создан более новой версией argon2 (forward-compat).
var ErrIncompatibleVersion = errors.New("password: incompatible argon2 version")

// HashPassword хеширует пароль с заданными параметрами и случайной солью.
// Возвращает строку в PHC-формате.
func HashPassword(plain string, p Params) (string, error) {
	if p.SaltLen == 0 || p.KeyLen == 0 || p.Memory == 0 || p.Iterations == 0 || p.Parallelism == 0 {
		return "", errors.New("password: invalid params")
	}

	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password: read salt: %w", err)
	}

	hash := idKey([]byte(plain), salt, p, p.KeyLen)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Iterations, p.Parallelism, b64Salt, b64Hash,
	), nil
}

// ComparePasswordAndHash сравнивает plain-пароль с PHC-хешем в constant time.
// Вторым значением возвращает true, если текущие параметры слабее, чем defaults,
// — это сигнал «перехешь пароль на login под новые параметры».
func ComparePasswordAndHash(plain, encoded string, defaults Params) (ok bool, needsRehash bool, err error) {
	p, salt, key, err := decodeHash(encoded)
	if err != nil {
		return false, false, err
	}

	other := idKey([]byte(plain), salt, p, uint32(len(key)))
	if subtle.ConstantTimeCompare(key, other) != 1 {
		return false, false, nil
	}

	needsRehash = p.Memory < defaults.Memory ||
		p.Iterations < defaults.Iterations ||
		p.Parallelism < defaults.Parallelism ||
		uint32(len(key)) < defaults.KeyLen
	return true, needsRehash, nil
}

// decodeHash парсит PHC-строку. Строгий парсер — никаких частичных матчей.
func decodeHash(encoded string) (Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Params{}, nil, nil, ErrInvalidHashFormat
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Params{}, nil, nil, fmt.Errorf("%w: %v", ErrInvalidHashFormat, err)
	}
	if version != argon2.Version {
		return Params{}, nil, nil, ErrIncompatibleVersion
	}

	var p Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil {
		return Params{}, nil, nil, fmt.Errorf("%w: %v", ErrInvalidHashFormat, err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Params{}, nil, nil, fmt.Errorf("%w: decode salt: %v", ErrInvalidHashFormat, err)
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Params{}, nil, nil, fmt.Errorf("%w: decode key: %v", ErrInvalidHashFormat, err)
	}
	p.SaltLen = uint32(len(salt))
	p.KeyLen = uint32(len(key))

	return p, salt, key, nil
}

// DummyCompare выполняет "заглушечное" сравнение с синтетическим хешем, чтобы
// время ответа на login с несуществующим email совпадало с реальной веткой.
// Защита от account-enumeration по таймингу.
//
// Хеш создаётся один раз на старте, но здесь мы просто вычисляем argon2 на
// фиксированной соли — это дешевле и безопасно: никаких атакуемых значений не
// возвращается.
func DummyCompare(defaults Params) {
	// Фиксированные, но безвредные byte-slices — главное, чтобы argon2.IDKey
	// реально провернул все iterations * memory и съел сопоставимое время.
	salt := []byte("dummy-account-enum")
	_ = idKey([]byte("dummy-password"), salt, defaults, defaults.KeyLen)
}
