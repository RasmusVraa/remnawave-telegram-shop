// Command release publishes RELEASE_NOTES.md to GitHub Releases and/or Telegram.
// Not part of the shop Docker image (built only via ./cmd/app).
//
// Usage (from repo root):
//
//	go run ./tools/release preview
//	go run ./tools/release github
//	go run ./tools/release telegram
//	go run ./tools/release photos <message_id>
//	go run ./tools/release publish
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "release: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printUsage()
		return nil
	}
	cmd := args[0]

	root, err := findRepoRoot()
	if err != nil {
		return err
	}
	if err := loadEnvRelease(root); err != nil {
		return err
	}

	notesPath := filepath.Join(root, "RELEASE_NOTES.md")
	if v := strings.TrimSpace(os.Getenv("RELEASE_NOTES_PATH")); v != "" {
		notesPath = v
		if !filepath.IsAbs(notesPath) {
			notesPath = filepath.Join(root, notesPath)
		}
	}

	raw, err := os.ReadFile(notesPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", notesPath, err)
	}
	notes, err := ParseReleaseNotes(string(raw))
	if err != nil {
		return err
	}
	repo := strings.TrimSpace(os.Getenv("RELEASE_GITHUB_REPO"))

	switch cmd {
	case "preview":
		cfg, err := loadTelegramConfig()
		if err != nil {
			return err
		}
		html := FormatTelegramHTML(notes, cfg.Footer)
		fmt.Printf("version: %s\n", notes.Version)
		fmt.Printf("release_url: %s\n", notes.ReleaseURL)
		fmt.Printf("github_repo: %s\n", emptyAs(repo, "(default gh remote)"))
		fmt.Println("--- GitHub notes ---")
		fmt.Println(notes.GitHubBody)
		fmt.Println("--- Telegram HTML ---")
		fmt.Println(html)
		photos, err := listReleasePhotos(releasePhotosDir(root))
		if err != nil {
			return err
		}
		switch n := telegramTextRunes(html); {
		case len(photos) == 0:
			fmt.Println("--- Telegram album: none ---")
		case fitsCaption(html):
			fmt.Printf("--- Telegram album: %d screenshot(s), post is their caption (%d/%d chars) ---\n", len(photos), n, telegramMaxCaptionRunes)
		default:
			fmt.Printf("--- Telegram album: %d screenshot(s), reply to the post (post %d chars > caption limit %d) ---\n", len(photos), n, telegramMaxCaptionRunes)
		}
		for _, p := range photos {
			fmt.Println(filepath.Base(p))
		}
		return nil
	case "github":
		return createGitHubRelease(notes.Version, notes.GitHubBody, repo)
	case "telegram":
		cfg, err := loadTelegramConfig()
		if err != nil {
			return err
		}
		return announceTelegram(root, notes, cfg)
	case "photos":
		// Повтор одного альбома, если пост ушёл, а картинки нет: второй
		// `telegram` отправил бы дубликат поста.
		if len(args) < 2 {
			return fmt.Errorf("usage: go run ./tools/release photos <message_id of the release post>")
		}
		replyTo, err := strconv.Atoi(args[1])
		if err != nil || replyTo <= 0 {
			return fmt.Errorf("invalid message_id %q", args[1])
		}
		cfg, err := loadTelegramConfig()
		if err != nil {
			return err
		}
		photos, err := listReleasePhotos(releasePhotosDir(root))
		if err != nil {
			return err
		}
		if _, err := sendTelegramPhotos(cfg, replyTo, photosCaption(notes.Version), "", photos); err != nil {
			return err
		}
		fmt.Printf("telegram: album sent (%d)\n", len(photos))
		return nil
	case "publish":
		if err := createGitHubRelease(notes.Version, notes.GitHubBody, repo); err != nil {
			return err
		}
		fmt.Println("github: release created")
		cfg, err := loadTelegramConfig()
		if err != nil {
			return fmt.Errorf("github release created, but telegram config invalid: %w", err)
		}
		if err := announceTelegram(root, notes, cfg); err != nil {
			return fmt.Errorf("github release created, but telegram failed: %w", err)
		}
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

// announceTelegram — пост релиза и, если есть скриншоты, альбом: короткий пост
// уходит подписью к нему, длинный — отдельно, а альбом ответом на него.
func announceTelegram(root string, notes *ReleaseNotes, cfg telegramConfig) error {
	// Скриншоты проверяются до поста: битая папка не должна оставить пост без них.
	photos, err := listReleasePhotos(releasePhotosDir(root))
	if err != nil {
		return err
	}
	html := FormatTelegramHTML(notes, cfg.Footer)
	if len(photos) > 0 && fitsCaption(html) {
		if strings.TrimSpace(cfg.Token) == "" || strings.TrimSpace(cfg.ChatID) == "" {
			return fmt.Errorf("RELEASE_TG_BOT_TOKEN and RELEASE_TG_CHAT_ID must be set")
		}
		messageID, err := sendTelegramPhotos(cfg, 0, html, "HTML", photos)
		if err != nil {
			return fmt.Errorf("%w (retry: go run ./tools/release telegram)", err)
		}
		fmt.Printf("telegram: post sent with %d screenshot(s) attached (id %d)\n", len(photos), messageID)
		return nil
	}
	messageID, err := sendTelegramHTML(cfg, html)
	if err != nil {
		return fmt.Errorf("%w (retry: go run ./tools/release telegram)", err)
	}
	fmt.Printf("telegram: message sent (id %d)\n", messageID)
	if len(photos) == 0 {
		return nil
	}
	if _, err := sendTelegramPhotos(cfg, messageID, photosCaption(notes.Version), "", photos); err != nil {
		return fmt.Errorf("post sent, album failed (retry only the album: go run ./tools/release photos %d): %w", messageID, err)
	}
	fmt.Printf("telegram: album sent (%d)\n", len(photos))
	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `usage:
  go run ./tools/release preview    # print parsed GitHub + Telegram HTML
  go run ./tools/release github     # gh release create from RELEASE_NOTES.md
  go run ./tools/release telegram   # send Telegram HTML (+ screenshots album) to forum topic
  go run ./tools/release photos ID  # resend only the screenshots album as a reply to post ID
  go run ./tools/release publish    # github + telegram

Screenshots: release-screenshots/*.png|jpg|webp in repo root (or RELEASE_TG_PHOTOS_DIR),
sent in file-name order as an album. A post of up to 1024 visible chars becomes the
album's caption (one message); a longer post goes first and the album replies to it.
No folder — no album.

Config: .env.release in repo root (see .env.release.sample).
`)
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, "RELEASE_NOTES.md")) {
			return dir, nil
		}
		if fileExists(filepath.Join(dir, "go.mod")) {
			// Prefer go.mod root even if RELEASE_NOTES.md missing (preview will fail later).
			if fileExists(filepath.Join(dir, ".git")) || fileExists(filepath.Join(dir, ".env.release.sample")) {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return wd, nil
}

func loadEnvRelease(root string) error {
	path := filepath.Join(root, ".env.release")
	if !fileExists(path) {
		return nil
	}
	if err := godotenv.Load(path); err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	return nil
}

func loadTelegramConfig() (telegramConfig, error) {
	threadID, err := parseThreadID(os.Getenv("RELEASE_TG_MESSAGE_THREAD_ID"))
	if err != nil {
		return telegramConfig{}, err
	}
	footerText := strings.TrimSpace(os.Getenv("RELEASE_TG_FOOTER_TEXT"))
	if footerText == "" {
		footerText = "Meows VPN Group"
	}
	footerURL := strings.TrimSpace(os.Getenv("RELEASE_TG_FOOTER_URL"))
	if footerURL == "" {
		footerURL = "https://t.me/meows_vpn_bot"
	}
	return telegramConfig{
		Token:           strings.TrimSpace(os.Getenv("RELEASE_TG_BOT_TOKEN")),
		ChatID:          strings.TrimSpace(os.Getenv("RELEASE_TG_CHAT_ID")),
		MessageThreadID: threadID,
		Footer: TelegramFooter{
			Text:           footerText,
			URL:            footerURL,
			CustomEmojiID:  strings.TrimSpace(os.Getenv("RELEASE_TG_FOOTER_CUSTOM_EMOJI_ID")),
			CustomEmojiAlt: strings.TrimSpace(os.Getenv("RELEASE_TG_FOOTER_CUSTOM_EMOJI_ALT")),
		},
		ProxyURL: strings.TrimSpace(os.Getenv("RELEASE_TG_PROXY_URL")),
	}, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func emptyAs(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
