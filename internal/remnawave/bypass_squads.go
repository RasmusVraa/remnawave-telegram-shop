package remnawave

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// SetUserSquadsAndLimit обновляет у пользователя только внутренние сквады и/или лимит трафика.
// squads == nil — сквады не трогаем; limit == nil — лимит не трогаем.
// Статус не передаётся: после повышения лимита панель сама возвращает ACTIVE из LIMITED
// (проверено на стенде 3.4).
func (r *Client) SetUserSquadsAndLimit(ctx context.Context, userID int64, squads *[]uuid.UUID, limit *int64) error {
	if userID <= 0 {
		return ErrUserNotFound
	}
	if squads == nil && limit == nil {
		return nil
	}
	body := &UpdateUserRequest{ID: &userID, ActiveInternalSquads: squads, TrafficLimitBytes: limit}
	var resp apiResponse[User]
	if err := r.doJSON(ctx, http.MethodPatch, "/api/users", body, &resp); err != nil {
		return fmt.Errorf("remnawave: update user %d: %w", userID, err)
	}
	return nil
}

// UserSquadUUIDs — текущие внутренние сквады пользователя.
func UserSquadUUIDs(u *User) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(u.ActiveInternalSquads))
	for _, s := range u.ActiveInternalSquads {
		out = append(out, s.UUID)
	}
	return out
}

// WithSquads возвращает have с добавленными add (без дублей, порядок сохранён).
func WithSquads(have, add []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(have)+len(add))
	out := make([]uuid.UUID, 0, len(have)+len(add))
	for _, u := range have {
		if _, ok := seen[u]; !ok {
			seen[u] = struct{}{}
			out = append(out, u)
		}
	}
	for _, u := range add {
		if _, ok := seen[u]; !ok {
			seen[u] = struct{}{}
			out = append(out, u)
		}
	}
	return out
}

// WithoutSquads возвращает have без drop.
func WithoutSquads(have, drop []uuid.UUID) []uuid.UUID { return removeSquads(have, drop) }
