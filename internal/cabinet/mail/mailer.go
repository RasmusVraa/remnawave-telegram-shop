package mail

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
)

//go:embed templates/*.html
var templatesFS embed.FS

// Mailer — высокоуровневый API для отправки писем кабинета (verify, reset,
// duplicate register). Знает о шаблонах и о выборе языка — вызывающий код
// передаёт только модель (адрес, код или URL, TTL).
type Mailer struct {
	sender *Sender
	tpls   *template.Template
}

// NewMailer компилит все шаблоны на старте. Если шаблоны повреждены, функция
// panic'нет — это намеренно, такой баг ловим в CI/тестах, не в рантайме.
func NewMailer(sender *Sender) *Mailer {
	tpls := template.Must(template.ParseFS(templatesFS, "templates/*.html"))
	return &Mailer{sender: sender, tpls: tpls}
}

// VerifyEmailData — контекст шаблона email_verify_*.
type VerifyEmailData struct {
	Code     string // 6-значный код; ввод на странице подтверждения кабинета
	TTLHuman string // "24 часа" / "24 hours"
}

// SendVerifyEmail отправляет письмо подтверждения email.
func (m *Mailer) SendVerifyEmail(ctx context.Context, toEmail, language string, data VerifyEmailData) error {
	tplName := pickTemplate("email_verify", language)
	subject := subjectFor("email_verify", language)
	return m.render(ctx, tplName, subject, toEmail, data)
}

// DuplicateRegisterData — контекст шаблона duplicate_register_*.
type DuplicateRegisterData struct {
	LoginURL string
}

// SendDuplicateRegister отправляет письмо «кто-то пытался зарегаться на ваш email».
// Часть защиты от account-enumeration: сервис отвечает пользователю «успех»,
// реальный аккаунт получает это письмо.
func (m *Mailer) SendDuplicateRegister(ctx context.Context, toEmail, language string, data DuplicateRegisterData) error {
	tplName := pickTemplate("duplicate_register", language)
	subject := subjectFor("duplicate_register", language)
	return m.render(ctx, tplName, subject, toEmail, data)
}

// PasswordResetData — контекст шаблона password_reset_*.
type PasswordResetData struct {
	ResetURL string
	TTLHuman string
}

// SendPasswordReset отправляет письмо со ссылкой на сброс пароля.
func (m *Mailer) SendPasswordReset(ctx context.Context, toEmail, language string, data PasswordResetData) error {
	tplName := pickTemplate("password_reset", language)
	subject := subjectFor("password_reset", language)
	return m.render(ctx, tplName, subject, toEmail, data)
}

// GoogleLinkConfirmData — контекст шаблона google_link_confirm_*.
type GoogleLinkConfirmData struct {
	ConfirmURL   string
	ProviderName string // «Google», «Яндекс ID», «VK ID»
}

// TelegramLinkedData — контекст шаблона telegram_linked_*.
type TelegramLinkedData struct {
	MergeResult string // "linked" | "merged" | "noop"
}

type EmailMergeCodeData struct {
	Code    string
	TTLHuman string
}

// SendTelegramLinked уведомляет пользователя об успешной привязке / слиянии
// Telegram-аккаунта с веб-кабинетом.
func (m *Mailer) SendTelegramLinked(ctx context.Context, toEmail, language, mergeResult string) error {
	tplName := pickTemplate("telegram_linked", language)
	subject := subjectFor("telegram_linked", language)
	return m.render(ctx, tplName, subject, toEmail, TelegramLinkedData{MergeResult: mergeResult})
}

// SendOAuthLinkConfirm отправляет письмо подтверждения привязки внешнего входа
// (Google, Яндекс, VK). Вызывается, когда email провайдера совпадает с уже
// существующим cabinet_account: привязать можно только по ссылке из этого письма.
func (m *Mailer) SendOAuthLinkConfirm(ctx context.Context, toEmail, language, provider, confirmURL string) error {
	tplName := pickTemplate("google_link_confirm", language)
	subject := subjectFor("google_link_confirm", language)
	return m.render(ctx, tplName, subject, toEmail, GoogleLinkConfirmData{
		ConfirmURL:   confirmURL,
		ProviderName: oauthProviderName(provider, language),
	})
}

// oauthProviderName — название провайдера для текста письма.
func oauthProviderName(provider, language string) string {
	switch provider {
	case "yandex":
		if language == "en" {
			return "Yandex ID"
		}
		return "Яндекс ID"
	case "vk":
		return "VK ID"
	default:
		return "Google"
	}
}

// SendEmailMergeCode отправляет 6-значный код подтверждения merge email-аккаунта,
// когда peer-аккаунт не имеет пароля (OAuth-only).
func (m *Mailer) SendEmailMergeCode(ctx context.Context, toEmail, language, code, ttlHuman string) error {
	tplName := pickTemplate("email_merge_code", language)
	subject := subjectFor("email_merge_code", language)
	return m.render(ctx, tplName, subject, toEmail, EmailMergeCodeData{Code: code, TTLHuman: ttlHuman})
}

func (m *Mailer) render(ctx context.Context, tplName, subject, toEmail string, data any) error {
	var buf bytes.Buffer
	if err := m.tpls.ExecuteTemplate(&buf, tplName, data); err != nil {
		return fmt.Errorf("mail: render %s: %w", tplName, err)
	}
	return m.sender.Send(ctx, toEmail, subject, buf.String(), "")
}

// pickTemplate возвращает имя файла-шаблона для заданной категории + языка.
// Если язык не поддержан — fallback на ru (дефолт проекта).
func pickTemplate(base, language string) string {
	switch language {
	case "en":
		return base + "_en.html"
	default:
		return base + "_ru.html"
	}
}

// subjectFor возвращает локализованный subject. Вынесено в код (а не в
// шаблоны), чтобы subject нельзя было случайно сломать HTML-форматированием.
func subjectFor(kind, language string) string {
	switch kind {
	case "email_verify":
		if language == "en" {
			return "Verify your email"
		}
		return "Подтверждение email"
	case "duplicate_register":
		if language == "en" {
			return "Someone tried to register with your email"
		}
		return "Попытка регистрации на ваш email"
	case "password_reset":
		if language == "en" {
			return "Password reset"
		}
		return "Сброс пароля"
	case "google_link_confirm":
		if language == "en" {
			return "Confirm sign-in method link"
		}
		return "Подтверждение привязки способа входа"
	case "telegram_linked":
		if language == "en" {
			return "Sign-in method updated"
		}
		return "Способ входа обновлён"
	case "email_merge_code":
		if language == "en" {
			return "Merge confirmation code"
		}
		return "Код подтверждения объединения аккаунтов"
	default:
		return "Cabinet notification"
	}
}
