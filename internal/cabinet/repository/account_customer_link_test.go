package repository

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsAccountFKViolation(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"account fk", fmt.Errorf("create link: %w", &pgconn.PgError{
			Code: "23503", ConstraintName: "cabinet_account_customer_link_account_id_fkey"}), true},
		{
			// FK по customer_id — это другой сбой, аккаунт тут ни при чём.
			name: "customer fk",
			err:  &pgconn.PgError{Code: "23503", ConstraintName: "cabinet_account_customer_link_customer_id_fkey"},
			want: false,
		},
		{
			name: "unique violation",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "cabinet_account_customer_link_account_id_key"},
			want: false,
		},
		{"unrelated", errors.New("connection reset by peer"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAccountFKViolation(tc.err); got != tc.want {
				t.Fatalf("isAccountFKViolation(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
