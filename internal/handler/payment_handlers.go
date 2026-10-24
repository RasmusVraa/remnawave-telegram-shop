package handler

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/loyalty"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/remnawave"
)

func (h Handler) BuyCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery.Message.Message
	langCode := update.CallbackQuery.From.LanguageCode
	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.Chat.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("customer not exist", "chatID", callback.Chat.ID, "error", err)
		return
	}

	if config.SalesMode() == "tariffs" && h.tariffRepository != nil {
		tariffs, err := h.tariffRepository.ListActive(ctx)
		if err != nil {
			slog.Error("list tariffs", "error", err)
		} else if len(tariffs) > 0 {
			if len(tariffs) == 1 {
				h.renderTariffMonthChoice(ctx, b, update, &tariffs[0], langCode, customer, CallbackStart)
				return
			}
			var rows [][]models.InlineKeyboardButton
			for _, t := range tariffs {
				label := t.Slug
				if t.Name != nil && strings.TrimSpace(*t.Name) != "" {
					label = strings.TrimSpace(*t.Name)
				}
				rows = append(rows, []models.InlineKeyboardButton{
					{Text: label, CallbackData: fmt.Sprintf("%s?tid=%d", CallbackSell, t.ID)},
				})
			}
			rows = append(rows, []models.InlineKeyboardButton{
				h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: CallbackStart}),
			})
			infoKey := h.resolvePricingInfoKey(ctx, customer, 0)
			text := h.pricingInfoTextWithDiscount(ctx, langCode, customer, infoKey)
			if bt, err := h.buildTariffBuySelectionHTML(ctx, langCode, customer); err == nil {
				text = bt
			}
			err = SendOrEditAfterInlineCallback(ctx, b, update, text, models.ParseModeHTML, models.InlineKeyboardMarkup{
				InlineKeyboard: rows,
			}, nil)
			logEditError("Error sending buy message (tariffs)", err)
			return
		}
	}

	var priceButtons []models.InlineKeyboardButton
	price1Rub := config.Price1()

	if price1Rub > 0 {
		a := price1Rub
		priceButtons = append(priceButtons, h.monthPriceButton(langCode, "month_1", a, 1, price1Rub, models.InlineKeyboardButton{
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, 1, a),
		}))
	}

	if config.Price3() > 0 {
		a := config.Price3()
		priceButtons = append(priceButtons, h.monthPriceButton(langCode, "month_3", a, 3, price1Rub, models.InlineKeyboardButton{
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, 3, a),
		}))
	}

	if config.Price6() > 0 {
		a := config.Price6()
		priceButtons = append(priceButtons, h.monthPriceButton(langCode, "month_6", a, 6, price1Rub, models.InlineKeyboardButton{
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, 6, a),
		}))
	}

	if config.Price12() > 0 {
		a := config.Price12()
		priceButtons = append(priceButtons, h.monthPriceButton(langCode, "month_12", a, 12, price1Rub, models.InlineKeyboardButton{
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, 12, a),
		}))
	}

	keyboard := [][]models.InlineKeyboardButton{}

	if len(priceButtons) == 4 {
		keyboard = append(keyboard, priceButtons[:2])
		keyboard = append(keyboard, priceButtons[2:])
	} else if len(priceButtons) > 0 {
		keyboard = append(keyboard, priceButtons)
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: CallbackStart}),
	})

	infoKey := h.resolvePricingInfoKey(ctx, customer, 0)
	err = SendOrEditAfterInlineCallback(ctx, b, update, h.pricingInfoTextWithDiscount(ctx, langCode, customer, infoKey), models.ParseModeHTML, models.InlineKeyboardMarkup{
		InlineKeyboard: keyboard,
	}, nil)
	logEditError("Error sending buy message", err)
}

