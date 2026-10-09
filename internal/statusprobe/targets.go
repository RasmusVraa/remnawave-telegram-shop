package statusprobe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const targetsKey = "status_probe_targets"

const maxConfiguredTargets = 24

// ConfiguredTarget — страна, которую админ поставил на проверку.
// Address виден только в админке и в замере. В публичный JSON он не входит.
type ConfiguredTarget struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Country      string `json:"country"`
	Address      string `json:"address"`
	Enabled      bool   `json:"enabled"`
	IntervalMin  int    `json:"interval_min"`
	WorldProbes  int    `json:"world_probes"`
	RussiaProbes int    `json:"russia_probes"`
	// Whitelist — отдельная проверка через belspiski.online, без второго сервиса.
	Whitelist bool `json:"whitelist"`
	// Note — короткая приписка под названием, например «+ белые списки».
	Note string `json:"note,omitempty"`
}

// KVTargets хранит список стран в bot_runtime_settings, без новой таблицы.
type KVTargets struct {
	repo kvStore
}

func NewKVTargets(repo kvStore) *KVTargets {
	return &KVTargets{repo: repo}
}

func (s *KVTargets) Load(ctx context.Context) ([]ConfiguredTarget, error) {
	if s == nil || s.repo == nil {
		return []ConfiguredTarget{}, nil
	}
	all, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	raw := all[targetsKey]
	if strings.TrimSpace(raw) == "" {
		return []ConfiguredTarget{}, nil
	}
	var parsed []ConfiguredTarget
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return []ConfiguredTarget{}, nil
	}
	if parsed == nil {
		return []ConfiguredTarget{}, nil
	}
	return parsed, nil
}

func (s *KVTargets) Save(ctx context.Context, targets []ConfiguredTarget) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("status targets store is not configured")
	}
	if targets == nil {
		targets = []ConfiguredTarget{}
	}
	raw, err := json.Marshal(targets)
	if err != nil {
		return err
	}
	return s.repo.UpsertBatch(ctx, map[string]string{targetsKey: string(raw)}, nil)
}

// NormalizeTargets проверяет список перед записью и подставляет id.
func NormalizeTargets(in []ConfiguredTarget) ([]ConfiguredTarget, error) {
	if len(in) > maxConfiguredTargets {
		return nil, fmt.Errorf("too many targets")
	}
	out := make([]ConfiguredTarget, 0, len(in))
	seen := map[string]struct{}{}
	for _, item := range in {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return nil, fmt.Errorf("name is required")
		}
		if utf8.RuneCountInString(name) > 80 {
			return nil, fmt.Errorf("name is too long")
		}
		country := strings.ToUpper(strings.TrimSpace(item.Country))
		if len(country) != 2 || country[0] < 'A' || country[0] > 'Z' || country[1] < 'A' || country[1] > 'Z' {
			return nil, fmt.Errorf("country must be 2 letters")
		}
		host, ok := PublicTarget(item.Address)
		if !ok || isLocalHostname(host) {
			return nil, fmt.Errorf("address is not a public host")
		}
		interval := item.IntervalMin
		if interval == 0 {
			interval = ProbeIntervalMinutes()
		}
		if interval < 5 || interval > 180 {
			return nil, fmt.Errorf("interval must be between 5 and 180")
		}
		world := item.WorldProbes
		if world != 0 && (world < 1 || world > 10) {
			return nil, fmt.Errorf("world probes must be between 1 and 10")
		}
		russia := item.RussiaProbes
		if russia != 0 && (russia < 1 || russia > 30) {
			return nil, fmt.Errorf("russia probes must be between 1 and 30")
		}
		note := strings.TrimSpace(item.Note)
		if utf8.RuneCountInString(note) > 48 {
			return nil, fmt.Errorf("note is too long")
		}
		id := strings.TrimSpace(item.ID)
		if id == "" || len(id) > 40 {
			id = newTargetID()
		}
		if _, dup := seen[id]; dup {
			id = newTargetID()
		}
		seen[id] = struct{}{}
		out = append(out, ConfiguredTarget{
			ID:           id,
			Name:         name,
			Country:      country,
			Address:      host,
			Enabled:      item.Enabled,
			IntervalMin:  interval,
			WorldProbes:  world,
			RussiaProbes: russia,
			Whitelist:    item.Whitelist,
			Note:         note,
		})
	}
	return out, nil
}

func isLocalHostname(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	return h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local")
}

func newTargetID() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "t"
	}
	return hex.EncodeToString(buf)
}
