package ratelimit

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAllow_countPerKey(t *testing.T) {
	l := New(Rule{Count: 2, Interval: time.Hour})
	for i := range 2 {
		if !l.Allow("a") {
			t.Fatalf("request %d must pass", i+1)
		}
	}
	if l.Allow("a") {
		t.Fatal("third request within interval must be rejected")
	}
	if !l.Allow("b") {
		t.Fatal("other key has its own bucket")
	}
}

// Длинный ключ хранится хэшем и не делит бакет с другим длинным ключом.
func TestAllow_longKeys(t *testing.T) {
	l := New(Rule{Count: 1, Interval: time.Hour})
	a := strings.Repeat("x", 64<<10)
	b := a[:len(a)-1] + "y"
	if !l.Allow(a) || !l.Allow(b) {
		t.Fatal("distinct long keys must get distinct buckets")
	}
	if l.Allow(a) {
		t.Fatal("same long key must reuse its bucket")
	}
}

// Перебор ключами упирается в потолок: новые ключи получают отказ, а не
// раздувают map; простоявшие бакеты освобождают место.
func TestAllow_bucketCap(t *testing.T) {
	l := New(Rule{Count: 5, Interval: time.Minute})
	l.max = 3
	for i := range 3 {
		if !l.Allow("k" + strconv.Itoa(i)) {
			t.Fatalf("key %d must pass", i)
		}
	}
	if l.Allow("overflow") {
		t.Fatal("new key over the cap must be rejected")
	}
	if len(l.buckets) != 3 {
		t.Fatalf("buckets = %d, want 3", len(l.buckets))
	}

	// Все бакеты простояли дольше lifetime — следующий ключ их вычищает.
	for _, e := range l.buckets {
		e.lastSeen = time.Now().Add(-2 * l.lifetime)
	}
	if !l.Allow("fresh") {
		t.Fatal("stale buckets must be collected to make room")
	}
	if len(l.buckets) != 1 {
		t.Fatalf("buckets after gc = %d, want 1", len(l.buckets))
	}
}
