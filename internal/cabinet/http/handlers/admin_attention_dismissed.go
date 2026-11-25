package handlers

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// maxDismissBatch — «Скрыть всё» не бывает длиннее: пунктов «Требует внимания» единицы.
const maxDismissBatch = 100

// AdminAttentionDismissed — какие пункты «Требует внимания» админ уже прочитал.
// Сам список пунктов считается на лету; тут только отметки «скрыто» на аккаунт.
type AdminAttentionDismissed struct {
	pool *pgxpool.Pool
}

// NewAdminAttentionDismissed — конструктор.
func NewAdminAttentionDismissed(pool *pgxpool.Pool) *AdminAttentionDismissed {
	return &AdminAttentionDismissed{pool: pool}
}

type dismissedItem struct {
	Key      string     `json:"key"`
	Count    int        `json:"count"`
	Severity string     `json:"severity"`
	Since    *time.Time `json:"since,omitempty"`
}

type dismissBody struct {
	Items []dismissedItem `json:"items"`
}

// Handle — GET (список скрытых), POST (скрыть пункты), DELETE (вернуть: ?key=… один
// пункт, без key — все).
func (h *AdminAttentionDismissed) Handle(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	account := adminActorID(r)
	if account == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		h.list(w, r, account)
	case http.MethodPost:
		h.dismiss(w, r, account)
	case http.MethodDelete:
		h.restore(w, r, account)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *AdminAttentionDismissed) list(w http.ResponseWriter, r *http.Request, account int64) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT item_key, count, severity, since FROM cabinet_admin_attention_dismissed WHERE account_id = $1`, account)
	if err != nil {
		slog.Error("admin attention dismissed: list", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	items := []dismissedItem{}
	for rows.Next() {
		var it dismissedItem
		if err := rows.Scan(&it.Key, &it.Count, &it.Severity, &it.Since); err != nil {
			slog.Error("admin attention dismissed: scan", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *AdminAttentionDismissed) dismiss(w http.ResponseWriter, r *http.Request, account int64) {
	var body dismissBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if len(body.Items) == 0 || len(body.Items) > maxDismissBatch {
		http.Error(w, "invalid items", http.StatusBadRequest)
		return
	}
	batch := &pgx.Batch{}
	for _, it := range body.Items {
		key := strings.TrimSpace(it.Key)
		if key == "" || len(key) > 160 {
			http.Error(w, "invalid key", http.StatusBadRequest)
			return
		}
		sev := "warn"
		if it.Severity == "crit" {
			sev = "crit"
		}
		count := it.Count
		if count < 1 {
			count = 1
		}
		batch.Queue(`
			INSERT INTO cabinet_admin_attention_dismissed (account_id, item_key, count, severity, since)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (account_id, item_key)
			DO UPDATE SET count = EXCLUDED.count, severity = EXCLUDED.severity, since = EXCLUDED.since, created_at = NOW()`,
			account, key, count, sev, it.Since)
	}
	res := h.pool.SendBatch(r.Context(), batch)
	for range body.Items {
		if _, err := res.Exec(); err != nil {
			_ = res.Close()
			slog.Error("admin attention dismissed: upsert", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
	if err := res.Close(); err != nil {
		slog.Error("admin attention dismissed: close batch", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *AdminAttentionDismissed) restore(w http.ResponseWriter, r *http.Request, account int64) {
	var err error
	if key := strings.TrimSpace(r.URL.Query().Get("key")); key != "" {
		_, err = h.pool.Exec(r.Context(),
			`DELETE FROM cabinet_admin_attention_dismissed WHERE account_id = $1 AND item_key = $2`, account, key)
	} else {
		_, err = h.pool.Exec(r.Context(),
			`DELETE FROM cabinet_admin_attention_dismissed WHERE account_id = $1`, account)
	}
	if err != nil {
		slog.Error("admin attention dismissed: restore", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
