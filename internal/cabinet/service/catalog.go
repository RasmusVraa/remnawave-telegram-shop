// Package service собирает публичные сервисы кабинета, которые нельзя отнести
// к «авторизации» (auth/service) и к «оплатам» (cabinet/payments, появится на
// Этапе 5). Сейчас здесь только Catalog — публичная витрина тарифов.
//
// Сервисы этого пакета:
//
//   - не зависят от HTTP-слоя (никаких http.Request / ResponseWriter);
//   - переиспользуют существующие расчёты из бота (config + database) без
//     дублирования бизнес-логики;
//   - возвращают «плоские» DTO, пригодные для JSON-сериализации без адаптеров.
package service

import (
	"context"
	"fmt"

	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

// Режимы продаж. Константы, чтобы не ошибиться в сравнении строк.
const (
	SalesModeClassic = "classic"
	SalesModeTariffs = "tariffs"
)

// supportedMonths — четыре периода, которые поддерживает бот (см. tariff_price
// и PRICE_*). Если в проекте появится ещё один период (например, 24 мес),
// эту константу придётся обновить вместе с самим PRICE_24 / amount_rub(months=24).
var supportedMonths = [4]int{1, 3, 6, 12}

// Catalog — источник публичной витрины тарифов для web-кабинета.
//
// Режим выбирается через config.SalesMode(): в «tariffs» мы читаем активные
// строки из TariffRepository, в «classic» — синтезируем единственный тариф
// из env (PRICE_*, TRAFFIC_LIMIT, PAID_HWID_LIMIT, ...).
type Catalog struct {
	tariffs *database.TariffRepository
}

// NewCatalog — конструктор. tariffs может быть nil только в classic-режиме;
// в tariffs-режиме при nil ответ списка тарифов будет пустой, и клиент поймёт,
// что витрина выключена.
func NewCatalog(tariffs *database.TariffRepository) *Catalog {
	return &Catalog{tariffs: tariffs}
}

// Price — цена за один период. monthly_base_rub нужен фронту для подписи
// «экономия N%» без дублирования расчёта: base = price(1 мес) * months.
// amount_stars намеренно не публикуется: кабинет не принимает Telegram Stars
// как метод оплаты (Этап 5 — только YooKassa и CryptoPay).
type Price struct {
	Months         int `json:"months"`
	AmountRub      int `json:"amount_rub"`
	MonthlyBaseRub int `json:"monthly_base_rub"`
}

// LocationView — площадка тарифа «свой сервер». SoldOut нельзя выбрать при оплате.
type LocationView struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	SoldOut     bool   `json:"sold_out"`
	CountryCode string `json:"country_code,omitempty"`
}

// TariffView — один тариф в ответе витрины.
//
// id == 0 и slug == "classic" зарезервированы за виртуальным тарифом в режиме
// classic — чтобы фронту было за что закрепиться при выборе.
type TariffView struct {
	ID                        int64          `json:"id"`
	Slug                      string         `json:"slug"`
	Name                      *string        `json:"name,omitempty"`
	Description               *string        `json:"description,omitempty"`
	DescriptionDetail         *string        `json:"description_detail,omitempty"`
	DeviceLimit               int            `json:"device_limit"`
	TrafficLimitBytes         int64          `json:"traffic_limit_bytes"`
	TrafficLimitResetStrategy string         `json:"traffic_limit_reset_strategy"`
	Prices                    []Price        `json:"prices"`
	Locations                 []LocationView `json:"locations,omitempty"`
}

// Response — корневой объект `GET /cabinet/api/tariffs`.
type Response struct {
	SalesMode    string       `json:"sales_mode"`
	Currency     string       `json:"currency"` // MVP: всегда "RUB" (CryptoPay конвертирует)
	ShowSavings  bool         `json:"show_savings"`
	PriceDisplay string       `json:"price_display"` // monthly | marketing (CABINET_TARIFF_PRICE_DISPLAY)
	SavingsBadge string       `json:"savings_badge"` // none | corner | old_price (CABINET_TARIFF_SAVINGS_BADGE)
	Tariffs      []TariffView `json:"tariffs"`
}

