package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	cabrepo "remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Migrator struct {
	cfg        *Config
	dryRun     bool
	step       string
	src        *sourceDB
	target     *pgxpool.Pool
	rw         *remnawave.Client
	rep        *reporter
	custRepo   *database.CustomerRepository
	tariffRepo *database.TariffRepository
	refRepo    *database.ReferralRepository
	accRepo    *cabrepo.AccountRepo
	idRepo     *cabrepo.IdentityRepo
	linkRepo   *cabrepo.AccountCustomerLinkRepo

	// bedolaga tariff id → our tariff id / slug / price 1m
	tariffMap       map[int]int64
	tariffSlug      map[int]string
	tariffPrice1m   map[int]int
	cheapestPrice1m int

	rwByTelegram map[int64]*remnawave.User
	rwByShort    map[string]*remnawave.User

	// bedolaga user id → our customer telegram_id (for referrals)
	bdgUserToTG map[int]int64
}

func newMigrator(cfg *Config, dryRun bool, step string) (*Migrator, error) {
	rep, err := newReporter(cfg.Reporting.Dir)
	if err != nil {
		return nil, err
	}
	rep.summary.DryRun = dryRun
	rep.summary.Step = step
	return &Migrator{
		cfg:             cfg,
		dryRun:          dryRun,
		step:            step,
		rep:             rep,
		tariffMap:       map[int]int64{},
		tariffSlug:      map[int]string{},
		tariffPrice1m:   map[int]int{},
		cheapestPrice1m: 0,
		rwByTelegram:    map[int64]*remnawave.User{},
		rwByShort:       map[string]*remnawave.User{},
		bdgUserToTG:     map[int]int64{},
	}, nil
}

func (m *Migrator) Run(ctx context.Context) error {
	src, err := openSource(ctx, m.cfg.Source.DatabaseURL)
	if err != nil {
		return err
	}
	m.src = src
	defer src.Close()

	m.rep.summary.BedolagaGeneration = src.caps.HintGeneration
	for _, w := range src.caps.WarningLines() {
		m.rep.summary.Warnings = append(m.rep.summary.Warnings, w)
		slog.Warn(w)
		m.rep.addProblem("compat_warning", "source", src.caps.HintGeneration, w)
	}

	target, err := pgxpool.New(ctx, m.cfg.Target.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect target db: %w", err)
	}
	m.target = target
	defer target.Close()

	m.custRepo = database.NewCustomerRepository(target)
	m.tariffRepo = database.NewTariffRepository(target)
	m.refRepo = database.NewReferralRepository(target)
	m.accRepo = cabrepo.NewAccountRepo(target)
	m.idRepo = cabrepo.NewIdentityRepo(target)
	m.linkRepo = cabrepo.NewAccountCustomerLinkRepo(target)

	if strings.TrimSpace(m.cfg.Remnawave.BaseURL) != "" && strings.TrimSpace(m.cfg.Remnawave.Token) != "" {
		m.rw = remnawave.NewClient(m.cfg.Remnawave.BaseURL, m.cfg.Remnawave.Token, m.cfg.Remnawave.Mode)
		if err := m.loadRemnawaveIndex(ctx); err != nil {
			m.rep.addProblem("rw_load_failed", "remnawave", "", err.Error())
			slog.Warn("remnawave index failed — continuing with empty RW index", "err", err)
		}
	} else {
		m.rep.addProblem("rw_not_configured", "remnawave", "", "base_url/token empty — RW reconcile skipped")
	}

	steps := m.expandSteps()
	for _, st := range steps {
		slog.Info("migrate step", "step", st, "dry_run", m.dryRun)
		switch st {
		case "tariffs":
			if err := m.stepTariffs(ctx); err != nil {
				return err
			}
		case "customers":
			if err := m.stepCustomers(ctx); err != nil {
				return err
			}
		case "balance":
			if err := m.stepBalance(ctx); err != nil {
				return err
			}
		case "referrals":
			if err := m.stepReferrals(ctx); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown step %q", st)
		}
	}
	return m.rep.Flush()
}

