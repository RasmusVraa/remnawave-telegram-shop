package database

import (
	"reflect"
	"testing"
)

// Скан позиционный: i-я колонка purchaseColumns должна попадать в поле с тем
// же db-тегом, и ни одно поле Purchase не должно остаться без колонки.
func TestPurchaseColumnsMatchScanArgs(t *testing.T) {
	var p Purchase
	args := purchaseScanArgs(&p)
	if len(args) != len(purchaseColumns) {
		t.Fatalf("scan args %d, columns %d", len(args), len(purchaseColumns))
	}

	v := reflect.ValueOf(&p).Elem()
	byTag := make(map[string]uintptr, v.NumField())
	for i := 0; i < v.NumField(); i++ {
		if tag := v.Type().Field(i).Tag.Get("db"); tag != "" {
			byTag[tag] = v.Field(i).Addr().Pointer()
		}
	}
	if len(byTag) != len(purchaseColumns) {
		t.Fatalf("Purchase has %d db fields, purchaseColumns has %d", len(byTag), len(purchaseColumns))
	}
	for i, col := range purchaseColumns {
		addr, ok := byTag[col]
		if !ok {
			t.Fatalf("column %q has no Purchase field", col)
		}
		if got := reflect.ValueOf(args[i]).Pointer(); got != addr {
			t.Errorf("column %d %q is scanned into the wrong field", i, col)
		}
	}
}
