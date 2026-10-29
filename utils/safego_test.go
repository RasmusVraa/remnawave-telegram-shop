package utils

import (
	"testing"
	"time"
)

// Паника в фоновой задаче не роняет процесс, отложенные вызовы задачи
// выполняются (на них держатся флаги «идёт рассылка» и т. п.).
func TestGo_RecoversPanicAndRunsDefers(t *testing.T) {
	released := make(chan struct{})
	Go("test", func() {
		defer close(released)
		panic("boom")
	})
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("deferred call in panicking task did not run")
	}
}