// renderTariffMonthChoice показывает кнопки периодов для одного тарифа (цены из БД).
// backCallback — куда ведёт «Назад»: CallbackStart при единственном тарифе из «Купить», иначе CallbackBuy (список тарифов).
func (h Handler) renderTariffMonthChoice(ctx context.Context, b *bot.Bot, update *models.Update, tariff *database.Tariff, langCode string, customer *database.Customer, backCallback string) {
	prices, err := h.tariffRepository.ListPricesForTariff(ctx, tariff.ID)
	if err != nil {
		slog.Error("list tariff prices", "error", err)
		return
	}
	price1Rub := 0
	for _, p := range prices {
		if p.Months == 1 && p.AmountRub > 0 {
			price1Rub = p.AmountRub
			break
		}
	}

	var priceButtons []models.InlineKeyboardButton
	for _, p := range prices {
		if p.AmountRub <= 0 {
			continue
		}
		priceButtons = append(priceButtons, h.monthPriceButton(langCode, monthButtonKey(p.Months), p.AmountRub, p.Months, price1Rub, models.InlineKeyboardButton{
			CallbackData: fmt.Sprintf("%s?tid=%d&month=%d&amount=%d", CallbackSell, tariff.ID, p.Months, p.AmountRub),
		}))
	}
	keyboard := [][]models.InlineKeyboardButton{}
	if len(priceButtons) == 4 {
		keyboard = append(keyboard, priceButtons[:2])
		keyboard = append(keyboard, priceButtons[2:])
	} else if len(priceButtons) > 0 {
		keyboard = append(keyboard, priceButtons)
	}
	keyboard = append(keyboard, []models.InlineKeyboardButton{
		h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: backCallback}),
	})
	text := h.buildTariffPeriodChoiceHTML(ctx, langCode, tariff)
	err = SendOrEditAfterInlineCallback(ctx, b, update, text, models.ParseModeHTML, models.InlineKeyboardMarkup{
		InlineKeyboard: keyboard,
	}, nil)
	logEditError("Error sending tariff month message", err)
}

func monthButtonKey(months int) string {
	switch months {
	case 1:
		return "month_1"
	case 3:
		return "month_3"
	case 6:
		return "month_6"
	case 12:
		return "month_12"
	default:
		return "month_1"
	}
}

// monthPriceButton текст кнопки периода: шаблон из ключей month_1…month_12 с одним %d (сумма в ₽).
// При SHOW_LONG_TERM_SAVINGS_PERCENT к периодам >1 мес добавляется (-N%) к базе «цена за 1 мес × месяцев» (price1Rub).
func (h Handler) monthPriceButton(lang, key string, amountRub int, months int, price1Rub int, cb models.InlineKeyboardButton) models.InlineKeyboardButton {
	cb.Text = fmt.Sprintf(h.translation.GetText(lang, key), amountRub)
	if config.ShowLongTermSavingsPercent() && months > 1 {
		if pct := longTermSavingsPercent(price1Rub, months, amountRub); pct > 0 {
			cb.Text += fmt.Sprintf(" (-%d%%)", pct)
		}
	}
	return cb
}

// longTermSavingsPercent — доля экономии относительно суммы N×price1Rub (месячная цена из classic или тарифа).
func longTermSavingsPercent(price1Rub, months, totalRub int) int {
	if months <= 1 || price1Rub <= 0 || totalRub <= 0 {
		return 0
	}
	baseline := price1Rub * months
	if totalRub >= baseline {
		return 0
	}
	return int(math.Round(float64(baseline-totalRub) * 100.0 / float64(baseline)))
}