func (m *Migrator) expandSteps() []string {
	switch m.step {
	case "", "all":
		return []string{"tariffs", "customers", "balance", "referrals"}
	default:
		return []string{m.step}
	}
}

func (m *Migrator) loadRemnawaveIndex(ctx context.Context) error {
	users, err := m.rw.GetUsers(ctx)
	if err != nil {
		return err
	}
	for i := range users {
		u := &users[i]
		if u.TelegramID != nil && *u.TelegramID != 0 {
			m.rwByTelegram[*u.TelegramID] = u
		}
		if u.ShortUUID != "" {
			m.rwByShort[u.ShortUUID] = u
		}
	}
	slog.Info("remnawave users indexed", "count", len(users))
	return nil
}

func (m *Migrator) resolveRW(u bedolagaUser, sub *bedolagaSub) (*remnawave.User, string) {
	if u.TelegramID != nil {
		if rw := m.rwByTelegram[*u.TelegramID]; rw != nil {
			return rw, "telegram_id"
		}
	}
	// Сопоставление по remnawave uuid больше невозможно: в панели 3.0.0 поле `uuid`
	// у пользователя удалено, адресация идёт по числовому id. Записи Bedolaga,
	// у которых есть только uuid и нет ни telegram_id, ни short_uuid, останутся
	// несопоставленными и попадут в отчёт как unmatched.
	if sub != nil && sub.RemnawaveShortUUID != nil && *sub.RemnawaveShortUUID != "" {
		if rw := m.rwByShort[*sub.RemnawaveShortUUID]; rw != nil {
			return rw, "short_uuid"
		}
	}
	return nil, ""
}

func rwRemainingDays(rw *remnawave.User, now time.Time) int {
	if rw == nil {
		return 0
	}
	if !rw.ExpireAt.After(now) {
		return 0
	}
	return int(rw.ExpireAt.Sub(now).Hours() / 24)
}

// --- tariffs ---

