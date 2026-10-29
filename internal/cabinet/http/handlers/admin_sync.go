package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	gosync "sync"
	"time"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/sync"
	"remnawave-tg-shop-bot/utils"
)

type AdminSyncHandler struct {
	syncService *sync.SyncService

	mu     gosync.Mutex
	status adminSyncStatus
}

// adminSyncStatus — состояние последнего запуска из кабинета. Синхронизация
// идёт в фоне (на большой панели — минуты), страница опрашивает GET.
type adminSyncStatus struct {
	// idle — из кабинета ещё не запускали; running; ok; delete_blocked —
	// клиенты обновлены, удаление остановлено предохранителем; busy — уже идёт
	// синхронизация, запущенная из бота; failed.
	State      string     `json:"state"`
	Created    int        `json:"created"`
	Updated    int        `json:"updated"`
	Deleted    int64      `json:"deleted"`
	Kept       int64      `json:"kept"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

func NewAdminSync(syncService *sync.SyncService) *AdminSyncHandler {
	return &AdminSyncHandler{syncService: syncService, status: adminSyncStatus{State: "idle"}}
}

// Status — GET /admin/sync: итог последнего запуска.
func (h *AdminSyncHandler) Status(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.snapshot())
}

// TriggerSync — POST /admin/sync: запуск в фоне.
func (h *AdminSyncHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.mu.Lock()
	if h.status.State == "running" {
		h.mu.Unlock()
		http.Error(w, "sync already in progress", http.StatusConflict)
		return
	}
	now := time.Now().UTC()
	h.status = adminSyncStatus{State: "running", StartedAt: &now}
	h.mu.Unlock()

	slog.Info("admin: sync triggered via cabinet")
	utils.Go("cabinet sync", func() {
		// Паника внутри не должна оставить страницу в «идёт» навсегда.
		done := false
		defer func() {
			if !done {
				h.mu.Lock()
				h.status.State = "failed"
				h.mu.Unlock()
			}
		}()
		// r.Context() отменится сразу после ответа — синхронизация идёт со своим.
		res, err := h.syncService.Sync(context.Background())
		finished := time.Now().UTC()
		st := adminSyncStatus{
			State:   "ok",
			Created: res.Created, Updated: res.Updated, Deleted: res.Deleted, Kept: res.Kept,
			StartedAt: &now, FinishedAt: &finished,
		}
		switch {
		case err == nil:
		case errors.Is(err, sync.ErrSyncRunning):
			st.State = "busy"
		case errors.Is(err, database.ErrSyncTooManyDeletions):
			st.State = "delete_blocked"
			slog.Warn("admin: cabinet sync deletion skipped by safety guard", "error", err)
		default:
			st.State = "failed"
			slog.Error("admin: cabinet sync failed", "error", err)
		}
		h.mu.Lock()
		h.status = st
		h.mu.Unlock()
		done = true
	})

	writeJSON(w, http.StatusOK, h.snapshot())
}

func (h *AdminSyncHandler) snapshot() adminSyncStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status
}
