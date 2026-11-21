//go:build integration

package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func reviewTestAccount(t *testing.T, ctx context.Context, accounts *AccountRepo) int64 {
	t.Helper()
	acc, err := accounts.Create(ctx, "rev-"+uuid.NewString()+"@example.com", "x", "ru")
	if err != nil {
		t.Fatal(err)
	}
	return acc.ID
}

func validReviewInput(accountID int64, body string) ReviewInput {
	return ReviewInput{AccountID: accountID, Rating: 5, Body: body, Visibility: ReviewVisibilityName, DisplayName: "Тест"}
}

// Сквозной сценарий: создание, правка клиентом, тихая правка админом с сохранением
// оригинала, ответ команды, скрытие и поведение скрытого отзыва при правке клиентом.
func TestReviews_lifecycle(t *testing.T) {
	pool := pgPoolIntegration(t)
	ctx := context.Background()
	accounts := NewAccountRepo(pool)
	repo := NewReviewRepo(pool)
	acc := reviewTestAccount(t, ctx, accounts)
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM cabinet_account WHERE id = $1`, acc) })

	// 1. Новый отзыв при модерации — «на проверке», не опубликован.
	rev, created, err := repo.UpsertClient(ctx, validReviewInput(acc, "Первый отзыв клиента"), true)
	if err != nil || !created {
		t.Fatalf("создание: %v created=%v", err, created)
	}
	if rev.Status != ReviewStatusPending || rev.PublishedAt != nil {
		t.Fatalf("ждём pending без даты публикации: %+v", rev)
	}
	if n, _ := repo.CountPublished(ctx); n != 0 {
		t.Fatalf("pending не должен считаться опубликованным: %d", n)
	}

	// 2. Админ публикует и тихо правит текст: оригинал запоминается.
	pub := ReviewStatusPublished
	edited := "Первый отзыв клиента (поправлено)"
	rev, _, err = repo.AdminUpdate(ctx, rev.ID, ReviewAdminUpdate{Status: &pub, Body: &edited})
	if err != nil {
		t.Fatal(err)
	}
	if rev.Status != ReviewStatusPublished || rev.PublishedAt == nil || rev.OriginalBody != "Первый отзыв клиента" || rev.Body != edited {
		t.Fatalf("после правки админа: %+v", rev)
	}
	// Вторая правка оригинал не затирает.
	again := "Ещё раз поправлено админом"
	rev, _, err = repo.AdminUpdate(ctx, rev.ID, ReviewAdminUpdate{Body: &again})
	if err != nil || rev.OriginalBody != "Первый отзыв клиента" {
		t.Fatalf("оригинал потерян: %v %+v", err, rev)
	}

	// 3. Ответ команды: метка времени и признак «изменился» для уведомления.
	reply := "Спасибо за отзыв!"
	rev, changed, err := repo.AdminUpdate(ctx, rev.ID, ReviewAdminUpdate{ReplyBody: &reply})
	if err != nil || !changed || rev.ReplyAt == nil || rev.ReplyBody != reply {
		t.Fatalf("ответ: %v changed=%v %+v", err, changed, rev)
	}
	rev, changed, _ = repo.AdminUpdate(ctx, rev.ID, ReviewAdminUpdate{ReplyBody: &reply})
	if changed {
		t.Fatal("тот же ответ не должен слать уведомление повторно")
	}

	// 4. Клиент правит свой отзыв при модерации: снова pending, следы админа сняты, ответ цел.
	rev, created, err = repo.UpsertClient(ctx, validReviewInput(acc, "Клиент переписал отзыв"), true)
	if err != nil || created {
		t.Fatalf("правка клиентом: %v created=%v", err, created)
	}
	if rev.Status != ReviewStatusPending || rev.OriginalBody != "" || rev.ReplyBody != reply {
		t.Fatalf("после правки клиентом: %+v", rev)
	}

	// 5. Скрытый админом отзыв остаётся скрытым, как бы клиент его ни правил.
	hidden := ReviewStatusHidden
	if _, _, err = repo.AdminUpdate(ctx, rev.ID, ReviewAdminUpdate{Status: &hidden}); err != nil {
		t.Fatal(err)
	}
	rev, _, err = repo.UpsertClient(ctx, validReviewInput(acc, "Клиент правит скрытый отзыв"), false)
	if err != nil || rev.Status != ReviewStatusHidden {
		t.Fatalf("скрытый должен остаться скрытым: %v %+v", err, rev)
	}
	if list, _ := repo.ListPublished(ctx, 50, 0); len(list) != 0 {
		t.Fatalf("скрытый попал в публичный список: %d", len(list))
	}

	// 6. Один отзыв на аккаунт; удаление.
	if _, err := repo.FindByAccountID(ctx, acc); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, rev.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindByID(ctx, rev.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("после удаления: %v", err)
	}
}

func TestReviews_validationAndSearch(t *testing.T) {
	pool := pgPoolIntegration(t)
	ctx := context.Background()
	accounts := NewAccountRepo(pool)
	repo := NewReviewRepo(pool)
	acc := reviewTestAccount(t, ctx, accounts)
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM cabinet_account WHERE id = $1`, acc) })

	for name, in := range map[string]ReviewInput{
		"короткий текст": validReviewInput(acc, "мало"),
		"длинный текст":  validReviewInput(acc, strings.Repeat("я", ReviewBodyMax+1)),
		"оценка 0":       {AccountID: acc, Rating: 0, Body: "Нормальный текст отзыва", Visibility: ReviewVisibilityName},
		"оценка 6":       {AccountID: acc, Rating: 6, Body: "Нормальный текст отзыва", Visibility: ReviewVisibilityName},
		"видимость":      {AccountID: acc, Rating: 5, Body: "Нормальный текст отзыва", Visibility: "всем"},
	} {
		if _, _, err := repo.UpsertClient(ctx, in, false); !errors.Is(err, ErrReviewValidation) {
			t.Errorf("%s: ждали ErrReviewValidation, получили %v", name, err)
		}
	}

	// Режим «сразу»: отзыв опубликован с датой публикации.
	rev, _, err := repo.UpsertClient(ctx, validReviewInput(acc, "Уникальное словоюникс в отзыве"), false)
	if err != nil || rev.Status != ReviewStatusPublished || rev.PublishedAt == nil {
		t.Fatalf("instant: %v %+v", err, rev)
	}
	// Поиск по тексту; спецсимволы LIKE не работают как маски.
	items, total, err := repo.AdminList(ctx, ReviewAdminFilter{Search: "словоюникс"}, 20, 0)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("поиск: %v total=%d", err, total)
	}
	if _, total, _ := repo.AdminList(ctx, ReviewAdminFilter{Search: "%"}, 20, 0); total != 0 {
		t.Fatalf("«%%» не должен находить всё: %d", total)
	}
	if _, _, err := repo.AdminList(ctx, ReviewAdminFilter{Status: "bogus"}, 20, 0); !errors.Is(err, ErrReviewValidation) {
		t.Fatalf("неизвестный статус: %v", err)
	}
	counts, err := repo.Counts(ctx)
	if err != nil || counts.All < 1 || counts.Published < 1 {
		t.Fatalf("счётчики: %v %+v", err, counts)
	}
}

