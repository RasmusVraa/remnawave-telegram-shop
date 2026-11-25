package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"remnawave-tg-shop-bot/internal/cabinet/repository"
)

// AdminExpensesHandler — журнал расходов и P/L месяца в админке.
type AdminExpensesHandler struct {
	repo *repository.ExpenseRepo
}

func NewAdminExpenses(repo *repository.ExpenseRepo) *AdminExpensesHandler {
	return &AdminExpensesHandler{repo: repo}
}

type adminExpenseDTO struct {
	ID         int64   `json:"id"`
	OccurredOn string  `json:"occurred_on"`
	AmountRub  float64 `json:"amount_rub"`
	Category   string  `json:"category"`
	Title      string  `json:"title"`
	Note       string  `json:"note"`
	Icon       string  `json:"icon"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

type adminExpenseCategoryDTO struct {
	Category  string  `json:"category"`
	AmountRub float64 `json:"amount_rub"`
	Count     int64   `json:"count"`
}

type adminExpenseSummaryDTO struct {
	Year         int                       `json:"year"`
	Month        int                       `json:"month"`
	RevenueRub   float64                   `json:"revenue_rub"`
	ExpensesRub  float64                   `json:"expenses_rub"`
	ProfitRub    float64                   `json:"profit_rub"`
	ByCategory   []adminExpenseCategoryDTO `json:"by_category"`
}

type adminExpenseListDTO struct {
	Items   []adminExpenseDTO      `json:"items"`
	Summary adminExpenseSummaryDTO `json:"summary"`
}

type adminExpenseWriteReq struct {
	OccurredOn string  `json:"occurred_on"`
	AmountRub  float64 `json:"amount_rub"`
	Category   string  `json:"category"`
	Title      string  `json:"title"`
	Note       string  `json:"note"`
	Icon       string  `json:"icon"`
}

func toExpenseDTO(e *repository.Expense) adminExpenseDTO {
	return adminExpenseDTO{
		ID:         e.ID,
		OccurredOn: e.OccurredOn.UTC().Format("2006-01-02"),
		AmountRub:  e.AmountRub,
		Category:   e.Category,
		Title:      e.Title,
		Note:       e.Note,
		Icon:       e.Icon,
		CreatedAt:  e.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  e.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func parseExpenseMonth(r *http.Request) (int, time.Month, bool) {
	now := time.Now().UTC()
	y, m := now.Year(), now.Month()
	q := r.URL.Query()
	if ys := strings.TrimSpace(q.Get("year")); ys != "" {
		n, err := strconv.Atoi(ys)
		if err != nil || n < 2000 || n > 2100 {
			return 0, 0, false
		}
		y = n
	}
	if ms := strings.TrimSpace(q.Get("month")); ms != "" {
		n, err := strconv.Atoi(ms)
		if err != nil || n < 1 || n > 12 {
			return 0, 0, false
		}
		m = time.Month(n)
	}
	return y, m, true
}

func parseExpenseWrite(r *http.Request) (repository.ExpenseInput, error) {
	var req adminExpenseWriteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return repository.ExpenseInput{}, err
	}
	day, err := time.Parse("2006-01-02", strings.TrimSpace(req.OccurredOn))
	if err != nil {
		return repository.ExpenseInput{}, repository.ErrExpenseValidation
	}
	return repository.ExpenseInput{
		OccurredOn: day,
		AmountRub:  req.AmountRub,
		Category:   req.Category,
		Title:      req.Title,
		Note:       req.Note,
		Icon:       req.Icon,
	}, nil
}

func (h *AdminExpensesHandler) buildSummary(r *http.Request, year int, month time.Month) (adminExpenseSummaryDTO, error) {
	expenses, byCat, err := h.repo.SumMonth(r.Context(), year, month)
	if err != nil {
		return adminExpenseSummaryDTO{}, err
	}
	revenue, err := h.repo.RevenueMonthRub(r.Context(), year, month)
	if err != nil {
		return adminExpenseSummaryDTO{}, err
	}
	cats := make([]adminExpenseCategoryDTO, 0, len(byCat))
	for _, c := range byCat {
		cats = append(cats, adminExpenseCategoryDTO{Category: c.Category, AmountRub: c.AmountRub, Count: c.Count})
	}
	return adminExpenseSummaryDTO{
		Year: year, Month: int(month),
		RevenueRub: revenue, ExpensesRub: expenses, ProfitRub: revenue - expenses,
		ByCategory: cats,
	}, nil
}

// List — GET /cabinet/api/admin/expenses?year=&month=
func (h *AdminExpensesHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	year, month, ok := parseExpenseMonth(r)
	if !ok {
		http.Error(w, "invalid month", http.StatusBadRequest)
		return
	}
	items, err := h.repo.ListMonth(r.Context(), year, month)
	if err != nil {
		slog.Error("admin expenses list", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	summary, err := h.buildSummary(r, year, month)
	if err != nil {
		slog.Error("admin expenses summary", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := make([]adminExpenseDTO, 0, len(items))
	for i := range items {
		out = append(out, toExpenseDTO(&items[i]))
	}
	writeJSON(w, http.StatusOK, adminExpenseListDTO{Items: out, Summary: summary})
}

// Create — POST /cabinet/api/admin/expenses
func (h *AdminExpensesHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	in, err := parseExpenseWrite(r)
	if err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	e, err := h.repo.Create(r.Context(), in)
	if errors.Is(err, repository.ErrExpenseValidation) {
		http.Error(w, "invalid expense", http.StatusBadRequest)
		return
	}
	if err != nil {
		slog.Error("admin expenses create", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, toExpenseDTO(e))
}

// HandleByID — PATCH/DELETE /cabinet/api/admin/expenses/{id}
func (h *AdminExpensesHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/cabinet/api/admin/expenses/")
	idStr = strings.Trim(idStr, "/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodPatch:
		in, err := parseExpenseWrite(r)
		if err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		e, err := h.repo.Update(r.Context(), id, in)
		if errors.Is(err, repository.ErrExpenseValidation) {
			http.Error(w, "invalid expense", http.StatusBadRequest)
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			slog.Error("admin expenses update", "error", err, "id", id)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, toExpenseDTO(e))
	case http.MethodDelete:
		if err := h.repo.Delete(r.Context(), id); errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		} else if err != nil {
			slog.Error("admin expenses delete", "error", err, "id", id)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		methodNotAllowed(w, http.MethodPatch+", "+http.MethodDelete)
	}
}
