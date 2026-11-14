package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"remnawave-tg-shop-bot/utils"
)

// История оплат: в Bedolaga деньги тратятся с внутреннего баланса, и каждая покупка/продление
// подписки — запись transactions(type = subscription_payment). Переносим их в purchase как
// оплаченные подписки (invoice_type = bedolaga) с настоящими датами. Без этого бот считает
// любого перенесённого клиента с действующей подпиской «триалом» (оплат в истории нет).

type bedolagaPayment struct {
	ID          int
	UserID      int
	AmountRub   float64
	Description string
	CreatedAt   time.Time
	PaidAt      time.Time
}

func (s *sourceDB) LoadPayments(ctx context.Context) ([]bedolagaPayment, error) {
	ok, err := s.hasTable(ctx, "transactions")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, ABS(COALESCE(amount_kopeks,0)), COALESCE(description,''),
		       created_at, COALESCE(completed_at, created_at)
		FROM transactions
		WHERE type = 'subscription_payment' AND COALESCE(is_completed,true) AND created_at IS NOT NULL
		ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("select transactions: %w", err)
	}
	defer rows.Close()
	var out []bedolagaPayment
	for rows.Next() {
		var p bedolagaPayment
		var kop int64
		if err := rows.Scan(&p.ID, &p.UserID, &kop, &p.Description, &p.CreatedAt, &p.PaidAt); err != nil {
			return nil, err
		}
		p.AmountRub = float64(kop) / 100
		out = append(out, p)
	}
	return out, rows.Err()
}

var paymentDaysRe = regexp.MustCompile(`(\d+)\s*(?:дн|day)`)

// paymentMonths — срок оплаты в месяцах по описанию «… на 90 дней». ok=false — запись не про
// подписку (доп. устройства, смена тарифа без срока) и в историю не попадает.
func paymentMonths(desc string) (months int, days int, ok bool) {
	low := strings.ToLower(desc)
	if strings.Contains(low, "устройств") || strings.Contains(low, "device") {
		return 0, 0, false
	}
	m := paymentDaysRe.FindStringSubmatch(low)
	if m == nil {
		return 0, 0, false
	}
	// Регулярка пропускает только цифры, но число может не влезть в int — такую запись
	// не угадываем, а пропускаем, как и запись без срока.
	days, err := strconv.Atoi(m[1])
	if err != nil || days <= 0 {
		return 0, 0, false
	}
	months = int(math.Round(float64(days) / 30))
	if months < 1 {
		months = 1
	}
	return months, days, true
}

// userCustomerMap: id пользователя Bedolaga → telegram_id клиента у нас. Нужен и когда шаг
// customers запускался в другом процессе (мастер гонит шаги по одному).
func (m *Migrator) userCustomerMap(ctx context.Context, users []bedolagaUser) {
	for _, u := range users {
		if _, ok := m.bdgUserToTG[u.ID]; ok {
			continue
		}
		if u.TelegramID != nil && *u.TelegramID != 0 {
			m.bdgUserToTG[u.ID] = *u.TelegramID
			continue
		}
		email := ""
		if u.Email != nil {
			email = strings.ToLower(strings.TrimSpace(*u.Email))
		}
		if acc, err := m.findExistingCabinetAccount(ctx, u, email); err == nil && acc != nil {
			m.bdgUserToTG[u.ID] = utils.SyntheticTelegramID(acc.ID)
		}
	}
}

func (m *Migrator) stepHistory(ctx context.Context) error {
	if !m.cfg.History.ImportPayments {
		slog.Info("history step skipped (history.import_payments=false)")
		return nil
	}
	pays, err := m.src.LoadPayments(ctx)
	if err != nil {
		return err
	}
	if len(pays) == 0 {
		slog.Info("no subscription payments in source")
		return nil
	}
	users, err := m.src.LoadUsers(ctx)
	if err != nil {
		return err
	}
	m.userCustomerMap(ctx, users)

	custID := map[int64]int64{}
	for _, p := range pays {
		months, days, ok := paymentMonths(p.Description)
		if !ok {
			m.rep.summary.PaymentsSkipped++
			continue
		}
		tg, ok := m.bdgUserToTG[p.UserID]
		if !ok || tg == 0 {
			m.rep.addProblem("history_no_customer", "transaction", fmt.Sprintf("%d", p.ID), "у пользователя нет клиента в нашей базе")
			m.rep.summary.PaymentsSkipped++
			continue
		}
		if m.dryRun {
			m.rep.summary.PaymentsOK++
			continue
		}
		cid, ok := custID[tg]
		if !ok {
			c, err := m.custRepo.FindByTelegramId(ctx, tg)
			if err != nil || c == nil {
				m.rep.addProblem("history_no_customer", "transaction", fmt.Sprintf("%d", p.ID), "клиент не найден (шаг customers не выполнен?)")
				m.rep.summary.PaymentsSkipped++
				continue
			}
			cid = c.ID
			custID[tg] = cid
		}
		expire := p.PaidAt.Add(time.Duration(days) * 24 * time.Hour)
		tag, err := m.target.Exec(ctx, `
			INSERT INTO purchase (amount, customer_id, created_at, month, paid_at, currency, expire_at,
			                      status, invoice_type, purchase_kind)
			SELECT $1, $2, $3, $4, $5, 'RUB', $6, 'paid', 'bedolaga', 'subscription'
			WHERE NOT EXISTS (
				SELECT 1 FROM purchase
				WHERE customer_id = $2 AND invoice_type = 'bedolaga' AND paid_at = $5 AND amount = $1 AND month = $4
			)`, p.AmountRub, cid, p.CreatedAt, months, p.PaidAt, expire)
		if err != nil {
			m.rep.addProblem("history_failed", "transaction", fmt.Sprintf("%d", p.ID), err.Error())
			continue
		}
		if tag.RowsAffected() > 0 {
			m.rep.summary.PaymentsOK++
		}
	}
	return nil
}
