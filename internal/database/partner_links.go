package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Ошибки управления потоками. Каждая соответствует отказу, который надо
// объяснить партнёру словами, а не показать «внутренняя ошибка».
var (
	ErrPartnerLinkNotFound = errors.New("partner link not found")
	// Основную ссылку нельзя ни удалить, ни убрать в архив: без неё партнёру
	// нечего давать людям.
	ErrPartnerLinkIsDefault = errors.New("default partner link cannot be removed")
	// По потоку уже кто-то пришёл — удалять нельзя, только в архив. Иначе из
	// журнала начислений исчезнет источник, и в спорной ситуации не на что
	// сослаться ни партнёру, ни магазину. То же правило держат внешние ключи
	// partner_attribution.link_id и partner_earning.link_id.
	ErrPartnerLinkHasHistory = errors.New("partner link has attributed customers")
	// Лимит рабочих потоков исчерпан — возврат из архива откладывается до
	// освобождения места.
	ErrPartnerLinkLimitReached = errors.New("partner links limit reached")
)

// FindLinkByID возвращает ссылку партнёра. Проверка владения встроена в запрос:
// без неё чужой id в URL позволил бы управлять потоками другого партнёра.
func (r *PartnerRepository) FindLinkByID(ctx context.Context, partnerID, linkID int64) (*PartnerLink, error) {
	var l PartnerLink
	err := r.pool.QueryRow(ctx,
		`SELECT id, partner_id, code, name, is_default, archived_at, created_at
		   FROM partner_link WHERE id = $1 AND partner_id = $2`, linkID, partnerID,
	).Scan(&l.ID, &l.PartnerID, &l.Code, &l.Name, &l.IsDefault, &l.ArchivedAt, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find partner link: %w", err)
	}
	return &l, nil
}

// RenameLink меняет название потока. Код ссылки не меняется никогда: он уже
// разослан по площадкам, и смена кода обнулила бы всю наружную рекламу.
func (r *PartnerRepository) RenameLink(ctx context.Context, partnerID, linkID int64, name string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE partner_link SET name = $3 WHERE id = $1 AND partner_id = $2`,
		linkID, partnerID, name)
	if err != nil {
		return fmt.Errorf("failed to rename partner link: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPartnerLinkNotFound
	}
	return nil
}

// SetLinkArchived закрывает поток для новых переходов либо возвращает его в
// работу. Уже приведённые клиенты остаются за партнёром и продолжают приносить
// процент: архив — это выключенная ссылка, а не отказ от заработанного.
//
// limit — сколько рабочих потоков разрешено партнёру; проверяется при возврате
// из архива. Без этой проверки лимит обходится циклом «заархивировать →
// создать новый → вернуть из архива»: архивные потоки лимит не занимают.
func (r *PartnerRepository) SetLinkArchived(ctx context.Context, partnerID, linkID int64, archived bool, limit int) error {
	link, err := r.FindLinkByID(ctx, partnerID, linkID)
	if err != nil {
		return err
	}
	if link == nil {
		return ErrPartnerLinkNotFound
	}
	if link.IsDefault {
		return ErrPartnerLinkIsDefault
	}
	if !archived && link.ArchivedAt != nil && limit > 0 {
		used, err := r.CountLinks(ctx, partnerID)
		if err != nil {
			return err
		}
		if used >= limit {
			return ErrPartnerLinkLimitReached
		}
	}

	var archivedAt *time.Time
	if archived {
		now := time.Now().UTC()
		archivedAt = &now
	}
	if _, err := r.pool.Exec(ctx,
		`UPDATE partner_link SET archived_at = $3 WHERE id = $1 AND partner_id = $2`,
		linkID, partnerID, archivedAt); err != nil {
		return fmt.Errorf("failed to archive partner link: %w", err)
	}
	return nil
}

// DeleteLink удаляет поток физически — но только пустой.
//
// Правило одно: пустой поток это опечатка, её стирают; поток с историей —
// источник начислений, его архивируют. Проверка идёт до удаления, потому что
// иначе партнёр получил бы отказ внешнего ключа вместо объяснения, что делать
// дальше.
func (r *PartnerRepository) DeleteLink(ctx context.Context, partnerID, linkID int64) error {
	link, err := r.FindLinkByID(ctx, partnerID, linkID)
	if err != nil {
		return err
	}
	if link == nil {
		return ErrPartnerLinkNotFound
	}
	if link.IsDefault {
		return ErrPartnerLinkIsDefault
	}
	hasHistory, err := r.LinkHasHistory(ctx, linkID)
	if err != nil {
		return err
	}
	if hasHistory {
		return ErrPartnerLinkHasHistory
	}

	tag, err := r.pool.Exec(ctx,
		`DELETE FROM partner_link WHERE id = $1 AND partner_id = $2 AND is_default = FALSE`,
		linkID, partnerID)
	if err != nil {
		return fmt.Errorf("failed to delete partner link: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPartnerLinkNotFound
	}
	return nil
}
