package middleware

import (
	"context"
	"net/http"

	adminauth "remnawave-tg-shop-bot/internal/cabinet/admin/auth"
	"remnawave-tg-shop-bot/internal/cabinet/auth/jwt"
)

// RequireAdmin — middleware поверх RequireAuth. Отклоняет 403, если аккаунт не
// является администратором (по привязанному Telegram == ADMIN_TELEGRAM_ID).
// Результат кешируется в контексте запроса, чтобы не дёргать БД повторно.
func RequireAdmin(checker *adminauth.Checker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := AuthClaims(r)
			if claims == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			actor, ok := checker.AdminTelegramID(r.Context(), claims.AccountID)
			if !ok {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyIsAdmin, true)
			auditAdminAction(w, r.WithContext(ctx), actor, next)
		})
	}
}

const ctxKeyIsAdmin ctxKey = "is_admin"

// IsAdminFromContext возвращает true, если RequireAdmin уже подтвердил админа.
func IsAdminFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeyIsAdmin).(bool)
	return v
}

// ResolveIsAdmin — хелпер для /me: проверяет admin-статус по claims без middleware.
func ResolveIsAdmin(ctx context.Context, checker *adminauth.Checker, claims *jwt.Claims) bool {
	if checker == nil || claims == nil {
		return false
	}
	return checker.IsAdmin(ctx, claims.AccountID)
}
