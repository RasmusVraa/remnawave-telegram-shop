package statusprobe

import (
	"context"
	"encoding/json"
	"time"
)

const historyKey = "status_probe_history"
const latestKey = "status_probe_latest"
const historyDays = 30

// DayRecord — итог одного дня по узлу. Хранится только счётчик успехов, без адресов.
type DayRecord struct {
	WorldOK     int `json:"wOk"`
	WorldTotal  int `json:"wN"`
	RussiaOK    int `json:"rOk"`
	RussiaTotal int `json:"rN"`
}

// HistoryStore читает и пишет журнал замеров. Реализация на bot_runtime_settings
// не требует новой таблицы.
type HistoryStore interface {
	Load(ctx context.Context) (map[string]map[string]DayRecord, error)
	Save(ctx context.Context, days map[string]map[string]DayRecord) error
}

type kvStore interface {
	GetAll(ctx context.Context) (map[string]string, error)
	UpsertBatch(ctx context.Context, settings map[string]string, updatedBy *int64) error
}

// KVHistory — журнал в существующей таблице ключ-значение.
type KVHistory struct {
	repo kvStore
}

func NewKVHistory(repo kvStore) *KVHistory {
	return &KVHistory{repo: repo}
}

func (h *KVHistory) Load(ctx context.Context) (map[string]map[string]DayRecord, error) {
	if h == nil || h.repo == nil {
		return map[string]map[string]DayRecord{}, nil
	}
	all, err := h.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	raw := all[historyKey]
	if raw == "" {
		return map[string]map[string]DayRecord{}, nil
	}
	var parsed map[string]map[string]DayRecord
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return map[string]map[string]DayRecord{}, nil
	}
	if parsed == nil {
		parsed = map[string]map[string]DayRecord{}
	}
	return parsed, nil
}

type persistedSample struct {
	WorldPingMs     *int       `json:"world_ping_ms,omitempty"`
	WorldOK         int        `json:"world_ok"`
	WorldTotal      int        `json:"world_total"`
	RussiaOK        int        `json:"russia_ok"`
	RussiaTotal     int        `json:"russia_total"`
	WhitelistOK     int        `json:"whitelist_ok,omitempty"`
	WhitelistTotal  int        `json:"whitelist_total,omitempty"`
	WhitelistPingMs *int       `json:"whitelist_ping_ms,omitempty"`
	MeasuredAt      time.Time  `json:"measured_at"`
	Probes          []ProbeHit `json:"probes,omitempty"`
}

func (h *KVHistory) LoadLatest(ctx context.Context) (map[string]Sample, error) {
	if h == nil || h.repo == nil {
		return map[string]Sample{}, nil
	}
	all, err := h.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	raw := all[latestKey]
	if raw == "" {
		return map[string]Sample{}, nil
	}
	var parsed map[string]persistedSample
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return map[string]Sample{}, nil
	}
	out := make(map[string]Sample, len(parsed))
	for key, item := range parsed {
		out[key] = Sample{
			WorldPingMs:     item.WorldPingMs,
			WorldOK:         item.WorldOK,
			WorldTotal:      item.WorldTotal,
			RussiaOK:        item.RussiaOK,
			RussiaTotal:     item.RussiaTotal,
			WhitelistOK:     item.WhitelistOK,
			WhitelistTotal:  item.WhitelistTotal,
			WhitelistPingMs: item.WhitelistPingMs,
			MeasuredAt:      item.MeasuredAt,
			Probes:          item.Probes,
		}
	}
	return out, nil
}

func (h *KVHistory) SaveLatest(ctx context.Context, samples map[string]Sample) error {
	if h == nil || h.repo == nil {
		return nil
	}
	payload := make(map[string]persistedSample, len(samples))
	for key, sample := range samples {
		payload[key] = persistedSample{
			WorldPingMs:     sample.WorldPingMs,
			WorldOK:         sample.WorldOK,
			WorldTotal:      sample.WorldTotal,
			RussiaOK:        sample.RussiaOK,
			RussiaTotal:     sample.RussiaTotal,
			WhitelistOK:     sample.WhitelistOK,
			WhitelistTotal:  sample.WhitelistTotal,
			WhitelistPingMs: sample.WhitelistPingMs,
			MeasuredAt:      sample.MeasuredAt,
			Probes:          sample.Probes,
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return h.repo.UpsertBatch(ctx, map[string]string{latestKey: string(raw)}, nil)
}

func (h *KVHistory) Save(ctx context.Context, days map[string]map[string]DayRecord) error {
	if h == nil || h.repo == nil {
		return nil
	}
	raw, err := json.Marshal(days)
	if err != nil {
		return err
	}
	return h.repo.UpsertBatch(ctx, map[string]string{historyKey: string(raw)}, nil)
}

// NodeKey связывает замер с узлом без адреса.
func NodeKey(country, name string) string {
	return country + "\n" + name
}

// RecordDay дописывает сегодняшний замер и выкидывает дни старше окна.
func RecordDay(history map[string]map[string]DayRecord, key string, sample Sample, now time.Time) {
	if history == nil {
		return
	}
	if sample.WorldTotal == 0 && sample.RussiaTotal == 0 {
		return
	}
	day := now.UTC().Format("2006-01-02")
	perNode := history[key]
	if perNode == nil {
		perNode = map[string]DayRecord{}
	}
	prev := perNode[day]
	perNode[day] = DayRecord{
		WorldOK:     prev.WorldOK + sample.WorldOK,
		WorldTotal:  prev.WorldTotal + sample.WorldTotal,
		RussiaOK:    prev.RussiaOK + sample.RussiaOK,
		RussiaTotal: prev.RussiaTotal + sample.RussiaTotal,
	}
	cutoff := now.UTC().AddDate(0, 0, -(historyDays - 1)).Format("2006-01-02")
	for d := range perNode {
		if d < cutoff {
			delete(perNode, d)
		}
	}
	history[key] = perNode
}

// Window возвращает 30 долей успешности от старых к новым. nil — замера в этот день не было.
func Window(perNode map[string]DayRecord, now time.Time) []*float64 {
	out := make([]*float64, historyDays)
	if perNode == nil {
		return out
	}
	start := now.UTC().AddDate(0, 0, -(historyDays - 1))
	for i := 0; i < historyDays; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		rec, ok := perNode[day]
		total := rec.WorldTotal + rec.RussiaTotal
		if !ok || total == 0 {
			continue
		}
		v := float64(rec.WorldOK+rec.RussiaOK) / float64(total)
		out[i] = &v
	}
	return out
}
