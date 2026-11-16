package handlers

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

func TestParsePromoListParams_defaults(t *testing.T) {
	p, msg := parsePromoListParams(url.Values{})
	if msg != "" {
		t.Fatalf("defaults rejected: %s", msg)
	}
	want := promoListParams{Page: 1, Limit: 20, Filter: database.PromoListFilter{
		Status: database.PromoStatusAll, Type: "", Query: "", Sort: database.PromoSortNew,
	}}
	if p != want {
		t.Fatalf("got %+v, want %+v", p, want)
	}
}

func TestParsePromoListParams_values(t *testing.T) {
	q := url.Values{
		"page": {"3"}, "limit": {"50"}, "status": {"ended"}, "type": {"discount"},
		"sort": {"expiring"}, "q": {"  mor  "},
	}
	p, msg := parsePromoListParams(q)
	if msg != "" {
		t.Fatalf("rejected: %s", msg)
	}
	want := promoListParams{Page: 3, Limit: 50, Filter: database.PromoListFilter{
		Status: "ended", Type: "discount", Query: "mor", Sort: "expiring",
	}}
	if p != want {
		t.Fatalf("got %+v, want %+v", p, want)
	}

	for _, status := range []string{"all", "active", "ended", "off"} {
		if _, msg := parsePromoListParams(url.Values{"status": {status}}); msg != "" {
			t.Errorf("status %q rejected: %s", status, msg)
		}
	}
	for _, typ := range []string{"", "all", "subscription_days", "trial", "extra_hwid", "discount"} {
		p, msg := parsePromoListParams(url.Values{"type": {typ}})
		if msg != "" {
			t.Errorf("type %q rejected: %s", typ, msg)
		}
		if typ == "all" && p.Filter.Type != "" {
			t.Errorf("type all should mean no filter, got %q", p.Filter.Type)
		}
	}
	for _, sort := range []string{"", "new", "uses", "expiring"} {
		if _, msg := parsePromoListParams(url.Values{"sort": {sort}}); msg != "" {
			t.Errorf("sort %q rejected: %s", sort, msg)
		}
	}
}

func TestParsePromoListParams_pageLimitClamp(t *testing.T) {
	for _, tc := range []struct{ page, limit string }{{"0", "0"}, {"-1", "101"}, {"abc", "x"}} {
		p, msg := parsePromoListParams(url.Values{"page": {tc.page}, "limit": {tc.limit}})
		if msg != "" || p.Page != 1 || p.Limit != 20 {
			t.Errorf("page=%s limit=%s: got page %d limit %d msg %q", tc.page, tc.limit, p.Page, p.Limit, msg)
		}
	}
}

func TestParsePromoListParams_invalid(t *testing.T) {
	cases := map[string]url.Values{
		"invalid status": {"status": {"expired"}},
		"invalid type":   {"type": {"bonus"}},
		"invalid sort":   {"sort": {"old"}},
		"invalid dir":    {"sort": {"code"}, "dir": {"up"}},
	}
	for want, q := range cases {
		if _, msg := parsePromoListParams(q); msg != want {
			t.Errorf("%v: got %q, want %q", q, msg, want)
		}
	}
}

func TestParsePromoListParams_queryTruncatedByRunes(t *testing.T) {
	long := strings.Repeat("я", 70)
	p, msg := parsePromoListParams(url.Values{"q": {long}})
	if msg != "" {
		t.Fatalf("rejected: %s", msg)
	}
	if got := []rune(p.Filter.Query); len(got) != promoSearchMaxRunes {
		t.Fatalf("query has %d runes, want %d", len(got), promoSearchMaxRunes)
	}
	// Спецсимволы LIKE доходят до репозитория как есть — экранирует он.
	p, _ = parsePromoListParams(url.Values{"q": {"50%_off"}})
	if p.Filter.Query != "50%_off" {
		t.Fatalf("query = %q", p.Filter.Query)
	}
}

func TestValidateCreatePromo_noteAndTariff(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	i := func(v int) *int { return &v }
	s := func(v string) *string { return &v }
	tid := int64(7)

	ok := []createPromoReq{
		{Code: "DAYS", Type: "subscription_days", SubscriptionDays: i(7), TariffID: &tid},
		{Code: "SALE", Type: "discount", DiscountPercent: i(10)},
		{Code: "NOTE", Type: "trial", TrialDays: i(3), Note: s(strings.Repeat("я", 200))},
		{Code: "NOTE2", Type: "trial", TrialDays: i(3), Note: s("   ")},
		{Code: "NOTE3", Type: "trial", TrialDays: i(3), Note: s("  " + strings.Repeat("a", 200) + "  ")},
	}
	for _, r := range ok {
		if msg := validateCreatePromo(&r, now); msg != "" {
			t.Errorf("%s rejected: %s", r.Code, msg)
		}
	}

	bad := []struct {
		req  createPromoReq
		want string
	}{
		{createPromoReq{Code: "SALE", Type: "discount", DiscountPercent: i(10), TariffID: &tid}, "tariff only for subscription days"},
		{createPromoReq{Code: "TRY", Type: "trial", TrialDays: i(3), TariffID: &tid}, "tariff only for subscription days"},
		{createPromoReq{Code: "DEV", Type: "extra_hwid", ExtraHwidDelta: i(1), TariffID: &tid}, "tariff only for subscription days"},
		{createPromoReq{Code: "NOTE", Type: "trial", TrialDays: i(3), Note: s(strings.Repeat("я", 201))}, "note too long"},
	}
	for _, tc := range bad {
		if got := validateCreatePromo(&tc.req, now); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.req.Code, got, tc.want)
		}
	}
}

