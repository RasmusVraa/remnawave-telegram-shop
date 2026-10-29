//go:build integration

package service

// Интеграционные тесты привязки внешнего входа по совпадению email.
// Запуск:
//
//	CABINET_INTEGRATION_PG=postgres://user:pass@localhost:5432/db?sslmode=disable \
//	  go test ./internal/cabinet/auth/service/ -tags=integration -count=1

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"remnawave-tg-shop-bot/internal/cabinet/auth/jwt"
	"remnawave-tg-shop-bot/internal/cabinet/auth/password"
	"remnawave-tg-shop-bot/internal/cabinet/mail"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/database"
)

func oauthTestService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("CABINET_INTEGRATION_PG")
	if dsn == "" {
		t.Skip("set CABINET_INTEGRATION_PG to run integration tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	_, f, _, _ := runtime.Caller(0)
	dir := filepath.Clean(filepath.Join(filepath.Dir(f), "..", "..", "..", "..", "db", "migrations"))
	if err := database.RunMigrations(ctx, &database.MigrationConfig{Direction: "up", MigrationsPath: dir}, pool); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	svc := New(Config{
		PublicURL:       "https://cabinet.example.invalid",
		AccessTTL:       15 * time.Minute,
		RefreshTTL:      24 * time.Hour,
		EmailVerifyTTL:  time.Hour,
		DefaultLanguage: "ru",
		PasswordParams:  password.DefaultParams(),
		PasswordPolicy:  password.DefaultPolicy(),
		EmailCodeKey:    []byte("test-key"),
	},
		repository.NewAccountRepo(pool), repository.NewIdentityRepo(pool), repository.NewSessionRepo(pool),
		repository.NewEmailVerificationRepo(pool), repository.NewPasswordResetRepo(pool),
		jwt.NewIssuer([]byte("0123456789abcdef0123456789abcdef"), 15*time.Minute, "test"),
		mail.NewMailer(mail.NewSender(mail.Config{DryRun: true})), nil,
		repository.NewEmailMergeVerificationRepo(pool))
	svc.ensureLinkStore(ctx)
	return svc, pool
}

func oauthTestAccount(t *testing.T, svc *Service, pool *pgxpool.Pool) *repository.Account {
	t.Helper()
	email := fmt.Sprintf("oauth-link-%d@example.invalid", time.Now().UnixNano())
	acc, err := svc.accounts.Create(context.Background(), email, "", "ru")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM cabinet_account WHERE id = $1`, acc.ID)
	})
	return acc
}

func pendingLinkToken(s *Service) string {
	s.linkStore.mu.Lock()
	defer s.linkStore.mu.Unlock()
	for tok := range s.linkStore.store {
		return tok
	}
	return ""
}

// Аккаунт с этим email заранее зарегистрировал кто-то, почту не подтвердив.
// Вход через Яндекс с тем же email не должен попасть в этот аккаунт сразу —
// только по ссылке из письма владельцу почты.
func TestOAuthLogin_ExistingEmailNeedsMailboxConfirmation(t *testing.T) {
	svc, pool := oauthTestService(t)
	ctx := context.Background()
	acc := oauthTestAccount(t, svc, pool)
	if acc.EmailVerified() {
		t.Fatal("precondition: email must be unverified")
	}
	pid := uuid.NewString()

	res, err := svc.oauthLoginFlowGeneric(ctx, repository.ProviderYandex, pid, *acc.Email, []byte(`{}`), "", "test", "127.0.0.1", "yandex")
	if !errors.Is(err, ErrGoogleLinkRequired) {
		t.Fatalf("err = %v, want ErrGoogleLinkRequired", err)
	}
	if res.Pair != nil {
		t.Fatal("session issued before mailbox confirmation")
	}
	if _, err := svc.ids.FindByProvider(ctx, repository.ProviderYandex, pid); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("identity linked before confirmation: err = %v", err)
	}

	tok := pendingLinkToken(svc)
	if tok == "" {
		t.Fatal("no pending link saved")
	}
	pair, err := svc.GoogleLinkConfirm(ctx, tok, "test", "127.0.0.1")
	if err != nil || pair == nil {
		t.Fatalf("confirm: pair=%v err=%v", pair, err)
	}
	ident, err := svc.ids.FindByProvider(ctx, repository.ProviderYandex, pid)
	if err != nil || ident.AccountID != acc.ID {
		t.Fatalf("after confirm identity = %+v, err = %v", ident, err)
	}
	if _, err := svc.GoogleLinkConfirm(ctx, tok, "test", "127.0.0.1"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("second confirm: err = %v, want ErrInvalidToken", err)
	}
}

// Привязка VK к своему аккаунту, когда email VK занят другим аккаунтом: никакой
// заявки на слияние — иначе второй аккаунт поглощался бы без доказательства
// владения им.
func TestOAuthLink_EmailOfAnotherAccountDoesNotCreateMergeClaim(t *testing.T) {
	svc, pool := oauthTestService(t)
	ctx := context.Background()
	current := oauthTestAccount(t, svc, pool)
	other := oauthTestAccount(t, svc, pool)

	claims := 0
	svc.SetMergeEmailPeerClaimSaver(func(context.Context, int64, int64) error {
		claims++
		return nil
	})
	pair, err := svc.issueSession(ctx, current, uuid.New(), "test", "127.0.0.1")
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}

	_, err = svc.oauthLinkFlowGeneric(ctx, current.ID, pair.RefreshToken, repository.ProviderVK, uuid.NewString(), *other.Email, []byte(`{}`), "test", "127.0.0.1", "vk")
	if !errors.Is(err, ErrVKLinkEmailConflict) {
		t.Fatalf("err = %v, want ErrVKLinkEmailConflict", err)
	}
	if claims != 0 {
		t.Fatalf("merge claim saved %d times without proof of owning the other account", claims)
	}
}
