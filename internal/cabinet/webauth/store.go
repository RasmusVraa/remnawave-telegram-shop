// Package webauth — одноразовые токены для входа в десктоп/мобильное
// приложение через Telegram (/start webauth_<token>) или сайт кабинета.
//
// Поток:
//  1. Клиент POST /cabinet/api/auth/deeplink/request → {token, telegram_url, web_url}
//  2. Пользователь подтверждает вход в боте или на /cabinet/app-auth
//  3. Клиент POST /cabinet/api/auth/deeplink/poll → 202 пока ждёт, 200 с JWT
package webauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

// Prefix — аргумент /start в Telegram: /start webauth_<token>.
const Prefix = "webauth_"

// DefaultTTL — сколько токен живёт до подтверждения.
const DefaultTTL = 5 * time.Minute

var (
	ErrNotFound   = errors.New("webauth: token not found")
	ErrExpired    = errors.New("webauth: token expired")
	ErrPending    = errors.New("webauth: still pending")
	ErrConsumed   = errors.New("webauth: token already used")
	ErrDisabled   = errors.New("webauth: not configured")
	ErrInvalidArg = errors.New("webauth: invalid token")
)

// Session — пара токенов кабинета, которую забирает приложение через poll.
type Session struct {
	AccessToken  string
	AccessExp    time.Time
	RefreshToken string
	CSRFToken    string
	AccountID    int64
}

type status int

const (
	statusPending status = iota
	statusReady
	statusConsumed
)

type record struct {
	expiresAt time.Time
	status    status
	session   *Session
}

// Store — in-memory хранилище одноразовых токенов (TTL + GC).
type Store struct {
	mu    sync.Mutex
	items map[string]*record
	ttl   time.Duration
}

// New создаёт пустое хранилище.
func New(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Store{items: make(map[string]*record), ttl: ttl}
}

// Issue создаёт новый pending-токен.
func (s *Store) Issue() (token string, expiresIn time.Duration, err error) {
	raw := make([]byte, 16)
	if _, err = rand.Read(raw); err != nil {
		return "", 0, err
	}
	token = hex.EncodeToString(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[token] = &record{
		expiresAt: time.Now().Add(s.ttl),
		status:    statusPending,
	}
	return token, s.ttl, nil
}

// Complete привязывает сессию к токену после подтверждения в боте/на сайте.
func (s *Store) Complete(token string, session Session) error {
	token = strings.TrimSpace(token)
	if token == "" || session.AccountID <= 0 || session.AccessToken == "" {
		return ErrInvalidArg
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.lookupLocked(token)
	if err != nil {
		return err
	}
	if rec.status == statusConsumed {
		return ErrConsumed
	}
	cp := session
	rec.session = &cp
	rec.status = statusReady
	return nil
}

// Poll возвращает сессию один раз. Пока не подтверждено — ErrPending.
func (s *Store) Poll(token string) (*Session, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrInvalidArg
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.lookupLocked(token)
	if err != nil {
		return nil, err
	}
	switch rec.status {
	case statusPending:
		return nil, ErrPending
	case statusConsumed:
		return nil, ErrConsumed
	case statusReady:
		if rec.session == nil {
			return nil, ErrNotFound
		}
		out := *rec.session
		rec.status = statusConsumed
		rec.session = nil
		delete(s.items, token)
		return &out, nil
	default:
		return nil, ErrNotFound
	}
}

// IsPending сообщает, ждёт ли токен подтверждения (для бота/сайта).
func (s *Store) IsPending(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.lookupLocked(strings.TrimSpace(token))
	return err == nil && rec.status == statusPending
}

func (s *Store) lookupLocked(token string) (*record, error) {
	rec, ok := s.items[token]
	if !ok {
		return nil, ErrNotFound
	}
	if time.Now().After(rec.expiresAt) {
		delete(s.items, token)
		return nil, ErrExpired
	}
	return rec, nil
}

// RunGC удаляет просроченные записи раз в минуту.
func (s *Store) RunGC(ctx context.Context) {
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				now := time.Now()
				s.mu.Lock()
				for k, v := range s.items {
					if now.After(v.expiresAt) {
						delete(s.items, k)
					}
				}
				s.mu.Unlock()
			}
		}
	}()
}

// TokenFromStartArg достаёт токен из аргумента /start ("webauth_abc" → "abc").
func TokenFromStartArg(arg string) (string, bool) {
	arg = strings.TrimSpace(arg)
	if !strings.HasPrefix(arg, Prefix) {
		return "", false
	}
	token := strings.TrimPrefix(arg, Prefix)
	if token == "" || len(token) > 64 {
		return "", false
	}
	for _, c := range token {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return "", false
		}
	}
	return strings.ToLower(token), true
}

// StartPayload собирает аргумент для t.me/bot?start=.
func StartPayload(token string) string {
	return Prefix + strings.TrimSpace(token)
}

// ---------------------------------------------------------------------------
// Мост бот ↔ HTTP: approver выставляется при Mount кабинета.
// ---------------------------------------------------------------------------

var (
	approverMu sync.RWMutex
	approver   func(ctx context.Context, token string, telegramID int64, username string) error
)

// SetTelegramApprover регистрирует обработчик подтверждения из /start.
func SetTelegramApprover(fn func(ctx context.Context, token string, telegramID int64, username string) error) {
	approverMu.Lock()
	defer approverMu.Unlock()
	approver = fn
}

// ApproveFromTelegram вызывается из StartCommandHandler бота.
func ApproveFromTelegram(ctx context.Context, token string, telegramID int64, username string) error {
	approverMu.RLock()
	fn := approver
	approverMu.RUnlock()
	if fn == nil {
		return ErrDisabled
	}
	return fn(ctx, token, telegramID, username)
}