// Get собирает витрину. Ошибку возвращает только при сбое чтения БД в
// tariffs-режиме — в classic-режиме DB не трогается.
func (c *Catalog) Get(ctx context.Context) (*Response, error) {
	resp := &Response{
		SalesMode:    config.SalesMode(),
		Currency:     "RUB",
		ShowSavings:  config.ShowLongTermSavingsPercent(),
		PriceDisplay: cabcfg.TariffPriceDisplay(),
		SavingsBadge: cabcfg.TariffSavingsBadge(),
	}

	switch resp.SalesMode {
	case SalesModeTariffs:
		if c.tariffs == nil {
			resp.Tariffs = []TariffView{}
			return resp, nil
		}
		items, err := c.tariffs.ListActive(ctx)
		if err != nil {
			return nil, fmt.Errorf("catalog: list tariffs: %w", err)
		}
		resp.Tariffs = make([]TariffView, 0, len(items))
		for _, t := range items {
			prices, err := c.tariffs.ListPricesForTariff(ctx, t.ID)
			if err != nil {
				return nil, fmt.Errorf("catalog: list prices for tariff %d: %w", t.ID, err)
			}
			locs, lerr := c.tariffs.ListLocations(ctx, t.ID)
			if lerr != nil {
				return nil, fmt.Errorf("catalog: list locations for tariff %d: %w", t.ID, lerr)
			}
			resp.Tariffs = append(resp.Tariffs, buildTariffsModeView(t, prices, locs))
		}
	default:
		// classic — одна виртуальная карточка.
		resp.Tariffs = []TariffView{buildClassicView()}
	}
	return resp, nil
}

// buildTariffsModeView превращает строку tariff + список tariff_price в публичный вид.
// Порядок строк определяется базой (ListPricesForTariff ORDER BY months ASC).
func buildTariffsModeView(t database.Tariff, prices []database.TariffPrice, locs []database.TariffLocation) TariffView {
	price1 := 0
	for _, p := range prices {
		if p.Months == 1 {
			price1 = p.AmountRub
			break
		}
	}
	v := TariffView{
		ID:                        t.ID,
		Slug:                      t.Slug,
		Name:                      t.Name,
		Description:               t.Description,
		DescriptionDetail:         t.DescriptionDetail,
		DeviceLimit:               t.DeviceLimit,
		TrafficLimitBytes:         t.TrafficLimitBytes,
		TrafficLimitResetStrategy: t.TrafficLimitResetStrategy,
		Prices:                    make([]Price, 0, len(prices)),
	}
	for _, p := range prices {
		v.Prices = append(v.Prices, Price{
			Months:         p.Months,
			AmountRub:      p.AmountRub,
			MonthlyBaseRub: price1 * p.Months,
		})
	}
	if len(locs) > 0 {
		v.Locations = make([]LocationView, 0, len(locs))
		for _, loc := range locs {
			view := LocationView{ID: loc.ID, Name: loc.Name, SoldOut: loc.SoldOut}
			if loc.CountryCode != nil {
				view.CountryCode = *loc.CountryCode
			}
			v.Locations = append(v.Locations, view)
		}
	}
	return v
}

// buildClassicView собирает виртуальный тариф для режима classic.
// Берём всё из env через config.* — теже цифры, что видит бот.
func buildClassicView() TariffView {
	strat := config.TrafficLimitResetStrategy()
	if strat == "" {
		strat = "month"
	}
	price1 := config.Price1()
	v := TariffView{
		ID:                        0,
		Slug:                      SalesModeClassic,
		DeviceLimit:               config.PaidHwidLimit(),
		TrafficLimitBytes:         int64(config.TrafficLimit()),
		TrafficLimitResetStrategy: strat,
		Prices:                    make([]Price, 0, len(supportedMonths)),
	}
	for _, m := range supportedMonths {
		v.Prices = append(v.Prices, Price{
			Months:         m,
			AmountRub:      config.Price(m),
			MonthlyBaseRub: price1 * m,
		})
	}
	return v
}
