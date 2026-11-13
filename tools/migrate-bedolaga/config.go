package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is migrate.yaml for Bedolaga → shop migration.
type Config struct {
	Source    SourceConfig    `yaml:"source"`
	Target    TargetConfig    `yaml:"target"`
	Remnawave RemnawaveConfig `yaml:"remnawave"`
	Tariffs   TariffsConfig   `yaml:"tariffs"`
	Balance   BalanceConfig   `yaml:"balance"`
	Customers CustomersConfig `yaml:"customers"`
	Referrals ReferralsConfig `yaml:"referrals"`
	Reporting ReportingConfig `yaml:"reporting"`
}

type SourceConfig struct {
	DatabaseURL string `yaml:"database_url"`
}

type TargetConfig struct {
	DatabaseURL string `yaml:"database_url"`
}

type RemnawaveConfig struct {
	BaseURL string `yaml:"base_url"`
	Token   string `yaml:"token"`
	Mode    string `yaml:"mode"` // local|remote — passed to remnawave.NewClient
}

type TariffsConfig struct {
	Mode    string         `yaml:"mode"` // import_from_bedolaga | map_existing
	Mapping map[int]string `yaml:"mapping"`
}

type BalanceConfig struct {
	Enabled          bool   `yaml:"enabled"`
	Policy           string `yaml:"policy"`          // user_mapped_tariff
	FallbackPolicy   string `yaml:"fallback_policy"` // cheapest_imported_1m
	ApplyToRemnawave bool   `yaml:"apply_to_remnawave"`
}

type CustomersConfig struct {
	OnConflict       string `yaml:"on_conflict"` // prefer_bedolaga
	SetLegalAccepted bool   `yaml:"set_legal_accepted"`
	SkipDeleted      bool   `yaml:"skip_deleted"`
	SkipBlocked      bool   `yaml:"skip_blocked"`
	ImportCabinet    bool   `yaml:"import_cabinet"`
}

type ReferralsConfig struct {
	ImportGraph bool `yaml:"import_graph"`
}

type ReportingConfig struct {
	Dir string `yaml:"dir"`
}

func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Tariffs.Mode == "" {
		c.Tariffs.Mode = "import_from_bedolaga"
	}
	if c.Balance.Policy == "" {
		c.Balance.Policy = "user_mapped_tariff"
	}
	if c.Balance.FallbackPolicy == "" {
		c.Balance.FallbackPolicy = "cheapest_imported_1m"
	}
	if c.Customers.OnConflict == "" {
		c.Customers.OnConflict = "prefer_bedolaga"
	}
	if c.Reporting.Dir == "" {
		c.Reporting.Dir = "./migrate-out"
	}
	if c.Remnawave.Mode == "" {
		c.Remnawave.Mode = "local"
	}
}

func (c *Config) validate() error {
	if c.Source.DatabaseURL == "" {
		return fmt.Errorf("source.database_url is required")
	}
	if c.Target.DatabaseURL == "" {
		return fmt.Errorf("target.database_url is required")
	}
	return nil
}

// DefaultConfigTemplate returns example YAML for the wizard / docs.
func DefaultConfigTemplate() string {
	return `# Bedolaga → Meows shop migration config
source:
  database_url: postgres://postgres:migrator@127.0.0.1:5433/bedolaga_restore?sslmode=disable
target:
  database_url: postgres://user:pass@127.0.0.1:5432/shop?sslmode=disable
remnawave:
  base_url: http://remnawave:3000
  token: CHANGE_ME
  mode: local
tariffs:
  mode: import_from_bedolaga   # or map_existing
  mapping: {}                  # bedolaga_tariff_id: our_slug (for map_existing)
balance:
  enabled: true
  policy: user_mapped_tariff
  fallback_policy: cheapest_imported_1m
  apply_to_remnawave: true     # only on --step balance --apply
customers:
  on_conflict: prefer_bedolaga
  set_legal_accepted: true
  skip_deleted: true
  skip_blocked: true
  import_cabinet: true
referrals:
  import_graph: true
reporting:
  dir: ./migrate-out
`
}
