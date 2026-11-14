package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type problemRow struct {
	Code    string `json:"code"`
	Entity  string `json:"entity"`
	Ref     string `json:"ref"`
	Message string `json:"message"`
}

type customerReportRow struct {
	BedolagaUserID int    `json:"bedolaga_user_id"`
	TelegramID     string `json:"telegram_id"`
	Email          string `json:"email"`
	Status         string `json:"status"`
	TariffSlug     string `json:"tariff_slug"`
	BalanceDays    int    `json:"balance_days"`
	RWDays         int    `json:"rw_days"`
	TargetDays     int    `json:"target_days"`
	RWStatus       string `json:"rw_status"`
	Notes          string `json:"notes"`
}

type tariffReportRow struct {
	BedolagaID int    `json:"bedolaga_id"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Notes      string `json:"notes"`
}

type summaryReport struct {
	DryRun             bool     `json:"dry_run"`
	Step               string   `json:"step"`
	BedolagaGeneration string   `json:"bedolaga_generation_hint"`
	Warnings           []string `json:"warnings"`
	TariffsOK          int      `json:"tariffs_ok"`
	CustomersOK        int      `json:"customers_ok"`
	CustomersCabinet   int      `json:"customers_cabinet"`
	BalanceApplied     int      `json:"balance_applied"`
	ReferralsOK        int      `json:"referrals_ok"`
	PaymentsOK         int      `json:"payments_ok"`
	PaymentsSkipped    int      `json:"payments_skipped"`
	Problems           int      `json:"problems"`
}

type reporter struct {
	dir       string
	mu        sync.Mutex
	problems  []problemRow
	customers []customerReportRow
	tariffs   []tariffReportRow
	summary   summaryReport
}

func newReporter(dir string) (*reporter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &reporter{dir: dir}, nil
}

func (r *reporter) addProblem(code, entity, ref, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.problems = append(r.problems, problemRow{Code: code, Entity: entity, Ref: ref, Message: msg})
}

func (r *reporter) addCustomer(row customerReportRow) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.customers = append(r.customers, row)
}

func (r *reporter) addTariff(row tariffReportRow) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tariffs = append(r.tariffs, row)
}

func (r *reporter) Flush() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.summary.Problems = len(r.problems)
	if err := writeJSON(filepath.Join(r.dir, "summary.json"), r.summary); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(r.dir, "problems.json"), r.problems); err != nil {
		return err
	}
	if err := writeCSV(filepath.Join(r.dir, "problems.csv"), []string{"code", "entity", "ref", "message"}, func(w *csv.Writer) error {
		for _, p := range r.problems {
			if err := w.Write([]string{p.Code, p.Entity, p.Ref, p.Message}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := writeCSV(filepath.Join(r.dir, "customers.csv"),
		[]string{"bedolaga_user_id", "telegram_id", "email", "status", "tariff_slug", "balance_days", "rw_days", "target_days", "rw_status", "notes"},
		func(w *csv.Writer) error {
			rows := append([]customerReportRow(nil), r.customers...)
			sort.Slice(rows, func(i, j int) bool { return rows[i].BedolagaUserID < rows[j].BedolagaUserID })
			for _, c := range rows {
				if err := w.Write([]string{
					fmt.Sprintf("%d", c.BedolagaUserID), c.TelegramID, c.Email, c.Status, c.TariffSlug,
					fmt.Sprintf("%d", c.BalanceDays), fmt.Sprintf("%d", c.RWDays), fmt.Sprintf("%d", c.TargetDays),
					c.RWStatus, c.Notes,
				}); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
		return err
	}
	if err := writeCSV(filepath.Join(r.dir, "tariffs.csv"),
		[]string{"bedolaga_id", "slug", "name", "status", "notes"},
		func(w *csv.Writer) error {
			for _, t := range r.tariffs {
				if err := w.Write([]string{fmt.Sprintf("%d", t.BedolagaID), t.Slug, t.Name, t.Status, t.Notes}); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
		return err
	}
	return writeCSV(filepath.Join(r.dir, "balances.csv"),
		[]string{"bedolaga_user_id", "telegram_id", "balance_days", "rw_days", "target_days", "status", "notes"},
		func(w *csv.Writer) error {
			for _, c := range r.customers {
				if c.BalanceDays == 0 && c.TargetDays == 0 {
					continue
				}
				if err := w.Write([]string{
					fmt.Sprintf("%d", c.BedolagaUserID), c.TelegramID,
					fmt.Sprintf("%d", c.BalanceDays), fmt.Sprintf("%d", c.RWDays), fmt.Sprintf("%d", c.TargetDays),
					c.Status, c.Notes,
				}); err != nil {
					return err
				}
			}
			return nil
		})
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func writeCSV(path string, header []string, writeRows func(*csv.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		return err
	}
	if err := writeRows(w); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}
