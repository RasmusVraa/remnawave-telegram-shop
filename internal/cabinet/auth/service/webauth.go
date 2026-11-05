package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/cabinet/webauth"
)

var (
	ErrWebAuthDisabled = errors.New("auth: webauth disabled")
	ErrWebAuthInvalid  = errors.New("auth: webauth token invalid")
	ErrWebAuthPending  = errors.New("auth: webauth pending")
	ErrWebAuthExpired  = errors.New("auth: webauth expired")
)

// SetWebAuthStore подключает хранилище токенов входа из приложения.
func (s *Service) SetWebAuthStore(store *webauth.Store) {
	s.webAuth = store
}

// RequestWebAuthToken выпускает одноразовый токен для deeplink-входа.
func (s *Service) RequestWebAuthToken() (token string, expiresInSec int, err error) {
	if s.webAuth == nil {
		return "", 0, ErrWebAuthDisabled
	}
	tok, ttl, err := s.webAuth.Issue()
	if err != nil {
		return "", 0, err
	}
	return tok, int(ttl.Seconds()), nil
}

// ApproveWebAuthFromTelegram подтверждает токен после /start webauth_<token>.
func (s *Service) ApproveWebAuthFromTelegram(ctx context.Context, token string, telegramID int64, username string) error {
	if s.webAuth == nil {
		return ErrWebAuthDisabled
	}
	token = strings.TrimSpace(token)
	if token == "" || telegramID == 0 {
		return ErrWebAuthInvalid
	}
	if !s.webAuth.IsPending(token) {
		return ErrWebAuthInvalid
	}
	pair, err := s.telegramFindOrCreate(ctx, telegramID, username, "", "webauth-bot", "")
	if err != nil {
		return err
	}
	return s.completeWebAuth(token, pair)
}

// ApproveWebAuthFromAccount подтверждает токен залогиненным пользователем сайта.
func (s *Service) ApproveWebAuthFromAccount(ctx context.Context, token string, accountID int64, userAgent, ip string) error {
	if s.webAuth == nil {
		return ErrWebAuthDisabled
	}
	token = strings.TrimSpace(token)
	if token == "" || accountID <= 0 {
		return ErrWebAuthInvalid
	}
	if !s.webAuth.IsPending(token) {
		return ErrWebAuthInvalid
	}
	acc, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("webauth: load account: %w", err)
	}
	if acc == nil || acc.Status != repository.AccountStatusActive {
		return ErrInvalidCredentials
	}
	pair, err := s.issueSession(ctx, acc, uuid.New(), userAgent, ip)
	if err != nil {
		return err
	}
	return s.completeWebAuth(token, pair)
}

// PollWebAuth забирает сессию после подтверждения токена.
func (s *Service) PollWebAuth(token string) (*TokenPair, error) {
	if s.webAuth == nil {
		return nil, ErrWebAuthDisabled
	}
	sess, err := s.webAuth.Poll(token)
	if err != nil {
		switch {
		case errors.Is(err, webauth.ErrPending):
			return nil, ErrWebAuthPending
		case errors.Is(err, webauth.ErrExpired):
			return nil, ErrWebAuthExpired
		case errors.Is(err, webauth.ErrNotFound), errors.Is(err, webauth.ErrConsumed), errors.Is(err, webauth.ErrInvalidArg):
			return nil, ErrWebAuthInvalid
		default:
			return nil, err
		}
	}
	return &TokenPair{
		AccessToken:  sess.AccessToken,
		AccessExp:    sess.AccessExp,
		RefreshToken: sess.RefreshToken,
		CSRFToken:    sess.CSRFToken,
		AccountID:    sess.AccountID,
	}, nil
}

func (s *Service) completeWebAuth(token string, pair *TokenPair) error {
	if pair == nil {
		return ErrWebAuthInvalid
	}
	err := s.webAuth.Complete(token, webauth.Session{
		AccessToken:  pair.AccessToken,
		AccessExp:    pair.AccessExp,
		RefreshToken: pair.RefreshToken,
		CSRFToken:    pair.CSRFToken,
		AccountID:    pair.AccountID,
	})
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, webauth.ErrExpired):
		return ErrWebAuthExpired
	case errors.Is(err, webauth.ErrNotFound), errors.Is(err, webauth.ErrInvalidArg), errors.Is(err, webauth.ErrConsumed):
		return ErrWebAuthInvalid
	default:
		return err
	}
}