func (h Handler) SellCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery.Message.Message
	callbackQuery := parseCallbackData(update.CallbackQuery.Data)
	langCode := update.CallbackQuery.From.LanguageCode
	month := callbackQuery["month"]
	amount := callbackQuery["amount"]
	extraChoice := callbackQuery["extra"]
	tidStr := callbackQuery["tid"]

	var keyboard [][]models.InlineKeyboardButton
	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.Chat.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("customer not exist", "chatID", callback.Chat.ID, "error", err)
		return
	}

	if tidStr != "" && config.SalesMode() == "tariffs" && h.tariffRepository != nil {
		tid := parseInt64Safe(tidStr)
		if tid > 0 && month == "" {
			tariff, err := h.tariffRepository.GetByID(ctx, tid)
			if err != nil || tariff == nil {
				slog.Error("tariff for sell", "error", err, "tid", tid)
				return
			}
			h.renderTariffMonthChoice(ctx, b, update, tariff, langCode, customer, CallbackBuy)
			return
		}
	}

	if tidStr != "" && month != "" && config.SalesMode() == "tariffs" && h.tariffRepository != nil {
		monthInt := parseIntSafe(month)
		tid := parseInt64Safe(tidStr)
		if tid > 0 && monthInt > 0 {
			kind, _, _, _, err := payment.ResolveTariffPurchase(ctx, h.tariffRepository, customer, tid, monthInt, false)
			if err != nil {
				slog.Error("tariff sell resolve", "error", err)
			} else if kind == payment.TariffCheckoutDowngrade && callbackQuery["dg"] != "1" {
				warnText := h.translation.GetText(langCode, "tariff_downgrade_early_warning")
				confirmCb := fmt.Sprintf("%s?tid=%d&month=%d&amount=%s&dg=1", CallbackSell, tid, monthInt, amount)
				_, err = editCallbackOriginToHTMLText(ctx, b, callback, warnText, models.ParseModeHTML, models.InlineKeyboardMarkup{
					InlineKeyboard: [][]models.InlineKeyboardButton{
						{
							h.translation.WithButton(langCode, "tariff_downgrade_confirm_button", models.InlineKeyboardButton{CallbackData: confirmCb}),
						},
						{
							h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s?tid=%d", CallbackSell, tid)}),
						},
					},
				}, nil)
				logEditError("downgrade warning", err)
				return
			}
		}
	}

	if extraChoice == "" {
		if err := h.cleanupExpiredExtraHwid(ctx, customer); err != nil {
			slog.Error("Error cleaning expired extra hwid", "error", err)
			return
		}
		if config.HwidExtraDevicesEnabled() && customer.ExtraHwid > 0 && customer.ExtraHwidExpiresAt != nil && customer.ExtraHwidExpiresAt.After(time.Now()) {
			monthInt := parseIntSafe(month)
			promptText := fmt.Sprintf(h.translation.GetText(langCode, "hwid_renew_prompt"), customer.ExtraHwid, monthInt, config.HwidAddPrice()*customer.ExtraHwid*monthInt)
			sellRenew := fmt.Sprintf("%s?month=%s&amount=%s", CallbackSell, month, amount)
			if tidStr != "" {
				sellRenew = fmt.Sprintf("%s?tid=%s&month=%s&amount=%s", CallbackSell, tidStr, month, amount)
			}
			if callbackQuery["dg"] == "1" {
				sellRenew += "&dg=1"
			}
			_, err = editCallbackOriginToHTMLText(ctx, b, callback, promptText, models.ParseModeHTML, models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{
					{
						h.translation.WithButton(langCode, "hwid_renew_confirm_button", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s&extra=%d", sellRenew, customer.ExtraHwid)}),
					},
					{
						h.translation.WithButton(langCode, "hwid_renew_cancel_button", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s&extra=%d", sellRenew, 0)}),
					},
					{
						h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: CallbackBuy}),
					},
				},
			}, nil)
			logEditError("Error sending renew prompt", err)
			return
		}
	}

	extraCount := 0
	if extraChoice != "" {
		extraCount = parseIntSafe(extraChoice)
	}

	if config.IsCryptoPayEnabled() {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			h.translation.WithButton(langCode, "crypto_button", models.InlineKeyboardButton{CallbackData: paymentCallbackQuery(tidStr, month, string(database.InvoiceTypeCrypto), amount, extraCount)}),
		})
	}

	if config.IsYookasaEnabled() {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			h.translation.WithButton(langCode, "card_button", models.InlineKeyboardButton{CallbackData: paymentCallbackQuery(tidStr, month, string(database.InvoiceTypeYookasa), amount, extraCount)}),
		})
	}

	keyboard = h.appendPlategaPaymentRows(keyboard, langCode, tidStr, month, amount, extraCount)

	if config.IsTelegramStarsEnabled() {
		shouldShowStarsButton := true

		if config.RequirePaidPurchaseForStars() {
			customer, err := h.customerRepository.FindByTelegramId(ctx, callback.Chat.ID)
			if err != nil {
				slog.Error("Error finding customer for stars check", err)
				shouldShowStarsButton = false
			} else if customer != nil {
				paidPurchase, err := h.purchaseRepository.FindSuccessfulPaidPurchaseByCustomer(ctx, customer.ID)
				if err != nil {
					slog.Error("Error checking paid purchase", err)
					shouldShowStarsButton = false
				} else if paidPurchase == nil {
					shouldShowStarsButton = false
				}
			} else {
				shouldShowStarsButton = false
			}
		}

		if shouldShowStarsButton {
			keyboard = append(keyboard, []models.InlineKeyboardButton{
				h.translation.WithButton(langCode, "stars_button", models.InlineKeyboardButton{CallbackData: paymentCallbackQuery(tidStr, month, string(database.InvoiceTypeTelegram), amount, extraCount)}),
			})
		}
	}

	if config.GetTributeWebHookUrl() != "" {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			h.translation.WithButton(langCode, "tribute_button", models.InlineKeyboardButton{URL: config.GetTributePaymentUrl()}),
		})
	}

	backSell := CallbackBuy
	if tidStr != "" {
		backSell = fmt.Sprintf("%s?tid=%s", CallbackSell, tidStr)
	}
	keyboard = append(keyboard, []models.InlineKeyboardButton{
		h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: backSell}),
	})

	textKey := h.resolvePricingInfoKey(ctx, customer, extraCount)
	textBody := h.pricingInfoTextWithDiscount(ctx, langCode, customer, textKey)
	if tidStr != "" && config.SalesMode() == "tariffs" && h.tariffRepository != nil {
		textBody = h.appendPendingDiscountToPricingHTML(ctx, langCode, customer, h.buildTariffPaymentMethodsHTML(langCode))
		if month != "" {
			monthInt := parseIntSafe(month)
			tid := parseInt64Safe(tidStr)
			if tid > 0 && monthInt > 0 {
				kind, _, pk, earlyDG, rerr := payment.ResolveTariffPurchase(ctx, h.tariffRepository, customer, tid, monthInt, false)
				showSwitchBanner := rerr == nil && customer.CurrentTariffID != nil && *customer.CurrentTariffID > 0 &&
					((kind == payment.TariffCheckoutUpgrade && pk == database.PurchaseKindTariffUpgrade) ||
						(kind == payment.TariffCheckoutDowngrade && earlyDG))
				if showSwitchBanner {
					tpNew, e1 := h.tariffRepository.GetPrice(ctx, tid, monthInt)
					tpOld, e2 := h.tariffRepository.GetPrice(ctx, *customer.CurrentTariffID, monthInt)
					if e1 == nil && e2 == nil && tpNew != nil && tpOld != nil {
						now := time.Now().UTC()
						bonus := payment.ComputeUpgradeBonusDays(customer, tpOld, tpNew, monthInt, now)
						dim := config.DaysInMonth()
						if dim <= 0 {
							dim = 30
						}
						baseDays := monthInt * dim
						totalDays := baseDays + bonus
						noticeKey := "tariff_downgrade_sell_notice"
						if kind == payment.TariffCheckoutUpgrade {
							noticeKey = "tariff_upgrade_sell_notice"
						}
						banner := fmt.Sprintf(h.translation.GetText(langCode, noticeKey), baseDays, bonus, totalDays)
						textBody = banner + "\n\n" + textBody
					}
				}
			}
		}
	}
	_, err = editCallbackOriginToHTMLText(ctx, b, callback, textBody, models.ParseModeHTML, models.InlineKeyboardMarkup{
		InlineKeyboard: keyboard,
	}, nil)
	logEditError("Error sending sell message", err)
}