// Клиент поправил только оценку или галочки, текст не трогал: тихая правка админа
// остаётся правкой, оригинал не пропадает. Сбой чтения клиента не стирает «клиент с».
func TestReviews_clientSaveKeepsAdminOriginalWhenBodyUnchanged(t *testing.T) {
	pool := pgPoolIntegration(t)
	ctx := context.Background()
	accounts := NewAccountRepo(pool)
	repo := NewReviewRepo(pool)
	acc := reviewTestAccount(t, ctx, accounts)
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM cabinet_account WHERE id = $1`, acc) })

	since := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	in := validReviewInput(acc, "Исходный текст клиента")
	in.ClientSince, in.TariffLabel = &since, "Годовой"
	rev, _, err := repo.UpsertClient(ctx, in, false)
	if err != nil {
		t.Fatal(err)
	}
	fixed := "Текст после правки админа"
	if rev, _, err = repo.AdminUpdate(ctx, rev.ID, ReviewAdminUpdate{Body: &fixed}); err != nil {
		t.Fatal(err)
	}

	// Клиент сохраняет форму: текст тот, что видит (правленый), оценка другая, данных о клиенте нет.
	in2 := validReviewInput(acc, fixed)
	in2.Rating = 3
	rev, _, err = repo.UpsertClient(ctx, in2, false)
	if err != nil {
		t.Fatal(err)
	}
	if rev.OriginalBody != "Исходный текст клиента" {
		t.Fatalf("оригинал потерян при сохранении без правки текста: %q", rev.OriginalBody)
	}
	if rev.ClientSince == nil || rev.TariffLabel != "Годовой" {
		t.Fatalf("данные автора затёрты: %+v", rev)
	}

	// Клиент переписал текст сам: следы правки админа снимаются.
	rev, _, err = repo.UpsertClient(ctx, validReviewInput(acc, "Совсем новый текст клиента"), false)
	if err != nil || rev.OriginalBody != "" {
		t.Fatalf("после переписывания: %v %q", err, rev.OriginalBody)
	}
}

// Админ переносит старые отзывы (например из канала) и задаёт им настоящие даты:
// и при создании, и при правке; будущая дата отклоняется.
func TestReviews_adminCanSetPublishDate(t *testing.T) {
	pool := pgPoolIntegration(t)
	ctx := context.Background()
	accounts := NewAccountRepo(pool)
	repo := NewReviewRepo(pool)
	acc := reviewTestAccount(t, ctx, accounts)
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM cabinet_account WHERE id = $1`, acc) })

	old := time.Date(2025, 3, 10, 12, 0, 0, 0, time.UTC)
	in := validReviewInput(acc, "Отзыв из старого канала")
	in.PublishedAt = &old
	rev, err := repo.UpsertByAdmin(ctx, in, ReviewStatusPublished)
	if err != nil {
		t.Fatal(err)
	}
	if rev.PublishedAt == nil || !rev.PublishedAt.Equal(old) || !rev.CreatedAt.Equal(old) {
		t.Fatalf("дата при создании: published=%v created=%v", rev.PublishedAt, rev.CreatedAt)
	}

	newer := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	rev, _, err = repo.AdminUpdate(ctx, rev.ID, ReviewAdminUpdate{PublishedAt: &newer})
	if err != nil || rev.PublishedAt == nil || !rev.PublishedAt.Equal(newer) || !rev.CreatedAt.Equal(newer) {
		t.Fatalf("дата при правке: %v %+v", err, rev)
	}

	future := time.Now().Add(72 * time.Hour)
	if _, _, err = repo.AdminUpdate(ctx, rev.ID, ReviewAdminUpdate{PublishedAt: &future}); !errors.Is(err, ErrReviewValidation) {
		t.Fatalf("будущая дата должна отклоняться: %v", err)
	}
	in2 := validReviewInput(acc, "Отзыв из будущего")
	in2.PublishedAt = &future
	if _, err = repo.UpsertByAdmin(ctx, in2, ReviewStatusPublished); !errors.Is(err, ErrReviewValidation) {
		t.Fatalf("будущая дата при создании: %v", err)
	}

	// Публичный порядок — по дате публикации: свежие выше старых.
	if list, _ := repo.ListPublished(ctx, 50, 0); len(list) == 0 || !list[len(list)-1].PublishedAt.Before(time.Now()) {
		t.Fatalf("список: %d", len(list))
	}
}
