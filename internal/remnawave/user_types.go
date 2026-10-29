package remnawave

import (
	"time"

	"github.com/google/uuid"
)

// InternalSquadRef короткое описание internal squad из ответа панели.
// Сквады остались на UUID и в Remnawave 3.x — сменился только идентификатор пользователя.
type InternalSquadRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// User represents a Remnawave user.
//
// ВАЖНО (Remnawave 3.0.0): поле `uuid` из объекта пользователя удалено,
// пользователь адресуется числовым `id`. `shortUuid` сохранён и используется
// в публичных ссылках на подписку. См. .cursor/work-in-progress/remnawave-3x-migration/.
type User struct {
	ID                   int64              `json:"id"`
	ShortUUID            string             `json:"shortUuid"`
	Username             string             `json:"username"`
	SubscriptionUrl      string             `json:"subscriptionUrl"`
	ExpireAt             time.Time          `json:"expireAt"`
	TelegramID           *int64             `json:"telegramId"`
	Status               string             `json:"status"`
	TrafficLimitBytes    int64              `json:"trafficLimitBytes"`
	TrafficLimitStrategy string             `json:"trafficLimitStrategy"`
	HwidDeviceLimit      *int               `json:"hwidDeviceLimit"`
	Description          *string            `json:"description"`
	Tag                  *string            `json:"tag"`
	Email                *string            `json:"email"`
	ExternalSquadUuid    *uuid.UUID         `json:"externalSquadUuid"`
	LastTrafficResetAt   *time.Time         `json:"lastTrafficResetAt"`
	CreatedAt            *time.Time         `json:"createdAt"`
	UpdatedAt            *time.Time         `json:"updatedAt"`
	ActiveInternalSquads []InternalSquadRef `json:"activeInternalSquads"`
	UserTraffic          UserTraffic        `json:"userTraffic"`
}

type UserTraffic struct {
	UsedTrafficBytes         float64    `json:"usedTrafficBytes"`
	LifetimeUsedTrafficBytes float64    `json:"lifetimeUsedTrafficBytes"`
	OnlineAt                 *time.Time `json:"onlineAt"`
	FirstConnectedAt         *time.Time `json:"firstConnectedAt"`
	LastConnectedNodeUuid    *uuid.UUID `json:"lastConnectedNodeUuid"`
}

// Device — устройство пользователя. В 3.x привязка идёт по числовому userId.
type Device struct {
	Hwid        string    `json:"hwid"`
	UserID      int64     `json:"userId"`
	Platform    *string   `json:"platform"`
	OsVersion   *string   `json:"osVersion"`
	DeviceModel *string   `json:"deviceModel"`
	UserAgent   *string   `json:"userAgent"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// usersStreamBody — тело ответа GET /api/users/stream.
//
// Внимание на асимметрию из спеки 3.3.2: query-параметр `cursor` объявлен как
// number, а `nextCursor` в ответе — строка (nullable). Поэтому здесь string,
// и в запрос он подставляется как есть, без парсинга в число.
type usersStreamBody struct {
	Users      []User  `json:"users"`
	NextCursor *string `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
}

// getUserDevicesResponse is the raw API response for GET /api/hwid/devices/{userId}.
type getUserDevicesResponse struct {
	Response struct {
		Total   int      `json:"total"`
		Devices []Device `json:"devices"`
	} `json:"response"`
}

// apiResponse is a generic wrapper for { "response": T } API responses.
type apiResponse[T any] struct {
	Response T `json:"response"`
}

// apiErrorResponse is the standard error response from the Remnawave API.
// В 3.x схема стала типизированной (RemnawaveNotFoundErrorDto и др.), но
// остаётся плоской: {timestamp, path, message, errorCode}.
type apiErrorResponse struct {
	Message   string `json:"message"`
	ErrorCode string `json:"errorCode"`
}

// internalSquadItem is a single squad in the internal squads response.
type internalSquadItem struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// internalSquadsResponse is the response body for GET /api/internal-squads.
type internalSquadsResponse struct {
	InternalSquads []internalSquadItem `json:"internalSquads"`
}

// CreateUserRequest is the request body for POST /api/users (CreateUserBodyDto).
// Кастомный uuid панель больше не принимает.
type CreateUserRequest struct {
	Username             string    `json:"username"`
	ExpireAt             time.Time `json:"expireAt"`
	Status               string    `json:"status,omitempty"`
	TrafficLimitBytes    *int64    `json:"trafficLimitBytes,omitempty"`
	TrafficLimitStrategy string    `json:"trafficLimitStrategy,omitempty"`
	HwidDeviceLimit      *int      `json:"hwidDeviceLimit,omitempty"`
	// nil — поле не уходит в JSON; non-nil (в т.ч. пустой слайс) — массив UUID.
	ActiveInternalSquads *[]uuid.UUID `json:"activeInternalSquads,omitempty"`
	ExternalSquadUuid    *uuid.UUID   `json:"externalSquadUuid,omitempty"`
	Tag                  *string      `json:"tag,omitempty"`
	TelegramID           *int64       `json:"telegramId,omitempty"`
	Description          *string      `json:"description,omitempty"`
}

// UpdateUserRequest is the request body for PATCH /api/users (UpdateUserBodyDto).
// Идентификатор — числовой ID (был UUID до 3.0.0).
// Status в 3.x принимает только ACTIVE|DISABLED: LIMITED/EXPIRED вычисляются панелью.
type UpdateUserRequest struct {
	ID                   *int64     `json:"id,omitempty"`
	Username             *string    `json:"username,omitempty"`
	TelegramID           *int64     `json:"telegramId,omitempty"`
	Status               string     `json:"status,omitempty"`
	ExpireAt             *time.Time `json:"expireAt,omitempty"`
	TrafficLimitBytes    *int64     `json:"trafficLimitBytes,omitempty"`
	TrafficLimitStrategy string     `json:"trafficLimitStrategy,omitempty"`
	HwidDeviceLimit      *int       `json:"hwidDeviceLimit,omitempty"`
	// nil — не менять сквады; &[] — снять все внутренние сквады (пустой JSON-массив).
	ActiveInternalSquads *[]uuid.UUID `json:"activeInternalSquads,omitempty"`
	ExternalSquadUuid    *uuid.UUID   `json:"externalSquadUuid,omitempty"`
	Tag                  *string      `json:"tag,omitempty"`
	Description          *string      `json:"description,omitempty"`
}

// deleteUserDeviceRequest — тело POST /api/hwid/devices/delete.
type deleteUserDeviceRequest struct {
	UserID int64  `json:"userId"`
	Hwid   string `json:"hwid"`
}

// deleteAllUserDevicesRequest — тело POST /api/hwid/devices/delete-all.
type deleteAllUserDevicesRequest struct {
	UserID int64 `json:"userId"`
}

// revokeUserSubscriptionRequest — тело POST /api/users/{userId}/actions/revoke.
// Пустой объект: shortUuid генерирует панель, revokeOnlyPasswords=false.
type revokeUserSubscriptionRequest struct{}

// extendUserRequest — тело POST /api/users/{userId}/actions/extend.
type extendUserRequest struct {
	Days int `json:"days"`
}
