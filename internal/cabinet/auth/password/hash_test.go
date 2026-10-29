package password

import (
	"testing"
	"time"
)

// testParams — быстрый Argon2 для unit-тестов (не для прода).
func testParams() Params {
	return Params{
		Memory:      8,
		Iterations:  1,
		Parallelism: 1,
		SaltLen:     8,
		KeyLen:      16,
	}
}

func TestHashPassword_ComparePasswordAndHash_roundTrip(t *testing.T) {
	p := testParams()
	plain := "correct-horse-battery-staple-9"
	hash, err := HashPassword(plain, p)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, needsRehash, err := ComparePasswordAndHash(plain, hash, DefaultParams())
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if !ok {
		t.Fatal("expected password match")
	}
	if !needsRehash {
		t.Fatal("test params weaker than DefaultParams — expected needsRehash true")
	}
	ok, _, err = ComparePasswordAndHash("wrong", hash, p)
	if err != nil {
		t.Fatalf("Compare wrong: %v", err)
	}
	if ok {
		t.Fatal("expected mismatch for wrong password")
	}
}

func TestComparePasswordAndHash_invalidFormat(t *testing.T) {
	_, _, err := ComparePasswordAndHash("x", "not-a-phc-string", DefaultParams())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSlotCount(t *testing.T) {
	for procs, want := range map[int]int{1: 2, 2: 2, 4: 2, 6: 3, 8: 4, 64: 4} {
		if got := slotCount(procs); got != want {
			t.Errorf("slotCount(%d) = %d, want %d", procs, got, want)
		}
	}
}

// Пока все слоты заняты, новое вычисление ждёт, а не запускается сверх лимита.
func TestHashPassword_waitsForFreeSlot(t *testing.T) {
	for i := 0; i < cap(slots); i++ {
		slots <- struct{}{}
	}
	done := make(chan struct{})
	go func() {
		_, _ = HashPassword("pw-waits-for-slot", testParams())
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("hash computed while all slots were busy")
	case <-time.After(50 * time.Millisecond):
	}
	for i := 0; i < cap(slots); i++ {
		<-slots
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("hash did not finish after slots were released")
	}
}