func (h Handler) PaymentCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery.Message.Message
	callbackQuery := parseCallbackData(update.CallbackQuery.Data)
	month, err := strconv.Atoi(callbackQuery["month"])
	if err != nil {
		slog.Error("Error getting month from query", err)
		return
	}

	invoiceType := database.InvoiceType(callbackQuery["invoiceType"])
	extra := parseIntSafe(callbackQuery["extra"])
	if !config.HwidExtraDevicesEnabled() && extra > 0 {
		extra = 0
	}
	tidStr := callbackQuery["tid"]

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.Chat.ID)
	if err != nil {
		slog.Error("Error finding customer", err)
		return
	}
	if customer == nil {
		slog.Error("customer not exist", "chatID", callback.Chat.ID, "error", err)
		return
	}

	var price int
	var tariffID *int64
	var tariffExtras *payment.TariffPurchaseExtras

	if tidStr != "" && config.SalesMode() == "tariffs" && h.tariffRepository != nil {
		tid := parseInt64Safe(tidStr)
		if tid <= 0 {
			slog.Error("invalid tariff id in payment callback")
			return
		}
		tidCopy := tid
		tariffID = &tidCopy
		invoiceStars := invoiceType == database.InvoiceTypeTelegram
		_, amount, pk, early, err := payment.ResolveTariffPurchase(ctx, h.tariffRepository, customer, tid, month, invoiceStars)
		if err != nil {
			slog.Error("tariff resolve for payment", "error", err, "tid", tid, "month", month)
			return
		}
		price = amount
		if extra > 0 {
			if invoiceStars {
				price += config.HwidAddStarsPrice() * extra * month
			} else {
				price += config.HwidAddPrice() * extra * month
			}
		}
		tariffExtras = &payment.TariffPurchaseExtras{Kind: pk, IsEarlyDowngrade: early}
	} else {
		if invoiceType == database.InvoiceTypeTelegram {
			price = config.StarsPrice(month)
			if extra > 0 {
				price += config.HwidAddStarsPrice() * extra * month
			}
		} else {
			price = config.Price(month)
			if extra > 0 {
				price += config.HwidAddPrice() * extra * month
			}
		}
	}

	var chosenLoc *database.TariffLocation
	if tariffID != nil && h.tariffRepository != nil {
		locs, lerr := h.tariffRepository.ListLocations(ctx, *tariffID)
		if lerr != nil {
			slog.Error("tariff locations", "error", lerr, "tid", *tariffID)
			return
		}
		if len(locs) > 0 {
			langCode := update.CallbackQuery.From.LanguageCode
			locID := parseInt64Safe(callbackQuery["l"])
			if locID == 0 {
				h.renderTariffLocationChoice(ctx, b, update, locs, tidStr, strconv.Itoa(month), string(invoiceType), extra, langCode)
				return
			}
			for i := range locs {
				if locs[i].ID != locID {
					continue
				}
				if locs[i].SoldOut {
					_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
						CallbackQueryID: update.CallbackQuery.ID,
						Text:            h.translation.GetText(langCode, "tariff_location_sold_out_alert"),
						ShowAlert:       true,
					})
					return
				}
				chosenLoc = &locs[i]
				break
			}
			if chosenLoc == nil {
				return
			}
		}
	}

	// Скидка от pending-промокода на полную сумму счёта за выбранный период (в т.ч. апгрейд/даунгрейд тарифа).
	meta := h.checkoutPromoMeta(ctx, customer, invoiceType, &price)

	ctxWithUsername := context.WithValue(ctx, remnawave.CtxKeyUsername, update.CallbackQuery.From.Username)
	var paymentURL string
	var purchaseId int64
	if extra > 0 {
		paymentURL, purchaseId, err = h.paymentService.CreatePurchaseWithExtra(ctxWithUsername, float64(price), month, extra, customer, invoiceType, meta, tariffID, tariffExtras)
	} else {
		paymentURL, purchaseId, err = h.paymentService.CreatePurchase(ctxWithUsername, float64(price), month, customer, invoiceType, meta, tariffID, tariffExtras)
	}
	if err != nil {
		slog.Error("Error creating payment", err)
		langCode := update.CallbackQuery.From.LanguageCode
		backSell := fmt.Sprintf("%s?month=%d&amount=%d&extra=%d", CallbackSell, month, price, extra)
		if tidStr != "" {
			backSell = fmt.Sprintf("%s?tid=%s&month=%d&amount=%d&extra=%d", CallbackSell, tidStr, month, price, extra)
		}
		h.notifyPaymentProviderUnavailable(ctx, b, update, langCode, backSell)
		return
	}
	if chosenLoc != nil && purchaseId > 0 && h.purchaseRepository != nil {
		if err := h.purchaseRepository.SetTariffLocation(ctx, purchaseId, chosenLoc.ID, chosenLoc.Name); err != nil {
			slog.Error("save tariff location", "error", err, "purchase", purchaseId)
		}
	}

	langCode := update.CallbackQuery.From.LanguageCode

	backSell := fmt.Sprintf("%s?month=%d&amount=%d&extra=%d", CallbackSell, month, price, extra)
	if tidStr != "" {
		backSell = fmt.Sprintf("%s?tid=%s&month=%d&amount=%d&extra=%d", CallbackSell, tidStr, month, price, extra)
	}
	replyMarkup := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				h.translation.WithButton(langCode, "pay_button", models.InlineKeyboardButton{URL: paymentURL}),
				h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: backSell}),
			},
		},
	}

	var message *models.Message
	if tidStr != "" && config.SalesMode() == "tariffs" && h.tariffRepository != nil {
		tid := parseInt64Safe(tidStr)
		invoiceStars := invoiceType == database.InvoiceTypeTelegram
		switchBonusDays := 0
		switchTariffKind := ""
		if tariffExtras != nil && customer.CurrentTariffID != nil && *customer.CurrentTariffID > 0 {
			kind, _, pk, _, rerr := payment.ResolveTariffPurchase(ctx, h.tariffRepository, customer, tid, month, invoiceStars)
			if rerr == nil &&
				((kind == payment.TariffCheckoutUpgrade && pk == database.PurchaseKindTariffUpgrade) ||
					kind == payment.TariffCheckoutDowngrade) {
				tpNew, e1 := h.tariffRepository.GetPrice(ctx, tid, month)
				tpOld, e2 := h.tariffRepository.GetPrice(ctx, *customer.CurrentTariffID, month)
				if e1 == nil && e2 == nil && tpNew != nil && tpOld != nil {
					switchBonusDays = payment.ComputeUpgradeBonusDays(customer, tpOld, tpNew, month, time.Now().UTC())
					if kind == payment.TariffCheckoutUpgrade && pk == database.PurchaseKindTariffUpgrade {
						switchTariffKind = "upgrade"
					} else if kind == payment.TariffCheckoutDowngrade {
						switchTariffKind = "downgrade"
					}
				}
			}
		}
		textMsg, sumErr := h.buildTariffCheckoutSummaryHTML(ctx, langCode, customer, tid, month, price, invoiceStars, switchBonusDays, extra, switchTariffKind)
		if sumErr != nil {
			slog.Error("build tariff payment link", "error", sumErr)
			textMsg = h.translation.GetText(langCode, "payment_tariff_screen_fallback")
		}
		message, err = editCallbackOriginToHTMLText(ctx, b, callback, textMsg, models.ParseModeHTML, replyMarkup, nil)
	} else {
		message, err = b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
			ChatID:      callback.Chat.ID,
			MessageID:   callback.ID,
			ReplyMarkup: replyMarkup,
		})
	}
	if err != nil {
		logEditError("Error updating sell message", err)
		return
	}
	h.cache.Set(purchaseId, message.ID)
}