func (m *Migrator) stepTariffs(ctx context.Context) error {
	bdg, err := m.src.LoadTariffs(ctx)
	if err != nil {
		return err
	}
	if m.cfg.Tariffs.Mode == "map_existing" {
		return m.mapExistingTariffs(ctx, bdg)
	}
	existing, err := m.tariffRepo.ListAll(ctx)
	if err != nil {
		return err
	}
	bySlug := map[string]database.Tariff{}
	for _, t := range existing {
		bySlug[t.Slug] = t
	}

	for _, t := range bdg {
		slug := BedolagaTariffSlug(t.ID, t.Name)
		notes := ""
		if t.IsDaily {
			notes = "daily_skipped_as_feature;"
			m.rep.addProblem("daily_tariff", "tariff", fmt.Sprintf("%d", t.ID), "daily tariff imported as normal; daily billing not supported")
		}
		rub, priceNotes := periodPricesToRub(t.PeriodPricesJSON)
		notes += priceNotes
		squads := parseAllowedSquads(t.AllowedSquadsJSON)
		squadStr := strings.Join(squads, ",")

		var ext *uuid.UUID
		if t.ExternalSquadUUID != nil && *t.ExternalSquadUUID != "" {
			if parsed, err := uuid.Parse(*t.ExternalSquadUUID); err == nil {
				ext = &parsed
			} else {
				m.rep.addProblem("bad_external_squad", "tariff", fmt.Sprintf("%d", t.ID), err.Error())
			}
		}

		trafficBytes := int64(t.TrafficLimitGB) * 1024 * 1024 * 1024
		if t.TrafficLimitGB <= 0 {
			trafficBytes = 0
		}
		reset := "MONTH"
		if t.TrafficResetMode != nil && *t.TrafficResetMode != "" {
			reset = strings.ToUpper(*t.TrafficResetMode)
		}
		tier := t.TierLevel
		name := t.Name
		desc := t.Description

		var status string
		ourID := int64(0)
		if prev, ok := bySlug[slug]; ok {
			ourID = prev.ID
			status = "exists_updated"
			if !m.dryRun {
				upd := map[string]interface{}{
					"name":                         name,
					"sort_order":                   t.DisplayOrder,
					"is_active":                    t.IsActive && !t.IsDaily,
					"device_limit":                 t.DeviceLimit,
					"traffic_limit_bytes":          trafficBytes,
					"traffic_limit_reset_strategy": reset,
					"active_internal_squad_uuids":  squadStr,
					"external_squad_uuid":          ext,
					"tier_level":                   tier,
				}
				if desc != nil {
					upd["description"] = *desc
				}
				if err := m.tariffRepo.UpdateTariff(ctx, ourID, upd); err != nil {
					m.rep.addProblem("tariff_update_failed", "tariff", slug, err.Error())
					status = "failed"
				} else {
					_ = m.tariffRepo.ReplaceAllPrices(ctx, ourID, rub, [4]*int{})
				}
			}
		} else if !m.dryRun {
			nt := &database.Tariff{
				Slug:                      slug,
				Name:                      &name,
				SortOrder:                 t.DisplayOrder,
				IsActive:                  t.IsActive && !t.IsDaily,
				DeviceLimit:               t.DeviceLimit,
				TrafficLimitBytes:         trafficBytes,
				TrafficLimitResetStrategy: reset,
				ActiveInternalSquadUUIDs:  squadStr,
				ExternalSquadUUID:         ext,
				TierLevel:                 &tier,
				Description:               desc,
			}
			id, err := m.tariffRepo.CreateWithPrices(ctx, nt, rub, [4]*int{})
			if err != nil {
				m.rep.addProblem("tariff_create_failed", "tariff", slug, err.Error())
				status = "failed"
			} else {
				ourID = id
				status = "created"
			}
		} else {
			status = "would_create"
		}

		m.tariffMap[t.ID] = ourID
		m.tariffSlug[t.ID] = slug
		if rub[0] > 0 {
			m.tariffPrice1m[t.ID] = rub[0]
			if m.cheapestPrice1m == 0 || rub[0] < m.cheapestPrice1m {
				m.cheapestPrice1m = rub[0]
			}
		}
		if status == "ok" || status == "created" || status == "exists_updated" || status == "would_create" {
			m.rep.summary.TariffsOK++
		}
		m.rep.addTariff(tariffReportRow{BedolagaID: t.ID, Slug: slug, Name: t.Name, Status: status, Notes: notes})
	}
	return nil
}

func (m *Migrator) mapExistingTariffs(ctx context.Context, bdg []bedolagaTariff) error {
	existing, err := m.tariffRepo.ListAll(ctx)
	if err != nil {
		return err
	}
	bySlug := map[string]database.Tariff{}
	for _, t := range existing {
		bySlug[t.Slug] = t
		if p, err := m.tariffRepo.GetPrice(ctx, t.ID, 1); err == nil && p != nil && p.AmountRub > 0 {
			if m.cheapestPrice1m == 0 || p.AmountRub < m.cheapestPrice1m {
				m.cheapestPrice1m = p.AmountRub
			}
		}
	}
	for _, t := range bdg {
		slug, ok := m.cfg.Tariffs.Mapping[t.ID]
		if !ok || slug == "" {
			m.rep.addProblem("tariff_unmapped", "tariff", fmt.Sprintf("%d", t.ID), "no mapping in migrate.yaml")
			m.rep.addTariff(tariffReportRow{BedolagaID: t.ID, Name: t.Name, Status: "unmapped"})
			continue
		}
		ot, ok := bySlug[slug]
		if !ok {
			m.rep.addProblem("tariff_slug_missing", "tariff", slug, "slug not in target DB")
			m.rep.addTariff(tariffReportRow{BedolagaID: t.ID, Slug: slug, Name: t.Name, Status: "missing"})
			continue
		}
		m.tariffMap[t.ID] = ot.ID
		m.tariffSlug[t.ID] = slug
		if p, err := m.tariffRepo.GetPrice(ctx, ot.ID, 1); err == nil && p != nil {
			m.tariffPrice1m[t.ID] = p.AmountRub
		}
		m.rep.summary.TariffsOK++
		m.rep.addTariff(tariffReportRow{BedolagaID: t.ID, Slug: slug, Name: t.Name, Status: "mapped"})
	}
	return nil
}

