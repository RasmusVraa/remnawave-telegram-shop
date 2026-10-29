// Package pgerr распознаёт ошибки PostgreSQL по SQLSTATE, а не по тексту.
//
// Раньше в кабинете нарушения уникальности и внешних ключей искались
// подстроками («duplicate key», «23505»): текст зависит от lc_messages сервера
// и от того, как драйвер оборачивает ошибку, а код — нет.
package pgerr

import (
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

// IsUniqueViolation — нарушение уникальности (23505).
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation
}

// IsForeignKeyViolation — нарушение внешнего ключа (23503) с указанным именем
// ограничения; пустое имя — любого.
func IsForeignKeyViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgerrcode.ForeignKeyViolation {
		return false
	}
	return constraint == "" || pgErr.ConstraintName == constraint
}