func (h Handler) PreCheckoutCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, err := b.AnswerPreCheckoutQuery(ctx, &bot.AnswerPreCheckoutQueryParams{
		PreCheckoutQueryID: update.PreCheckoutQuery.ID,
		OK:                 true,
	})
	if err != nil {
		slog.Error("Error sending answer pre checkout query", err)
	}
}

func (h Handler) SuccessPaymentHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	payload := strings.Split(update.Message.SuccessfulPayment.InvoicePayload, "&")
	purchaseId, err := strconv.Atoi(payload[0])
	username := payload[1]
	if err != nil {
		slog.Error("Error parsing purchase id", err)
		return
	}

	ctxWithUsername := context.WithValue(ctx, remnawave.CtxKeyUsername, username)
	sp := update.Message.SuccessfulPayment
	ctxPaid := payment.WithStarsNotifyMeta(ctxWithUsername, payment.StarsNotifyMeta{
		TelegramPaymentChargeID: sp.TelegramPaymentChargeID,
		ProviderPaymentChargeID: sp.ProviderPaymentChargeID,
		TotalAmount:             sp.TotalAmount,
		Currency:                sp.Currency,
	})
	err = h.paymentService.ProcessPurchaseById(ctxPaid, int64(purchaseId))
	if err != nil {
		slog.Error("Error processing purchase", err)
	}
}

