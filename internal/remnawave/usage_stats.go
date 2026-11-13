package remnawave

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// UserUsageSeries — расход трафика пользователя по дням за период.
//
// GET /api/bandwidth-stats/users/{userId}?start=YYYY-MM-DD&end=YYYY-MM-DD
// («Get User Usage by Range»). Панель сама отдаёт готовый sparklineData,
// поэтому агрегировать series на нашей стороне не нужно.
type UserUsageSeries struct {
	// Categories — подписи точек, по одной на каждое значение Sparkline.
	Categories []string `json:"categories"`
	// Sparkline — расход по дням. Единицы те же, что и в остальном API, — байты.
	Sparkline []float64 `json:"sparklineData"`
	// Series — расход по нодам: на каждую ноду байты по дням и сумма за период.
	// Панель отдаёт их на 2.8 и 3.x одинаково; нода без включённого «Отслеживания
	// трафика» сюда не попадает.
	Series []NodeUsageSeries `json:"series"`
}

// NodeUsageSeries — расход одной ноды за период.
type NodeUsageSeries struct {
	UUID  string    `json:"uuid"`
	Name  string    `json:"name"`
	Total float64   `json:"total"`
	Data  []float64 `json:"data"`
}

// NodesTotal — суммарный расход по перечисленным нодам (uuid в нижнем регистре).
func (u UserUsageSeries) NodesTotal(nodeUUIDs map[string]struct{}) int64 {
	var sum float64
	for _, n := range u.Series {
		if _, ok := nodeUUIDs[strings.ToLower(n.UUID)]; ok {
			sum += n.Total
		}
	}
	return int64(sum)
}

// Total — суммарный расход за период.
func (u UserUsageSeries) Total() float64 {
	var sum float64
	for _, v := range u.Sparkline {
		sum += v
	}
	return sum
}

// GetUserUsageByRange возвращает расход трафика по дням за [start, end].
//
// Границы включительные и передаются датами без времени: панель считает по
// суткам. Пустой ряд — законный ответ (пользователь ничего не потратил),
// это не ошибка.
func (r *Client) GetUserUsageByRange(ctx context.Context, userID int64, start, end time.Time) (*UserUsageSeries, error) {
	if userID <= 0 {
		return nil, ErrUserNotFound
	}
	if end.Before(start) {
		return nil, fmt.Errorf("usage range: end %s is before start %s", end.Format(time.DateOnly), start.Format(time.DateOnly))
	}

	query := url.Values{}
	query.Set("start", start.Format(time.DateOnly))
	query.Set("end", end.Format(time.DateOnly))

	path := "/api/bandwidth-stats/users/" + strconv.FormatInt(userID, 10) + "?" + query.Encode()

	var resp apiResponse[UserUsageSeries]
	if err := r.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Response, nil
}
