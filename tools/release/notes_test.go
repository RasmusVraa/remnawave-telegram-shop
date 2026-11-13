package main

import (
	"strings"
	"testing"
)

const sampleNotes = `# Релиз v4.12.1

## Вариант для GitHub Releases

## ✨ Улучшения

- Поддержка панели **Remnawave 2.8.x**.

## 🧱 Технические изменения

- Поле telegramId как **int64**.

---

## Вариант для Telegram

4.12.1 https://github.com/MrMe0ws/remnawave-telegram-shop/releases/tag/4.12.1

✨ Улучшения
• Поддержка панели Remnawave 2.8.x: клиент выровнен под актуальные требования API при создании и обновлении пользователей.

🧱 Технические изменения
• telegramId в запросах к панели передаётся как int64 без усечения.
• Имя пользователя в панели укладывается в лимит API 3–36 символов (fallback для очень длинных id).
`

func TestParseReleaseNotes(t *testing.T) {
	n, err := ParseReleaseNotes(sampleNotes)
	if err != nil {
		t.Fatal(err)
	}
	if n.Version != "4.12.1" {
		t.Fatalf("version: got %q", n.Version)
	}
	if !strings.Contains(n.ReleaseURL, "releases/tag/4.12.1") {
		t.Fatalf("url: got %q", n.ReleaseURL)
	}
	if !strings.Contains(n.GitHubBody, "## ✨ Улучшения") {
		t.Fatalf("github body missing section: %q", n.GitHubBody)
	}
	if strings.Contains(n.GitHubBody, "---") {
		t.Fatalf("github body should not contain separator: %q", n.GitHubBody)
	}
	if len(n.TGSections) != 2 {
		t.Fatalf("sections: got %d", len(n.TGSections))
	}
	if n.TGSections[0].Header != "✨ Улучшения" || len(n.TGSections[0].Items) != 1 {
		t.Fatalf("section0: %+v", n.TGSections[0])
	}
	if n.TGSections[1].Header != "🧱 Технические изменения" || len(n.TGSections[1].Items) != 2 {
		t.Fatalf("section1: %+v", n.TGSections[1])
	}
}

func TestParseVersionLineParen(t *testing.T) {
	v, u, err := parseVersionLine("4.12.1 (https://github.com/MrMe0ws/remnawave-telegram-shop/releases/tag/4.12.1)")
	if err != nil {
		t.Fatal(err)
	}
	if v != "4.12.1" {
		t.Fatalf("version %q", v)
	}
	if !strings.HasPrefix(u, "https://") {
		t.Fatalf("url %q", u)
	}
}

func TestNormalizeReleaseVersionStripsV(t *testing.T) {
	cases := map[string]string{
		"4.12.2":  "4.12.2",
		"v4.12.2": "4.12.2",
		" v4.12 ": "4.12",
		"":        "",
	}
	for in, want := range cases {
		if got := normalizeReleaseVersion(in); got != want {
			t.Fatalf("normalizeReleaseVersion(%q)=%q, want %q", in, got, want)
		}
	}
	v, _, err := parseVersionLine("v4.12.2 https://github.com/MrMe0ws/remnawave-telegram-shop/releases/tag/4.12.2")
	if err != nil {
		t.Fatal(err)
	}
	if v != "4.12.2" {
		t.Fatalf("parseVersionLine should strip v, got %q", v)
	}
}