func parseIntSafe(value string) int {
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func parseInt64Safe(value string) int64 {
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

// paymentCallbackQuery — данные callback для оплаты; для тарифов цена берётся из БД, amount не передаётся.
func plategaInvoiceEnabled(it database.InvoiceType) bool {
	if !config.IsPlategaEnabled() {
		return false
	}
	switch it {
	case database.InvoiceTypePlategaSBP:
		return config.IsPlategaSBPEnabled()
	case database.InvoiceTypePlategaCards:
		return config.IsPlategaCardsEnabled()
	case database.InvoiceTypePlategaAcquiring:
		return config.IsPlategaAcquiringEnabled()
	case database.InvoiceTypePlategaWorldwide:
		return config.IsPlategaWorldwideEnabled()
	case database.InvoiceTypePlategaCrypto:
		return config.IsPlategaCryptoEnabled()
	default:
		return false
	}
}

func (h Handler) appendPlategaPaymentRows(keyboard [][]models.InlineKeyboardButton, langCode, tidStr, month, amount string, extraCount int) [][]models.InlineKeyboardButton {
	type row struct {
		inv database.InvoiceType
		key string
	}
	for _, r := range []row{
		{database.InvoiceTypePlategaSBP, "platega_sbp_button"},
		{database.InvoiceTypePlategaCards, "platega_cards_button"},
		{database.InvoiceTypePlategaAcquiring, "platega_acquiring_button"},
		{database.InvoiceTypePlategaWorldwide, "platega_worldwide_button"},
		{database.InvoiceTypePlategaCrypto, "platega_crypto_button"},
	} {
		if plategaInvoiceEnabled(r.inv) {
			keyboard = append(keyboard, []models.InlineKeyboardButton{
				h.translation.WithButton(langCode, r.key, models.InlineKeyboardButton{
					CallbackData: paymentCallbackQuery(tidStr, month, string(r.inv), amount, extraCount),
				}),
			})
		}
	}
	if config.IsHeleketEnabled() {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			h.translation.WithButton(langCode, "heleket_button", models.InlineKeyboardButton{
				CallbackData: paymentCallbackQuery(tidStr, month, string(database.InvoiceTypeHeleket), amount, extraCount),
			}),
		})
	}
	return keyboard
}