func periodPricesToRub(raw []byte) ([4]int, string) {
	var rub [4]int
	notes := ""
	prices := parsePeriodPrices(raw) // days → kopeks
	monthIdx := map[int]int{1: 0, 3: 1, 6: 2, 12: 3}
	for days, kopeks := range prices {
		months := NormalizePeriodDays(days)
		if months == 0 {
			notes += fmt.Sprintf("price_%dd_skipped;", days)
			continue
		}
		rub[monthIdx[months]] = kopeks / 100
	}
	// fill missing from 1m if present
	if rub[0] > 0 {
		for i, mult := range []int{1, 3, 6, 12} {
			if rub[i] == 0 {
				rub[i] = rub[0] * mult
				notes += fmt.Sprintf("price_%dm_estimated;", mult)
			}
		}
	}
	return rub, notes
}

// --- customers ---

func (m *Migrator) ensureTariffMaps(ctx context.Context) error {
	if len(m.tariffMap) > 0 {
		return nil
	}
	bdg, err := m.src.LoadTariffs(ctx)
	if err != nil {
		return err
	}
	existing, err := m.tariffRepo.ListAll(ctx)
	if err != nil {
		return err
	}
	bySlug := map[string]database.Tariff{}
	for _, t := range existing {
		bySlug[t.Slug] = t
	}
	for _, t := range bdg {
		slug := ""
		if m.cfg.Tariffs.Mode == "map_existing" {
			slug = m.cfg.Tariffs.Mapping[t.ID]
		} else {
			slug = BedolagaTariffSlug(t.ID, t.Name)
		}
		if slug == "" {
			continue
		}
		ot, ok := bySlug[slug]
		if !ok {
			continue
		}
		m.tariffMap[t.ID] = ot.ID
		m.tariffSlug[t.ID] = slug
		if p, err := m.tariffRepo.GetPrice(ctx, ot.ID, 1); err == nil && p != nil && p.AmountRub > 0 {
			m.tariffPrice1m[t.ID] = p.AmountRub
			if m.cheapestPrice1m == 0 || p.AmountRub < m.cheapestPrice1m {
				m.cheapestPrice1m = p.AmountRub
			}
		}
	}
	return nil
}

