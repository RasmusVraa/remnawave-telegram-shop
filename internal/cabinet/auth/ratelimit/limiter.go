// Package ratelimit — простой in-memory token-bucket rate-limiter по ключу.
//
// Используется для дешёвой защиты от brute-force на auth-эндпоинты. MVP — один
// процесс, Redis не нужен. Когда появится вторая реплика, этот код можно
// заменить Redis-backed без изменения интерфейса Limiter.
//
// Ключи должны включать префикс эндпоинта, чтобы лимиты на login и register
// не делили bucket. Конкретные параметры — в cabinet/config.RateLimitRules().
package ratelimit

import (
	"context"
	"crypto/sha256"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Rule задаёт лимит «N запросов в T».
//
// Для наглядности задаём в формате Count/Interval, а в rate.Limiter внутрь
// уходит rate.Every(Interval/Count) и burst=Count.
type Rule struct {
	Count    int
	Interval time.Duration
}

// maxBuckets — потолок бакетов одного лимитера. Ключи строятся из того, что
// прислал клиент (IP, email из тела), поэтому без потолка перебор случайных
// email раздувал бы map без предела. 100 000 бакетов — порядка 15 МБ.
const maxBuckets = 100_000

// Limiter — thread-safe менеджер token-bucket'ов по строковому ключу.
//
// В map лежит sha256 ключа, а не сам ключ: email из тела бывает до 64 КБ.
type Limiter struct {
	mu       sync.Mutex
	buckets  map[[sha256.Size]byte]*entry
	rule     Rule
	lifetime time.Duration // после какого простоя бакет удаляется GC
	max      int
}

type entry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// New создаёт лимитер с единым правилом для всех ключей. Если нужны разные
// правила — создавайте отдельные инстансы Limiter (по одному на route).
func New(rule Rule) *Limiter {
	l := &Limiter{
		buckets:  make(map[[sha256.Size]byte]*entry),
		rule:     rule,
		lifetime: 10 * rule.Interval,
		max:      maxBuckets,
	}
	if l.lifetime < time.Minute {
		l.lifetime = time.Minute
	}
	return l
}

// Allow возвращает true, если запрос с данным ключом разрешён прямо сейчас.
// Не блокирует — мы хотим быстро дать 429, а не заставить пользователя ждать.
func (l *Limiter) Allow(key string) bool {
	k := sha256.Sum256([]byte(key))
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	e, ok := l.buckets[k]
	if !ok {
		if len(l.buckets) >= l.max {
			l.gcLocked(now)
		}
		if len(l.buckets) >= l.max {
			// Все бакеты живые — значит, лимитер перебирают ключами. Новый не
			// заводим и отвечаем отказом: пропустить запрос без учёта значило
			// бы снять лимит ровно тогда, когда он нужен.
			return false
		}
		e = &entry{
			lim: rate.NewLimiter(rate.Every(l.rule.Interval/time.Duration(max(l.rule.Count, 1))), l.rule.Count),
		}
		l.buckets[k] = e
	}
	e.lastSeen = now
	return e.lim.Allow()
}

// RunGC запускает фоновую горутину, которая периодически удаляет бакеты, не
// использовавшиеся больше lifetime. Вызывайте один раз при инициализации.
func (l *Limiter) RunGC(ctx context.Context) {
	go func() {
		t := time.NewTicker(l.lifetime / 2)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				l.mu.Lock()
				l.gcLocked(time.Now())
				l.mu.Unlock()
			}
		}
	}()
}

// gcLocked удаляет бакеты, не использовавшиеся дольше lifetime. Вызывается под l.mu.
func (l *Limiter) gcLocked(now time.Time) {
	cutoff := now.Add(-l.lifetime)
	for k, e := range l.buckets {
		if e.lastSeen.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}