func (h Handler) appendPlategaAddDevicePaymentRows(keyboard [][]models.InlineKeyboardButton, langCode string, target int) [][]models.InlineKeyboardButton {
	type row struct {
		inv database.InvoiceType
		key string
	}
	for _, r := range []row{
		{database.InvoiceTypePlategaSBP, "platega_sbp_button"},
		{database.InvoiceTypePlategaCards, "platega_cards_button"},
		{database.InvoiceTypePlategaAcquiring, "platega_acquiring_button"},
		{database.InvoiceTypePlategaWorldwide, "platega_worldwide_button"},
		{database.InvoiceTypePlategaCrypto, "platega_crypto_button"},
	} {
		if plategaInvoiceEnabled(r.inv) {
			keyboard = append(keyboard, []models.InlineKeyboardButton{
				h.translation.WithButton(langCode, r.key, models.InlineKeyboardButton{
					CallbackData: fmt.Sprintf("%s?target=%d&invoiceType=%s", CallbackAddDevicePayment, target, r.inv),
				}),
			})
		}
	}
	if config.IsHeleketEnabled() {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			h.translation.WithButton(langCode, "heleket_button", models.InlineKeyboardButton{
				CallbackData: fmt.Sprintf("%s?target=%d&invoiceType=%s", CallbackAddDevicePayment, target, database.InvoiceTypeHeleket),
			}),
		})
	}
	return keyboard
}

func (h Handler) appendPlategaRenewExtraRows(keyboard [][]models.InlineKeyboardButton, langCode string, extra, months int) [][]models.InlineKeyboardButton {
	type row struct {
		inv database.InvoiceType
		key string
	}
	for _, r := range []row{
		{database.InvoiceTypePlategaSBP, "platega_sbp_button"},
		{database.InvoiceTypePlategaCards, "platega_cards_button"},
		{database.InvoiceTypePlategaAcquiring, "platega_acquiring_button"},
		{database.InvoiceTypePlategaWorldwide, "platega_worldwide_button"},
		{database.InvoiceTypePlategaCrypto, "platega_crypto_button"},
	} {
		if plategaInvoiceEnabled(r.inv) {
			keyboard = append(keyboard, []models.InlineKeyboardButton{
				h.translation.WithButton(langCode, r.key, models.InlineKeyboardButton{
					CallbackData: fmt.Sprintf("%s?extra=%d&months=%d&invoiceType=%s", CallbackRenewExtraHwid, extra, months, r.inv),
				}),
			})
		}
	}
	if config.IsHeleketEnabled() {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			h.translation.WithButton(langCode, "heleket_button", models.InlineKeyboardButton{
				CallbackData: fmt.Sprintf("%s?extra=%d&months=%d&invoiceType=%s", CallbackRenewExtraHwid, extra, months, database.InvoiceTypeHeleket),
			}),
		})
	}
	return keyboard
}

func (h Handler) renderTariffLocationChoice(
	ctx context.Context,
	b *bot.Bot,
	update *models.Update,
	locs []database.TariffLocation,
	tidStr, month, invoiceType string,
	extra int,
	langCode string,
) {
	callback := update.CallbackQuery.Message.Message
	keyboard := make([][]models.InlineKeyboardButton, 0, len(locs)+1)
	soldLabel := h.translation.GetText(langCode, "tariff_location_sold_out")
	for _, loc := range locs {
		label := loc.Name
		if loc.SoldOut {
			label = soldLabel + ": " + loc.Name
		}
		data := paymentCallbackQuery(tidStr, month, invoiceType, "", extra) + fmt.Sprintf("&l=%d", loc.ID)
		if len(data) > 64 {
			slog.Error("tariff location callback too long", "len", len(data), "location", loc.ID)
			continue
		}
		keyboard = append(keyboard, []models.InlineKeyboardButton{{
			Text:         label,
			CallbackData: data,
		}})
	}
	back := fmt.Sprintf("%s?tid=%s&month=%s&amount=0&extra=%d", CallbackSell, tidStr, month, extra)
	keyboard = append(keyboard, []models.InlineKeyboardButton{
		h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: back}),
	})
	text := h.translation.GetText(langCode, "tariff_pick_location")
	_, err := editCallbackOriginToHTMLText(ctx, b, callback, text, models.ParseModeHTML, models.InlineKeyboardMarkup{
		InlineKeyboard: keyboard,
	}, nil)
	logEditError("tariff location choice", err)
}

func paymentCallbackQuery(tidStr, month, invoiceType, amount string, extra int) string {
	if tidStr != "" {
		return fmt.Sprintf("%s?tid=%s&month=%s&invoiceType=%s&extra=%d", CallbackPayment, tidStr, month, invoiceType, extra)
	}
	return fmt.Sprintf("%s?month=%s&invoiceType=%s&amount=%s&extra=%d", CallbackPayment, month, invoiceType, amount, extra)
}

