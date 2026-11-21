package cabinethttp

import (
	"context"
	"net/http"
	"time"

	adminauth "remnawave-tg-shop-bot/internal/cabinet/admin/auth"
	"remnawave-tg-shop-bot/internal/cabinet/auth/jwt"
	"remnawave-tg-shop-bot/internal/cabinet/auth/ratelimit"
	"remnawave-tg-shop-bot/internal/cabinet/gate"
	"remnawave-tg-shop-bot/internal/cabinet/http/handlers"
	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
)

// registerReviewRoutes — отзывы клиентов: публичные чтение и аватары, отзыв
// самого клиента и модерация в админке.
func registerReviewRoutes(
	ctx context.Context,
	api *http.ServeMux,
	reviews *handlers.ReviewsHandler,
	adminReviews *handlers.AdminReviewsHandler,
	jwtIssuer *jwt.Issuer,
	adminChecker *adminauth.Checker,
	channelGate *gate.Gate,
	accountLim *ratelimit.Limiter,
	adminLim *ratelimit.Limiter,
) {
	publicLim := ratelimit.New(ratelimit.Rule{Count: 120, Interval: time.Minute})
	publicLim.RunGC(ctx)
	public := func(h http.HandlerFunc, route string) http.Handler {
		return methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(h, middleware.RateLimit(publicLim, ipKey(route))),
		})
	}
	api.Handle("/cabinet/api/public/reviews", public(reviews.List, "public-reviews"))
	api.Handle("/cabinet/api/public/reviews/featured", public(reviews.Featured, "public-reviews-featured"))
	// /public/reviews/{id}/avatar
	api.Handle("/cabinet/api/public/reviews/", public(reviews.Avatar, "public-reviews-avatar"))

	api.Handle("/cabinet/api/me/review", methodRouter(map[string]http.Handler{
		http.MethodGet: middleware.Chain(
			http.HandlerFunc(reviews.Mine),
			middleware.RequireAuth(jwtIssuer),
			middleware.RateLimit(accountLim, accountKey("review_mine")),
		),
		http.MethodPut: middleware.Chain(
			http.HandlerFunc(reviews.Upsert),
			middleware.RequireAuth(jwtIssuer),
			channelGate.Require(),
			middleware.RequireVerifiedEmail(),
			middleware.CSRF(),
			middleware.RateLimit(accountLim, accountKey("review_upsert")),
		),
	}))

	api.Handle("/cabinet/api/admin/reviews", methodRouter(map[string]http.Handler{
		http.MethodGet: middleware.Chain(
			http.HandlerFunc(adminReviews.List),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.RateLimit(adminLim, accountKey("admin_reviews")),
		),
		http.MethodPost: middleware.Chain(
			http.HandlerFunc(adminReviews.Create),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminLim, accountKey("admin_reviews_create")),
		),
	}))
	api.Handle("/cabinet/api/admin/reviews/", middleware.Chain(
		http.HandlerFunc(adminReviews.HandleByID),
		middleware.RequireAuth(jwtIssuer),
		middleware.RequireAdmin(adminChecker),
		middleware.CSRF(),
		middleware.RateLimit(adminLim, accountKey("admin_reviews_byid")),
	))
}
