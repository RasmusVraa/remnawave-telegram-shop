package pgerr

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsUniqueViolation(t *testing.T) {
	wrapped := fmt.Errorf("create: %w", &pgconn.PgError{Code: "23505"})
	if !IsUniqueViolation(wrapped) {
		t.Fatal("wrapped 23505 must be recognised")
	}
	// Текст похож, а кода нет — это не нарушение уникальности.
	if IsUniqueViolation(errors.New("duplicate key value violates unique constraint (SQLSTATE 23505)")) {
		t.Fatal("plain text must not be recognised")
	}
	if IsUniqueViolation(&pgconn.PgError{Code: "23503"}) || IsUniqueViolation(nil) {
		t.Fatal("other codes and nil are not unique violations")
	}
}

func TestIsForeignKeyViolation(t *testing.T) {
	err := fmt.Errorf("insert: %w", &pgconn.PgError{Code: "23503", ConstraintName: "link_account_id_fkey"})
	if !IsForeignKeyViolation(err, "link_account_id_fkey") || !IsForeignKeyViolation(err, "") {
		t.Fatal("matching constraint must be recognised")
	}
	if IsForeignKeyViolation(err, "link_customer_id_fkey") {
		t.Fatal("other constraint must not match")
	}
	if IsForeignKeyViolation(&pgconn.PgError{Code: "23505"}, "") {
		t.Fatal("unique violation is not a foreign key violation")
	}
}