func parseCallbackData(data string) map[string]string {
	result := make(map[string]string)

	parts := strings.Split(data, "?")
	if len(parts) < 2 {
		return result
	}

	params := strings.Split(parts[1], "&")
	for _, param := range params {
		kv := strings.SplitN(param, "=", 2)
		if len(kv) == 2 {
			result[kv[0]] = kv[1]
		}
	}

	return result
}

func (h Handler) resolvePricingInfoKey(ctx context.Context, customer *database.Customer, extraCount int) string {
	if customer == nil {
		return "pricing_info_paid"
	}
	hasPaid, err := h.purchaseRepository.HasPaidSubscription(ctx, customer.ID)
	if err != nil {
		slog.Error("Error checking paid subscription", "error", err)
		return "pricing_info_paid"
	}
	if !hasPaid {
		return "pricing_info_trial"
	}
	if extraCount > 0 {
		return "pricing_info_paid_extra"
	}
	return "pricing_info_paid"
}

// pricingInfoTextWithDiscount inserts an active promo discount line before the payment methods block (all pricing_info_* keys).
func (h Handler) pricingInfoTextWithDiscount(ctx context.Context, lang string, customer *database.Customer, pricingInfoKey string) string {
	base := h.translation.GetText(lang, pricingInfoKey)
	return h.appendPendingDiscountToPricingHTML(ctx, lang, customer, base)
}

// appendPendingDiscountToPricingHTML вставляет блоки про лояльность и промо перед «Способы оплаты» / «Payment methods».
func (h Handler) appendPendingDiscountToPricingHTML(ctx context.Context, lang string, customer *database.Customer, base string) string {
	capPct := config.LoyaltyMaxTotalDiscountPercent()

	loyaltyPct := 0
	if config.LoyaltyEnabled() && h.loyaltyTierRepository != nil && customer != nil {
		pct, err := h.loyaltyTierRepository.DiscountPercentForXP(ctx, customer.LoyaltyXP)
		if err != nil {
			slog.Error("pricing screen loyalty discount", "error", err)
		} else {
			loyaltyPct = pct
		}
	}

	promoPct := 0
	promoOK := false
	if h.promoService != nil && customer != nil {
		pct, _, _, ok, err := h.promoService.PendingDiscountForConnectUI(ctx, customer.ID)
		if err != nil {
			slog.Error("pricing screen pending discount", "error", err)
		} else if ok && pct > 0 {
			promoPct = pct
			promoOK = true
		}
	}

	var blocks []string
	switch {
	case loyaltyPct > 0 && promoOK:
		total := loyalty.CombinedDiscountPercent(loyaltyPct, promoPct, capPct)
		blocks = append(blocks, fmt.Sprintf(h.translation.GetText(lang, "buy_screen_loyalty_promo_combo_note"), loyaltyPct, promoPct, total))
	case loyaltyPct > 0:
		blocks = append(blocks, fmt.Sprintf(h.translation.GetText(lang, "buy_screen_loyalty_discount_note"), loyaltyPct))
	case promoOK:
		blocks = append(blocks, fmt.Sprintf(h.translation.GetText(lang, "buy_screen_pending_discount_note"), promoPct))
	default:
		return base
	}

	note := strings.Join(blocks, "\n\n")
	var needles []string
	if lang == "ru" {
		needles = []string{"\n\n<b>Способы оплаты:</b>", "<b>Способы оплаты:</b>"}
	} else {
		needles = []string{"\n\n<b>Payment methods:</b>", "<b>Payment methods:</b>"}
	}
	for _, needle := range needles {
		if idx := strings.Index(base, needle); idx >= 0 {
			return base[:idx] + "\n\n" + note + base[idx:]
		}
	}
	return base + "\n\n" + note
}

// notifyPaymentProviderUnavailable сообщает о сбое платёжного провайдера и предлагает вернуться назад (отвечает на callback, чтобы снять «часики»).
func (h Handler) notifyPaymentProviderUnavailable(ctx context.Context, b *bot.Bot, update *models.Update, langCode, backCallbackData string) {
	if update.CallbackQuery != nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
		})
	}
	text := h.translation.GetText(langCode, "payment_provider_unavailable")
	markup := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: backCallbackData})},
		},
	}
	err := SendOrEditAfterInlineCallback(ctx, b, update, text, models.ParseModeHTML, markup, nil)
	if err != nil {
		logEditError("notify payment provider unavailable", err)
	}
}