func (m *Migrator) stepCustomers(ctx context.Context) error {
	if err := m.ensureTariffMaps(ctx); err != nil {
		slog.Warn("tariff map preload failed", "err", err)
	}

	users, err := m.src.LoadUsers(ctx)
	if err != nil {
		return err
	}
	subs, err := m.src.LoadSubscriptions(ctx)
	if err != nil {
		return err
	}
	subsByUser := map[int][]bedolagaSub{}
	for _, s := range subs {
		subsByUser[s.UserID] = append(subsByUser[s.UserID], s)
	}

	now := time.Now().UTC()
	for _, u := range users {
		if m.cfg.Customers.SkipDeleted && u.Status == "deleted" {
			continue
		}
		if m.cfg.Customers.SkipBlocked && u.Status == "blocked" {
			continue
		}

		userSubs := subsByUser[u.ID]
		primary, orphans := pickPrimarySub(userSubs)
		if orphans > 0 {
			m.rep.addProblem("multi_sub", "user", fmt.Sprintf("%d", u.ID), fmt.Sprintf("%d extra active/trial subscriptions", orphans))
		}

		rw, rwHow := m.resolveRW(u, primary)
		rwDays := rwRemainingDays(rw, now)
		rwStatus := "missing"
		if rw != nil {
			rwStatus = "ok:" + rwHow
		} else {
			m.rep.addProblem("rw_missing", "user", fmt.Sprintf("%d", u.ID), "remnawave user not found")
		}

		price1m, priceNote := m.priceForUser(primary)
		balDays := 0
		if m.cfg.Balance.Enabled {
			balDays = BalanceDays(u.BalanceKopeks, price1m)
		}
		target := TargetDays(rwDays, balDays)
		expire := now.Add(time.Duration(target) * 24 * time.Hour)
		if rw != nil && rw.ExpireAt.After(expire) {
			expire = rw.ExpireAt
		}
		if balDays > 0 {
			balExpire := now.Add(time.Duration(balDays) * 24 * time.Hour)
			if balExpire.After(expire) {
				expire = balExpire
			}
		}

		var link *string
		if rw != nil && rw.SubscriptionUrl != "" {
			l := rw.SubscriptionUrl
			link = &l
		} else if primary != nil && primary.SubscriptionURL != nil {
			link = primary.SubscriptionURL
		}

		var tariffID *int64
		tariffSlug := ""
		if primary != nil && primary.TariffID != nil {
			if id, ok := m.tariffMap[*primary.TariffID]; ok && id > 0 {
				tariffID = &id
				tariffSlug = m.tariffSlug[*primary.TariffID]
			}
		}

		lang := u.Language
		if lang == "" {
			lang = "ru"
		}
		notes := priceNote

		if u.TelegramID != nil && *u.TelegramID != 0 {
			tg := *u.TelegramID
			m.bdgUserToTG[u.ID] = tg
			status, err := m.upsertTGCustomer(ctx, u, tg, &expire, link, tariffID, lang)
			if err != nil {
				m.rep.addProblem("customer_failed", "user", fmt.Sprintf("%d", u.ID), err.Error())
				status = "failed"
			} else {
				m.rep.summary.CustomersOK++
			}
			m.rep.addCustomer(customerReportRow{
				BedolagaUserID: u.ID, TelegramID: fmt.Sprintf("%d", tg), Email: strPtr(u.Email),
				Status: status, TariffSlug: tariffSlug, BalanceDays: balDays, RWDays: rwDays,
				TargetDays: target, RWStatus: rwStatus, Notes: notes,
			})
			continue
		}

		// web-only / email
		if !m.cfg.Customers.ImportCabinet {
			m.rep.addProblem("no_telegram_id", "user", fmt.Sprintf("%d", u.ID), "skipped (import_cabinet=false)")
			m.rep.addCustomer(customerReportRow{
				BedolagaUserID: u.ID, Email: strPtr(u.Email), Status: "skipped_no_tg",
				BalanceDays: balDays, RWDays: rwDays, TargetDays: target, RWStatus: rwStatus, Notes: notes,
			})
			continue
		}
		status, synTG, err := m.upsertCabinetCustomer(ctx, u, &expire, link, tariffID, lang)
		if err != nil {
			m.rep.addProblem("cabinet_failed", "user", fmt.Sprintf("%d", u.ID), err.Error())
			status = "failed"
		} else {
			m.bdgUserToTG[u.ID] = synTG
			m.rep.summary.CustomersOK++
			m.rep.summary.CustomersCabinet++
		}
		m.rep.addCustomer(customerReportRow{
			BedolagaUserID: u.ID, TelegramID: fmt.Sprintf("%d", synTG), Email: strPtr(u.Email),
			Status: status, TariffSlug: tariffSlug, BalanceDays: balDays, RWDays: rwDays,
			TargetDays: target, RWStatus: rwStatus, Notes: notes,
		})
	}
	return nil
}