func TestFormatTelegramHTML(t *testing.T) {
	n, err := ParseReleaseNotes(sampleNotes)
	if err != nil {
		t.Fatal(err)
	}
	html := FormatTelegramHTML(n, TelegramFooter{
		Text:          "Meows VPN Group",
		URL:           "https://t.me/meows_vpn_bot",
		CustomEmojiID: "5451703345746058024",
	})
	wantParts := []string{
		`<a href="https://github.com/MrMe0ws/remnawave-telegram-shop/releases/tag/4.12.1">4.12.1</a>`,
		`<b>✨ Улучшения</b>`,
		`• Поддержка панели Remnawave 2.8.x`,
		`<b>🧱 Технические изменения</b>`,
		`<tg-emoji emoji-id="5451703345746058024">✨</tg-emoji> <a href="https://t.me/meows_vpn_bot">Meows VPN Group</a>`,
	}
	for _, p := range wantParts {
		if !strings.Contains(html, p) {
			t.Fatalf("missing %q in:\n%s", p, html)
		}
	}
	if strings.Contains(html, "**") {
		t.Fatal("html should not contain markdown bold")
	}
	// После заголовка и между пунктами — пустая строка; между секциями — две.
	if !strings.Contains(html, "<b>✨ Улучшения</b>\n\n• ") {
		t.Fatalf("expected blank line after section header, got:\n%s", html)
	}
	techIdx := strings.Index(html, `<b>🧱 Технические изменения</b>`)
	if techIdx < 0 {
		t.Fatal("tech section missing")
	}
	// Две пустые строки перед второй секцией: …id).\n\n\n<b>🧱
	beforeTech := html[:techIdx]
	if !strings.HasSuffix(beforeTech, "\n\n\n") {
		t.Fatalf("expected two blank lines between sections, suffix=%q\nfull:\n%s", beforeTech[len(beforeTech)-10:], html)
	}
	techBlock := html[techIdx:]
	if !strings.Contains(techBlock, "\n\n• Имя пользователя") {
		t.Fatalf("expected blank line between telegram bullets, got:\n%s", techBlock)
	}
}

func TestFormatTelegramHTML_collapsesLongSecondarySections(t *testing.T) {
	n := &ReleaseNotes{
		Version:    "9.9.9",
		ReleaseURL: "https://example.test/releases/tag/9.9.9",
		TGSections: []TGSection{
			{Header: "🚀 Новое", Items: []string{"a", "b", "c", "d"}},
			{Header: "🐛 Исправления", Items: []string{"x", "y", "z"}},
			{Header: "✨ Улучшения", Items: []string{"p", "q"}},
		},
	}
	got := FormatTelegramHTML(n, TelegramFooter{})

	if strings.Count(got, "<blockquote expandable>") != 1 {
		t.Fatalf("ожидалась ровно одна свёрнутая цитата, получено:\n%s", got)
	}
	if strings.Count(got, "</blockquote>") != 1 {
		t.Errorf("незакрытая цитата:\n%s", got)
	}
	// «Новое» не сворачивается даже при четырёх пунктах.
	newIdx := strings.Index(got, "🚀 Новое")
	quoteIdx := strings.Index(got, "<blockquote expandable>")
	fixIdx := strings.Index(got, "🐛 Исправления")
	if !(newIdx < fixIdx && fixIdx < quoteIdx) {
		t.Errorf("цитата должна открываться после заголовка «Исправления»:\n%s", got)
	}
	// Два пункта в «Улучшениях» остаются без цитаты.
	if strings.Contains(got[strings.Index(got, "✨ Улучшения"):], "<blockquote") {
		t.Errorf("секция из двух пунктов не должна сворачиваться:\n%s", got)
	}
	// Внутри цитаты первый пункт идёт сразу, без пустой строки.
	if !strings.Contains(got, "<blockquote expandable>• x") {
		t.Errorf("первый пункт цитаты не должен начинаться с пустой строки:\n%s", got)
	}
}

func TestCollapsibleSection_threshold(t *testing.T) {
	two := TGSection{Header: "🐛 Исправления", Items: []string{"a", "b"}}
	three := TGSection{Header: "🐛 Исправления", Items: []string{"a", "b", "c"}}
	if collapsibleSection(two) {
		t.Error("два пункта сворачиваться не должны")
	}
	if !collapsibleSection(three) {
		t.Error("три пункта должны сворачиваться")
	}
	if collapsibleSection(TGSection{Header: "🚀 Новое", Items: []string{"a", "b", "c", "d"}}) {
		t.Error("«Новое» не сворачивается")
	}
}

