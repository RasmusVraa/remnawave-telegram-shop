package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	gosync "sync"
	"time"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/utils"
)

// ErrSyncRunning — синхронизация уже идёт (запущена из бота или кабинета).
var ErrSyncRunning = errors.New("sync already in progress")

// syncTimeout ограничивает весь проход: выгрузка панели и запись в БД.
const syncTimeout = 10 * time.Minute

type SyncService struct {
	client             *remnawave.Client
	customerRepository *database.CustomerRepository
	running            *gosync.Mutex
}

func NewSyncService(client *remnawave.Client, customerRepository *database.CustomerRepository) *SyncService {
	return &SyncService{
		client: client, customerRepository: customerRepository, running: &gosync.Mutex{},
	}
}

func (s SyncService) GetRemnawaveClient() *remnawave.Client {
	return s.client
}

// Result — итог синхронизации.
type Result struct {
	Created int
	Updated int
	Deleted int64
	// Kept — клиентов нет в панели, но они оставлены: есть покупки, рефералы,
	// партнёрка, промокоды или привязка к кабинету.
	Kept int64
}

// Sync переносит в БД бота пользователей панели: создаёт новых, обновляет срок
// и ссылку у известных, затем удаляет пропавших. Удаление идёт последним и
// только если запись прошла без ошибок. Если удалять пришлось бы слишком много,
// оно пропускается, а ошибка оборачивает database.ErrSyncTooManyDeletions —
// Result при этом содержит счётчики уже выполненной записи.
func (s SyncService) Sync(ctx context.Context) (Result, error) {
	var res Result
	if !s.running.TryLock() {
		return res, ErrSyncRunning
	}
	defer s.running.Unlock()

	ctx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()

	slog.Info("Starting sync")
	users, err := s.client.GetUsers(ctx)
	if err != nil {
		return res, fmt.Errorf("get users from remnawave: %w", err)
	}

	var telegramIDs []int64
	telegramIDsSet := make(map[int64]struct{})
	var mappedUsers []database.Customer
	for _, user := range users {
		if user.TelegramID == nil {
			continue
		}
		tid := *user.TelegramID
		// Synthetic/web-only ids must never be imported by bot sync.
		// They belong to cabinet-only customers and can be rounded by panel side.
		if utils.IsSyntheticTelegramID(tid) {
			continue
		}
		if _, exists := telegramIDsSet[tid]; exists {
			continue
		}
		telegramIDsSet[tid] = struct{}{}
		telegramIDs = append(telegramIDs, tid)
		mappedUsers = append(mappedUsers, database.Customer{
			TelegramID:       tid,
			ExpireAt:         &user.ExpireAt,
			SubscriptionLink: &user.SubscriptionUrl,
		})
	}
	// Пустая панель или ни одного Telegram ID — это сбой или смена формата
	// ответа, а не «все ушли». Раньше в этом случае удалялись все клиенты бота.
	if len(telegramIDs) == 0 {
		return res, fmt.Errorf("remnawave returned %d users, none with telegram id: sync stopped", len(users))
	}

	existingCustomers, err := s.customerRepository.FindByTelegramIds(ctx, telegramIDs)
	if err != nil {
		return res, fmt.Errorf("find customers by telegram ids: %w", err)
	}
	existingMap := make(map[int64]database.Customer, len(existingCustomers))
	for _, cust := range existingCustomers {
		existingMap[cust.TelegramID] = cust
	}

	var toCreate, toUpdate []database.Customer
	for _, cust := range mappedUsers {
		if existing, found := existingMap[cust.TelegramID]; found {
			cust.ID = existing.ID
			cust.CreatedAt = existing.CreatedAt
			cust.Language = existing.Language
			toUpdate = append(toUpdate, cust)
		} else {
			toCreate = append(toCreate, cust)
		}
	}

	if len(toCreate) > 0 {
		if err := s.customerRepository.CreateBatch(ctx, toCreate); err != nil {
			return res, fmt.Errorf("create customers: %w", err)
		}
		res.Created = len(toCreate)
	}
	if len(toUpdate) > 0 {
		if err := s.customerRepository.UpdateBatch(ctx, toUpdate); err != nil {
			return res, fmt.Errorf("update customers: %w", err)
		}
		res.Updated = len(toUpdate)
	}

	// Web-only клиенты и привязанные к кабинету в список панели не попадают;
	// DeleteMissingFromPanel их не трогает, как и клиентов с историей.
	del, err := s.customerRepository.DeleteMissingFromPanel(ctx, telegramIDs)
	res.Deleted, res.Kept = del.Deleted, del.Kept
	if err != nil {
		return res, fmt.Errorf("delete customers missing from panel: %w", err)
	}
	slog.Info("Synchronization completed",
		"created", res.Created, "updated", res.Updated, "deleted", res.Deleted, "kept", res.Kept)
	return res, nil
}
