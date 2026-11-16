package database

import (
	"strings"
	"testing"
)

func TestEscapeLikePattern(t *testing.T) {
	cases := map[string]string{
		"MORNING":  "MORNING",
		"50%":      `50\%`,
		"a_b":      `a\_b`,
		`back\sl`:  `back\\sl`,
		`%_\`:      `\%\_\\`,
		"блогер Х": "блогер Х",
	}
	for in, want := range cases {
		if got := escapeLikePattern(in); got != want {
			t.Errorf("escapeLikePattern(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPromoListBaseWhere(t *testing.T) {
	where, args := promoListBaseWhere("")
	if where != notTechnicalPromo || len(args) != 0 {
		t.Fatalf("no query: %q %v", where, args)
	}
	where, args = promoListBaseWhere("50%")
	if !strings.Contains(where, `code ILIKE $1 ESCAPE '\'`) || !strings.Contains(where, `note ILIKE $1 ESCAPE '\'`) {
		t.Fatalf("query where: %q", where)
	}
	if len(args) != 1 || args[0] != `%50\%%` {
		t.Fatalf("query args: %v", args)
	}
}

func TestPromoStatusCondAndSort(t *testing.T) {
	if promoStatusCond(PromoStatusAll) != "TRUE" || promoStatusCond("") != "TRUE" {
		t.Fatal("all must not filter")
	}
	// «Есть лимит» — только max_uses > 0: ноль считается безлимитом.
	if !strings.Contains(promoStatusCond(PromoStatusActive), "max_uses > 0") {
		t.Fatal("active must treat max_uses = 0 as unlimited")
	}
	if promoSortOrder("", "") != "id DESC" || promoSortOrder(PromoSortNew, "") != "id DESC" {
		t.Fatal("default sort")
	}
	if promoSortOrder(PromoSortUses, "") != "uses_count DESC, id DESC" {
		t.Fatal("uses sort")
	}
	if !strings.HasPrefix(promoSortOrder(PromoSortExpiring, ""), "CASE WHEN valid_until > now() THEN 0 WHEN valid_until IS NULL THEN 1 ELSE 2 END") {
		t.Fatal("expiring sort")
	}
	// Направление из шапки таблицы.
	if promoSortOrder(PromoSortNew, PromoSortAsc) != "id ASC" {
		t.Fatal("new asc")
	}
	if promoSortOrder(PromoSortUses, PromoSortAsc) != "uses_count ASC, id DESC" {
		t.Fatal("uses asc")
	}
	if promoSortOrder(PromoSortCode, "") != "code ASC, id DESC" || promoSortOrder(PromoSortCode, PromoSortDesc) != "code DESC, id DESC" {
		t.Fatal("code sort")
	}
	if !strings.HasPrefix(promoSortOrder(PromoSortExpiring, PromoSortDesc), "CASE WHEN valid_until IS NULL THEN 1 ELSE 0 END, valid_until DESC") {
		t.Fatal("expiring desc: дальние сроки первыми, бессрочные в конце")
	}
}
