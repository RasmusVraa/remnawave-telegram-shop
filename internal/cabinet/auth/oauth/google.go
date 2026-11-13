// Package oauth — вспомогательный слой для Google OAuth 2.0 + PKCE.
//
// Архитектурные решения:
//
//   - State — случайный opaque-токен, хранится in-memory с TTL 10 минут.
//     Один процесс → shared map вполне работает. Если понадобится HA,
//     state можно перенести в Redis или подписанный cookie.
//
//   - PKCE — code_verifier хранится рядом со state (в той же записи);
//     code_challenge = BASE64URL(SHA256(verifier)).
//
//   - Userinfo читается через стандартный endpoint Google
//     (https://www.googleapis.com/oauth2/v3/userinfo).
//     «email_verified: true» от Google считается достаточным: показываем
//     пользователю, что адрес подтверждён через Google SSO.
package oauth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// stateTTL — сколько живёт OAuth state в памяти. 10 минут соответствуют
// рекомендациям RFC 6819 (state should expire after a short time).
const stateTTL = 10 * time.Minute

// stateRecord — запись о незавершённом OAuth flow.
type stateRecord struct {
	verifier      string    // PKCE code_verifier
	referralRaw   string    // опционально: ref из /google/start?ref= (для новой регистрации)
	linkAccountID int64     // 0 — обычный login/register; >0 — привязка Google к этому cabinet_account
	expiresAt     time.Time // когда выбрасывать запись
}

// StateStore — thread-safe хранилище state → {verifier, expiry}.
// RunGC необходимо вызвать один раз при старте, чтобы удалять просроченные state'ы.
type StateStore struct {
	mu    sync.Mutex
	store map[string]stateRecord
}

// NewStateStore инициализирует пустое хранилище.
func NewStateStore() *StateStore { return &StateStore{store: make(map[string]stateRecord)} }

// Save сохраняет (state → verifier + опциональный referral + linkAccountID).
func (s *StateStore) Save(state, verifier, referralRaw string, linkAccountID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[state] = stateRecord{
		verifier:      verifier,
		referralRaw:   referralRaw,
		linkAccountID: linkAccountID,
		expiresAt:     time.Now().Add(stateTTL),
	}
}

// Pop забирает verifier, referral и linkAccountID по state и тут же удаляет запись (одноразовое использование).
// Возвращает ok=false, если state не найден или просрочен.
func (s *StateStore) Pop(state string) (verifier string, referralRaw string, linkAccountID int64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, found := s.store[state]
	if !found {
		return "", "", 0, false
	}
	delete(s.store, state)
	if time.Now().After(rec.expiresAt) {
		return "", "", 0, false
	}
	return rec.verifier, rec.referralRaw, rec.linkAccountID, true
}

// RunGC запускает горутину, которая раз в минуту удаляет просроченные state'ы.
// ctx — контекст shutdown'а процесса.
func (s *StateStore) RunGC(ctx context.Context) {
	go func() {
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				s.gc()
			}
		}
	}()
}

func (s *StateStore) gc() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.store {
		if now.After(v.expiresAt) {
			delete(s.store, k)
		}
	}
}

// ============================================================================
// Google OAuth provider
// ============================================================================

// GoogleProvider инкапсулирует oauth2.Config для Google + StateStore.
type GoogleProvider struct {
	cfg   *oauth2.Config
	store *StateStore
}

// NewGoogleProvider — конструктор.
// clientID, clientSecret, redirectURL — из CABINET_GOOGLE_* переменных.
func NewGoogleProvider(clientID, clientSecret, redirectURL string, store *StateStore) *GoogleProvider {
	cfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes: []string{
			"openid",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}
	return &GoogleProvider{cfg: cfg, store: store}
}

// StartResult — что возвращает Start(), нужно для редиректа.
type StartResult struct {
	RedirectURL string // Куда слать браузер
	State       string // Для CSRF-проверки (возвращаем в cookie или логируем)
}

const maxOAuthReferralLen = 128

// Start генерирует state + PKCE verifier, сохраняет в StateStore,
// возвращает URL для редиректа в Google.
// referralRaw — опционально query ref (как у email-регистрации: ref_<telegram_id>).
// linkAccountID — если >0, callback привяжет Google к этому аккаунту (нужна валидная refresh-сессия).
func (p *GoogleProvider) Start(referralRaw string, linkAccountID int64) (*StartResult, error) {
	ref := strings.TrimSpace(referralRaw)
	if len(ref) > maxOAuthReferralLen {
		ref = ref[:maxOAuthReferralLen]
	}
	state, err := randomHex(16)
	if err != nil {
		return nil, fmt.Errorf("google oauth start: gen state: %w", err)
	}
	// PKCE: в AuthCodeURL нужно передать сам verifier — S256ChallengeOption
	// внутри считает SHA256(verifier); передавать готовый challenge нельзя (двойной хеш → invalid_grant на token).
	verifier := oauth2.GenerateVerifier()
	p.store.Save(state, verifier, ref, linkAccountID)

	authURL := p.cfg.AuthCodeURL(state,
		oauth2.AccessTypeOnline,
		oauth2.S256ChallengeOption(verifier),
	)
	return &StartResult{RedirectURL: authURL, State: state}, nil
}

// GoogleUserInfo — subset полей userinfo от Google.
type GoogleUserInfo struct {
	Sub           string `json:"sub"` // stable unique Google user ID
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

// Callback обменивает code на токены, проверяет state, возвращает userinfo, ref и linkAccountID из state.
func (p *GoogleProvider) Callback(ctx context.Context, state, code string) (*GoogleUserInfo, string, int64, error) {
	verifier, referralRaw, linkAccountID, ok := p.store.Pop(state)
	if !ok {
		return nil, "", 0, ErrStateInvalid
	}
	token, err := p.cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, "", 0, fmt.Errorf("google oauth exchange: %w", err)
	}
	info, err := fetchUserInfo(ctx, p.cfg, token)
	if err != nil {
		return nil, "", 0, fmt.Errorf("google userinfo: %w", err)
	}
	if info.Sub == "" {
		return nil, "", 0, errors.New("google userinfo: empty sub")
	}
	return info, referralRaw, linkAccountID, nil
}

// ErrStateInvalid — state не найден или просрочен.
var ErrStateInvalid = errors.New("oauth: state invalid or expired")

// fetchUserInfo делает GET на userinfo endpoint с Bearer-токеном.
func fetchUserInfo(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token) (*GoogleUserInfo, error) {
	client := cfg.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo: status %d", resp.StatusCode)
	}
	var info GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("userinfo: decode: %w", err)
	}
	return &info, nil
}

// ============================================================================
// Helpers
// ============================================================================

// randomHex возвращает n случайных байт, закодированных в hex (length=2n).
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}