func (m *Migrator) priceForUser(sub *bedolagaSub) (int, string) {
	if sub != nil && sub.TariffID != nil {
		if p, ok := m.tariffPrice1m[*sub.TariffID]; ok && p > 0 {
			return p, ""
		}
	}
	if m.cheapestPrice1m > 0 {
		return m.cheapestPrice1m, "balance_fallback_cheapest;"
	}
	return 0, "balance_no_price;"
}

func mergeExpire(existing *time.Time, proposed *time.Time) *time.Time {
	if proposed == nil {
		return existing
	}
	if existing == nil {
		return proposed
	}
	if existing.After(*proposed) {
		return existing
	}
	return proposed
}

func (m *Migrator) upsertTGCustomer(ctx context.Context, u bedolagaUser, tg int64, expire *time.Time, link *string, tariffID *int64, lang string) (string, error) {
	existing, err := m.custRepo.FindByTelegramId(ctx, tg)
	if err != nil {
		return "", err
	}
	if m.dryRun {
		if existing == nil {
			return "would_create", nil
		}
		return "would_update", nil
	}
	if existing == nil {
		created, err := m.custRepo.FindOrCreate(ctx, &database.Customer{TelegramID: tg, ExpireAt: expire, Language: lang})
		if err != nil {
			return "", err
		}
		existing = created
	}
	finalExpire := mergeExpire(existing.ExpireAt, expire)
	upd := map[string]interface{}{
		"expire_at": finalExpire,
		"language":  lang,
	}
	// Never wipe a good subscription_link with NULL.
	if link != nil && *link != "" {
		upd["subscription_link"] = *link
	}
	if tariffID != nil {
		upd["current_tariff_id"] = *tariffID
	}
	if u.Username != nil {
		upd["telegram_username"] = *u.Username
	}
	if m.cfg.Customers.SetLegalAccepted && existing.LegalAcceptedAt == nil {
		upd["legal_accepted_at"] = time.Now().UTC()
	}
	if err := m.custRepo.UpdateFields(ctx, existing.ID, upd); err != nil {
		return "", err
	}
	return "upserted", nil
}

func (m *Migrator) upsertCabinetCustomer(ctx context.Context, u bedolagaUser, expire *time.Time, link *string, tariffID *int64, lang string) (string, int64, error) {
	email := ""
	if u.Email != nil {
		email = strings.ToLower(strings.TrimSpace(*u.Email))
	}
	if email == "" && u.GoogleID == nil && u.YandexID == nil && u.VKID == nil {
		return "skipped_no_identity", 0, fmt.Errorf("no email/oauth identity")
	}

	if m.dryRun {
		syn := int64(0)
		return "would_create_cabinet", syn, nil
	}

	acc, err := m.findExistingCabinetAccount(ctx, u, email)
	if err != nil {
		return "", 0, err
	}
	passHash := ""
	passNote := ""
	if u.PasswordHash != nil && strings.HasPrefix(*u.PasswordHash, "$argon2id$") {
		passHash = *u.PasswordHash
	} else if u.PasswordHash != nil && *u.PasswordHash != "" {
		passNote = "cabinet_password_reset_required"
		m.rep.addProblem(passNote, "user", fmt.Sprintf("%d", u.ID), "password hash not argon2id — user must reset")
	}

	if acc == nil {
		acc, err = m.accRepo.Create(ctx, email, passHash, lang)
		if err != nil {
			return "", 0, err
		}
	}
	if u.EmailVerified {
		_ = m.accRepo.MarkEmailVerified(ctx, acc.ID)
	}

	m.ensureIdentities(ctx, acc.ID, u, email)

	synTG := utils.SyntheticTelegramID(acc.ID)
	cust, err := m.custRepo.CreateWebOnly(ctx, synTG, lang)
	if err != nil {
		return "", 0, err
	}
	if _, err := m.linkRepo.FindByAccountID(ctx, acc.ID); err != nil {
		if errors.Is(err, cabrepo.ErrNotFound) {
			_, _ = m.linkRepo.Create(ctx, acc.ID, cust.ID, cabrepo.LinkStatusLinked)
		}
	}

	finalExpire := mergeExpire(cust.ExpireAt, expire)
	upd := map[string]interface{}{
		"expire_at": finalExpire,
	}
	if link != nil && *link != "" {
		upd["subscription_link"] = *link
	}
	if tariffID != nil {
		upd["current_tariff_id"] = *tariffID
	}
	if m.cfg.Customers.SetLegalAccepted && cust.LegalAcceptedAt == nil {
		upd["legal_accepted_at"] = time.Now().UTC()
	}
	if err := m.custRepo.UpdateFields(ctx, cust.ID, upd); err != nil {
		return "", synTG, err
	}
	status := "cabinet_upserted"
	if passNote != "" {
		status += ";" + passNote
	}
	return status, synTG, nil
}

