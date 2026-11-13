package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// CabinetIdentity — что известно о веб-кабинете клиента: почта и способы входа.
type CabinetIdentity struct {
	AccountID     int64
	Email         string
	EmailVerified bool
	// Providers — привязанные способы входа без повторов, в порядке привязки.
	// "email" попадает сюда только при заданном пароле: иначе почта — просто
	// контакт, войти по ней нельзя (так же считает /me).
	Providers []string
}

// CabinetIdentityByCustomerID — кабинет клиента; nil, если клиент живёт только
// в боте и веб-кабинета у него нет.
func (cr *CustomerRepository) CabinetIdentityByCustomerID(ctx context.Context, customerID int64) (*CabinetIdentity, error) {
	const accQ = `
		SELECT a.id, COALESCE(a.email, ''), a.email_verified_at IS NOT NULL, a.password_hash IS NOT NULL
		FROM cabinet_account_customer_link l
		JOIN cabinet_account a ON a.id = l.account_id
		WHERE l.customer_id = $1
		ORDER BY l.account_id
		LIMIT 1`
	var out CabinetIdentity
	var hasPassword bool
	err := cr.pool.QueryRow(ctx, accQ, customerID).Scan(&out.AccountID, &out.Email, &out.EmailVerified, &hasPassword)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query cabinet account by customer: %w", err)
	}
	out.Email = strings.TrimSpace(out.Email)
	if out.Email == "" {
		out.EmailVerified = false
	}

	const idsQ = `
		SELECT provider FROM cabinet_identity
		WHERE account_id = $1 AND unlinked_at IS NULL
		ORDER BY id`
	rows, err := cr.pool.Query(ctx, idsQ, out.AccountID)
	if err != nil {
		return nil, fmt.Errorf("query cabinet identities: %w", err)
	}
	defer rows.Close()
	seen := make(map[string]struct{})
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("scan cabinet identity: %w", err)
		}
		if p == "email" && !hasPassword {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out.Providers = append(out.Providers, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &out, nil
}

// VerifiedCabinetEmail — подтверждённая почта клиента или "". Непроверенный
// адрес наружу (в панель) не отдаём: в нём может быть опечатка.
func (cr *CustomerRepository) VerifiedCabinetEmail(ctx context.Context, customerID int64) (string, error) {
	const q = `
		SELECT a.email
		FROM cabinet_account_customer_link l
		JOIN cabinet_account a ON a.id = l.account_id
		WHERE l.customer_id = $1 AND a.email IS NOT NULL AND a.email_verified_at IS NOT NULL
		ORDER BY l.account_id
		LIMIT 1`
	var email string
	err := cr.pool.QueryRow(ctx, q, customerID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("query verified cabinet email: %w", err)
	}
	return strings.TrimSpace(email), nil
}
