//go:build integration

package service

import (
	"context"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/cabinet/auth/jwt"
	"remnawave-tg-shop-bot/internal/cabinet/auth/tokens"
)

// Токен, выданный при подтверждении почты, уже несёт email_verified=true.
// Раньше аккаунт читался до пометки, и до следующего refresh кабинет получал
// 403 «email not verified» на подписке и оплате.
func TestConfirmEmail_IssuedTokenIsVerified(t *testing.T) {
	svc, pool := oauthTestService(t)
	ctx := context.Background()
	acc := oauthTestAccount(t, svc, pool)

	code, hash, err := tokens.GenerateEmailVerifyOTP(svc.cfg.EmailCodeKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.evs.Create(ctx, acc.ID, hash, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create code: %v", err)
	}
	pair, err := svc.ConfirmEmail(ctx, code, *acc.Email, "", "test", "127.0.0.1")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	claims, err := jwt.NewIssuer([]byte("0123456789abcdef0123456789abcdef"), 15*time.Minute, "test").Verify(pair.AccessToken)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if !claims.EmailVerified {
		t.Fatal("access token issued on email confirmation has email_verified=false")
	}
}
