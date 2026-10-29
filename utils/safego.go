package utils

import (
	"fmt"
	"log/slog"
	"runtime/debug"
)

// Go запускает fn в отдельной горутине и не даёт её панике уронить процесс.
//
// Паника в горутине, которую никто не перехватил, завершает всю программу —
// вместе с ботом, кабинетом и приёмом вебхуков оплат. Для фоновой задачи
// (рассылка, пересчёт, уведомление) это несоразмерно: задача пишется в лог со
// стеком и прекращается, остальное продолжает работать.
func Go(name string, fn func()) {
	go func() {
		defer LogPanic(name)
		fn()
	}()
}

// LogPanic перехватывает панику и пишет её в лог со стеком. Вызывается только
// через defer, напрямую: recover работает лишь в самой отложенной функции.
func LogPanic(name string) {
	if r := recover(); r != nil {
		slog.Error("recovered panic", "task", name, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
	}
}
