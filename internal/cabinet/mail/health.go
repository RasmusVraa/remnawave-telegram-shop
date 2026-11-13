package mail

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	gomail "github.com/wneessen/go-mail"
)

const (
	// alertRepeat — как часто напоминать админу, пока почта не отправляется.
	alertRepeat = 6 * time.Hour
	// alertQuiet — пауза после прошлого оповещения, раньше которой о новом
	// падении не пишем: если SMTP «моргает», админ получит одно «лежит»
	// и одно «поднялась», а не пару сообщений на каждое письмо.
	alertQuiet = 30 * time.Minute
)

// HealthFunc получает смену состояния почты: down=true — письма не уходят
// (reason — текст ошибки), down=false — снова уходят. Вызывается синхронно из
// Send, поэтому долгую работу (отправку в Telegram) обработчик уносит в фон сам.
type HealthFunc func(down bool, reason string)

// health следит за результатом отправок и решает, когда звать HealthFunc.
//
// Пользователь об ошибке почты не узнаёт: «сброс пароля» отвечает «письмо
// отправлено» в любом случае, чтобы по ответу нельзя было проверить, есть ли
// такой email. Поэтому без оповещения админа почта может лежать неделями.
type health struct {
	mu        sync.Mutex
	notify    HealthFunc
	announced bool // админу сказали «лежит» и ещё не сказали «поднялась»
	lastAlert time.Time
	now       func() time.Time
}

func (h *health) report(err error) {
	if h.notify == nil {
		return
	}
	// Отменённый запрос (клиент закрыл вкладку) ничего не говорит о почте.
	if err != nil && (recipientFault(err) || errors.Is(err, context.Canceled)) {
		return
	}

	now := time.Now
	if h.now != nil {
		now = h.now
	}

	h.mu.Lock()
	t := now()
	fire := false
	switch {
	case err == nil:
		fire = h.announced
		h.announced = false
	case h.announced:
		fire = t.Sub(h.lastAlert) >= alertRepeat
	default:
		fire = h.lastAlert.IsZero() || t.Sub(h.lastAlert) >= alertQuiet
		h.announced = fire
	}
	if fire {
		h.lastAlert = t
	}
	h.mu.Unlock()

	if !fire {
		return
	}
	if err != nil {
		slog.Error("mail: SMTP не отправляет письма, админ оповещён", "error", err)
		h.notify(true, err.Error())
		return
	}
	slog.Info("mail: SMTP снова отправляет письма, админ оповещён")
	h.notify(false, "")
}

// recipientFault — сервер отказал конкретному адресу (опечатка, несуществующий
// ящик). Это беда одного письма, а не почты целиком: будить админа не за что.
func recipientFault(err error) bool {
	var se *gomail.SendError
	return errors.As(err, &se) && se.Reason == gomail.ErrSMTPRcptTo && !se.IsTemp()
}
