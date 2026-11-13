// Package mail — отправка писем кабинета через SMTP (go-mail).
//
// Поддерживается любой универсальный SMTP, параметры читаются из
// cabinet/config (CABINET_SMTP_*). Если SMTPEnabled()==false, Sender работает
// в dry-run режиме: логирует письмо и возвращает nil — удобно для локальной
// разработки и при CABINET_ENABLED=true без настроенного SMTP.
package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	gomail "github.com/wneessen/go-mail"
)

// Config — минимальный набор параметров для Sender.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string // "Cabinet <no-reply@example.com>" либо "no-reply@example.com"
	UseTLS   bool   // true — SSL/TLS (implicit, порт 465); false — STARTTLS (порт 587)
	DryRun   bool   // если true — вместо отправки логируем
}

// ErrDisabled возвращается, когда код пытается отправить письмо, но SMTP
// выключен и DryRun не установлен. Auth-сервис должен на это реагировать
// graceful'ом и показывать пользователю «повторите позже».
var ErrDisabled = errors.New("mail: SMTP is not configured")

// Sender — обёртка над go-mail клиентом. Потокобезопасный: Send создаёт новый
// клиент под каждую отправку, что просто и не требует pool'а для MVP.
type Sender struct {
	cfg    Config
	health health
}

// NewSender — конструктор.
func NewSender(cfg Config) *Sender {
	return &Sender{cfg: cfg}
}

// OnHealthChange подписывает fn на смену состояния почты: «перестала
// отправляться» и «снова отправляется». Вызывать до первой отправки.
func (s *Sender) OnHealthChange(fn HealthFunc) {
	s.health.notify = fn
}

// Send отправляет одно письмо.
//
// toEmail — адрес получателя; subject — тема; htmlBody — готовый HTML.
// Plain-text часть генерируется автоматически (go-mail умеет через WithBody,
// но для простоты и читабельности шлём только HTML + явный AltBody).
func (s *Sender) Send(ctx context.Context, toEmail, subject, htmlBody, plainBody string) error {
	if s.cfg.DryRun {
		slog.Info("mail: dry-run",
			"to", toEmail,
			"subject", subject,
			"bytes", len(htmlBody),
		)
		return nil
	}
	if s.cfg.Host == "" || s.cfg.From == "" {
		return ErrDisabled
	}

	msg := gomail.NewMsg()
	if err := msg.From(s.cfg.From); err != nil {
		return fmt.Errorf("mail: from: %w", err)
	}
	if err := msg.To(toEmail); err != nil {
		return fmt.Errorf("mail: to: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(gomail.TypeTextHTML, htmlBody)
	if plainBody != "" {
		msg.AddAlternativeString(gomail.TypeTextPlain, plainBody)
	}

	opts := []gomail.Option{
		gomail.WithPort(s.cfg.Port),
		gomail.WithTimeout(15 * time.Second),
	}
	if s.cfg.UseTLS {
		opts = append(opts, gomail.WithSSLPort(false))
	} else {
		opts = append(opts, gomail.WithTLSPortPolicy(gomail.TLSMandatory))
	}
	if s.cfg.Username != "" {
		opts = append(opts,
			// У разных SMTP-провайдеров (в т.ч. часть shared-hosting) набор
			// AUTH-механизмов отличается: где-то нет PLAIN, но есть LOGIN.
			// AUTODISCOVER выбирает лучший поддерживаемый сервером вариант.
			gomail.WithSMTPAuth(gomail.SMTPAuthAutoDiscover),
			gomail.WithUsername(s.cfg.Username),
			gomail.WithPassword(s.cfg.Password),
		)
	}

	client, err := gomail.NewClient(s.cfg.Host, opts...)
	if err != nil {
		return fmt.Errorf("mail: client: %w", err)
	}
	err = client.DialAndSendWithContext(ctx, msg)
	s.health.report(err)
	if err != nil {
		return fmt.Errorf("mail: send: %w", err)
	}
	return nil
}