func TestNormalizePromoNote(t *testing.T) {
	if n, ok := normalizePromoNote("  блогер X  "); !ok || n == nil || *n != "блогер X" {
		t.Fatalf("trim: %v %v", n, ok)
	}
	if n, ok := normalizePromoNote(" \t "); !ok || n != nil {
		t.Fatalf("empty must be NULL: %v %v", n, ok)
	}
	if _, ok := normalizePromoNote(strings.Repeat("ж", 201)); ok {
		t.Fatal("201 runes accepted")
	}
}

func patchRaw(t *testing.T, body string) map[string]json.RawMessage {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

var promoPatchAllowedForTest = map[string]bool{"max_uses": true, "note": true, "tariff_id": true}

func TestParsePromoPatchFields_maxUsesZeroIsNull(t *testing.T) {
	fields, err := parsePromoPatchFields(patchRaw(t, `{"max_uses": 0}`), promoPatchAllowedForTest)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := fields["max_uses"]; !ok || v != nil {
		t.Fatalf("max_uses 0 → %v (present %v), want nil", v, ok)
	}
	if err := validatePromoUpdateFields("trial", fields); err != nil {
		t.Fatalf("validate: %v", err)
	}

	fields, err = parsePromoPatchFields(patchRaw(t, `{"max_uses": 5}`), promoPatchAllowedForTest)
	if err != nil || fields["max_uses"] != float64(5) {
		t.Fatalf("max_uses 5 → %v, %v", fields["max_uses"], err)
	}
	fields, _ = parsePromoPatchFields(patchRaw(t, `{"max_uses": -1}`), promoPatchAllowedForTest)
	if err := validatePromoUpdateFields("trial", fields); err == nil || err.Error() != "invalid max_uses" {
		t.Fatalf("max_uses -1: %v", err)
	}
}

func TestParsePromoPatchFields_note(t *testing.T) {
	cases := []struct {
		body    string
		want    interface{}
		wantErr string
	}{
		{`{"note": "  канал Y "}`, "канал Y", ""},
		{`{"note": "   "}`, nil, ""},
		{`{"note": null}`, nil, ""},
		{`{"note": 5}`, nil, "invalid note"},
		{`{"note": "` + strings.Repeat("я", 201) + `"}`, nil, "note too long"},
	}
	for _, tc := range cases {
		fields, err := parsePromoPatchFields(patchRaw(t, tc.body), promoPatchAllowedForTest)
		if tc.wantErr != "" {
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("%.40s: err %v, want %q", tc.body, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", tc.body, err)
			continue
		}
		if v, ok := fields["note"]; !ok || v != tc.want {
			t.Errorf("%s: note = %#v (present %v), want %#v", tc.body, v, ok, tc.want)
		}
	}
}

func TestPromoPatch_tariffOnlyForSubscriptionDays(t *testing.T) {
	fields, err := parsePromoPatchFields(patchRaw(t, `{"tariff_id": 12}`), promoPatchAllowedForTest)
	if err != nil {
		t.Fatal(err)
	}
	if fields["tariff_id"] != int64(12) {
		t.Fatalf("tariff_id = %#v, want int64(12)", fields["tariff_id"])
	}
	if err := validatePromoUpdateFields("subscription_days", fields); err != nil {
		t.Fatalf("subscription_days: %v", err)
	}
	for _, typ := range []string{"discount", "trial", "extra_hwid"} {
		if err := validatePromoUpdateFields(typ, fields); err == nil || err.Error() != "tariff only for subscription days" {
			t.Errorf("%s: err %v", typ, err)
		}
	}

	// Снять тариф (null) можно у любого типа — так чистятся старые скидки с тарифом.
	cleared, err := parsePromoPatchFields(patchRaw(t, `{"tariff_id": null}`), promoPatchAllowedForTest)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := cleared["tariff_id"]; !ok || v != nil {
		t.Fatalf("tariff_id null → %#v", v)
	}
	if err := validatePromoUpdateFields("discount", cleared); err != nil {
		t.Fatalf("discount clear tariff: %v", err)
	}

	if _, err := parsePromoPatchFields(patchRaw(t, `{"tariff_id": "x"}`), promoPatchAllowedForTest); err == nil || err.Error() != "invalid tariff_id" {
		t.Fatalf("tariff_id string: %v", err)
	}
}