func (m *Migrator) findExistingCabinetAccount(ctx context.Context, u bedolagaUser, email string) (*cabrepo.Account, error) {
	if email != "" {
		acc, err := m.accRepo.FindByEmail(ctx, email)
		if err == nil {
			return acc, nil
		}
		if !errors.Is(err, cabrepo.ErrNotFound) {
			return nil, err
		}
	}
	tryProvider := func(provider, providerUserID string) (*cabrepo.Account, error) {
		if providerUserID == "" {
			return nil, nil
		}
		id, err := m.idRepo.FindByProvider(ctx, provider, providerUserID)
		if err != nil {
			if errors.Is(err, cabrepo.ErrNotFound) {
				return nil, nil
			}
			return nil, err
		}
		return m.accRepo.FindByID(ctx, id.AccountID)
	}
	if u.GoogleID != nil {
		if acc, err := tryProvider(cabrepo.ProviderGoogle, *u.GoogleID); acc != nil || err != nil {
			return acc, err
		}
	}
	if u.YandexID != nil {
		if acc, err := tryProvider(cabrepo.ProviderYandex, *u.YandexID); acc != nil || err != nil {
			return acc, err
		}
	}
	if u.VKID != nil {
		if acc, err := tryProvider(cabrepo.ProviderVK, fmt.Sprintf("%d", *u.VKID)); acc != nil || err != nil {
			return acc, err
		}
	}
	return nil, nil
}

func (m *Migrator) ensureIdentities(ctx context.Context, accountID int64, u bedolagaUser, email string) {
	if email != "" {
		_, _ = m.idRepo.Create(ctx, accountID, cabrepo.ProviderEmail, fmt.Sprintf("%d", accountID), email, nil)
	}
	if u.GoogleID != nil && *u.GoogleID != "" {
		_, _ = m.idRepo.Create(ctx, accountID, cabrepo.ProviderGoogle, *u.GoogleID, email, nil)
	}
	if u.YandexID != nil && *u.YandexID != "" {
		_, _ = m.idRepo.Create(ctx, accountID, cabrepo.ProviderYandex, *u.YandexID, email, nil)
	}
	if u.VKID != nil {
		_, _ = m.idRepo.Create(ctx, accountID, cabrepo.ProviderVK, fmt.Sprintf("%d", *u.VKID), email, nil)
	}
}

// --- balance / RW extend ---