// TestGitHubBodyHasNoTelegramMarkup закрепляет разделение вариантов: сворачивание
// секций в раскрывающуюся цитату — приём Telegram, в GitHub Release уходит сырой
// GFM-блок из файла. Тест страхует от попытки «улучшить» и GitHub тоже.
func TestGitHubBodyHasNoTelegramMarkup(t *testing.T) {
	n, err := ParseReleaseNotes(sampleNotes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, forbidden := range []string{"<blockquote", "</blockquote>", "expandable", "<tg-emoji"} {
		if strings.Contains(n.GitHubBody, forbidden) {
			t.Errorf("GitHub-вариант содержит телеграм-разметку %q:\n%s", forbidden, n.GitHubBody)
		}
	}
	// Пункты GitHub остаются обычным GFM-списком.
	if !strings.Contains(n.GitHubBody, "\n- ") {
		t.Errorf("ожидался GFM-список в GitHub-варианте:\n%s", n.GitHubBody)
	}

	// А в телеграм-варианте разметка появляется — на том же источнике.
	tg := FormatTelegramHTML(n, TelegramFooter{})
	if strings.Contains(n.GitHubBody, "<blockquote") == strings.Contains(tg, "<blockquote") &&
		strings.Contains(tg, "<blockquote") {
		t.Error("разметка утекла в оба варианта")
	}
}

// Строка === делит пост на сообщения: у каждого своя строка версии с номером
// части, футер — только у последнего, плоский список секций — общий.
func TestFormatTelegramParts(t *testing.T) {
	split := strings.Replace(sampleNotes, "\n🧱 Технические изменения", "\n===\n\n🧱 Технические изменения", 1)
	n, err := ParseReleaseNotes(split)
	if err != nil {
		t.Fatal(err)
	}
	if len(n.TGParts) != 2 || len(n.TGSections) != 2 {
		t.Fatalf("parts=%d sections=%d, want 2 and 2", len(n.TGParts), len(n.TGSections))
	}
	footer := TelegramFooter{Text: "Meows VPN Group", URL: "https://t.me/meows_vpn_bot"}
	parts := FormatTelegramParts(n, footer)
	if len(parts) != 2 {
		t.Fatalf("got %d messages, want 2", len(parts))
	}
	if !strings.Contains(parts[0], "4.12.1</a> · часть 1 из 2") || !strings.Contains(parts[1], "4.12.1</a> · часть 2 из 2") {
		t.Fatalf("version lines:\n%s\n---\n%s", parts[0], parts[1])
	}
	if strings.Contains(parts[0], "Технические") || !strings.Contains(parts[1], "<b>🧱 Технические изменения</b>") {
		t.Fatalf("sections went to the wrong message:\n%s\n---\n%s", parts[0], parts[1])
	}
	if strings.Contains(parts[0], "Meows VPN Group") || !strings.Contains(parts[1], "Meows VPN Group") {
		t.Fatal("footer must be only under the last message")
	}

	// Без разбивки — одно сообщение, как раньше.
	one, err := ParseReleaseNotes(sampleNotes)
	if err != nil {
		t.Fatal(err)
	}
	if got := FormatTelegramParts(one, footer); len(got) != 1 || got[0] != FormatTelegramHTML(one, footer) {
		t.Fatal("post without === must stay one message")
	}

	for _, bad := range []string{
		strings.Replace(sampleNotes, "\n✨ Улучшения\n", "\n===\n✨ Улучшения\n", 1),
		strings.TrimRight(sampleNotes, "\n") + "\n===\n",
	} {
		if _, err := ParseReleaseNotes(bad); err == nil {
			t.Fatal("=== at the start or the end of the block must be rejected")
		}
	}
}