func (m *Migrator) stepBalance(ctx context.Context) error {
	if !m.cfg.Balance.Enabled {
		slog.Info("balance step skipped (disabled)")
		return nil
	}
	// Recompute from source + apply RW extend. Customers step already wrote expire_at.
	users, err := m.src.LoadUsers(ctx)
	if err != nil {
		return err
	}
	subs, err := m.src.LoadSubscriptions(ctx)
	if err != nil {
		return err
	}
	subsByUser := map[int][]bedolagaSub{}
	for _, s := range subs {
		subsByUser[s.UserID] = append(subsByUser[s.UserID], s)
	}
	_ = m.ensureTariffMaps(ctx)

	now := time.Now().UTC()
	for _, u := range users {
		if m.cfg.Customers.SkipDeleted && u.Status == "deleted" {
			continue
		}
		if m.cfg.Customers.SkipBlocked && u.Status == "blocked" {
			continue
		}
		primary, _ := pickPrimarySub(subsByUser[u.ID])
		rw, _ := m.resolveRW(u, primary)
		if rw == nil {
			continue
		}
		rwDays := rwRemainingDays(rw, now)
		price1m, _ := m.priceForUser(primary)
		balDays := BalanceDays(u.BalanceKopeks, price1m)
		target := TargetDays(rwDays, balDays)
		if target <= rwDays {
			continue
		}
		// Prefer absolute max(rw.ExpireAt, now+balanceDays) so we never shorten.
		balExpire := now.Add(time.Duration(balDays) * 24 * time.Hour)
		newExpire := rw.ExpireAt
		if balDays > 0 && balExpire.After(newExpire) {
			newExpire = balExpire
		}
		if !newExpire.After(rw.ExpireAt) {
			continue
		}
		extendBy := target - rwDays
		if !m.cfg.Balance.ApplyToRemnawave {
			m.rep.addProblem("balance_rw_skipped", "user", fmt.Sprintf("%d", u.ID),
				fmt.Sprintf("would extend RW expire (apply_to_remnawave=false), ~%d days", extendBy))
			continue
		}
		if m.dryRun {
			m.rep.addProblem("balance_would_extend", "user", fmt.Sprintf("%d", u.ID),
				fmt.Sprintf("dry-run: extend RW expire to %s", newExpire.UTC().Format(time.RFC3339)))
			continue
		}
		// ExpireAt only — never create users, never touch squads/status.
		uid := rw.ID
		req := &remnawave.UpdateUserRequest{
			ID:       &uid,
			ExpireAt: &newExpire,
		}
		if _, err := m.rw.PatchUser(ctx, req); err != nil {
			m.rep.addProblem("balance_extend_failed", "user", fmt.Sprintf("%d", u.ID), err.Error())
			continue
		}
		m.rep.summary.BalanceApplied++
	}
	return nil
}

// --- referrals ---

func (m *Migrator) stepReferrals(ctx context.Context) error {
	if !m.cfg.Referrals.ImportGraph {
		return nil
	}
	users, err := m.src.LoadUsers(ctx)
	if err != nil {
		return err
	}
	// Ensure telegram map if customers step not run in this process
	if len(m.bdgUserToTG) == 0 {
		for _, u := range users {
			if u.TelegramID != nil {
				m.bdgUserToTG[u.ID] = *u.TelegramID
			}
		}
	}
	for _, u := range users {
		if u.ReferredByID == nil {
			continue
		}
		refereeTG, ok1 := m.bdgUserToTG[u.ID]
		referrerTG, ok2 := m.bdgUserToTG[*u.ReferredByID]
		if !ok1 || !ok2 || refereeTG == 0 || referrerTG == 0 {
			m.rep.addProblem("referral_partial", "user", fmt.Sprintf("%d", u.ID), "missing telegram mapping for referrer or referee")
			continue
		}
		if m.dryRun {
			m.rep.summary.ReferralsOK++
			continue
		}
		if existing, err := m.refRepo.FindByReferee(ctx, refereeTG); err == nil && existing != nil {
			m.rep.summary.ReferralsOK++
			continue
		}
		if _, err := m.refRepo.Create(ctx, referrerTG, refereeTG); err != nil {
			m.rep.addProblem("referral_failed", "user", fmt.Sprintf("%d", u.ID), err.Error())
			continue
		}
		m.rep.summary.ReferralsOK++
	}
	return nil
}

func strPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
