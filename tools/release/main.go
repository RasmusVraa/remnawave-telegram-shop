// Command release publishes RELEASE_NOTES.md to GitHub Releases and/or Telegram.
// Not part of the shop Docker image (built only via ./cmd/app).
//
// Usage (from repo root):
//
//	go run ./tools/release preview
//	go run ./tools/release github
//	go run ./tools/release telegram
//	go run ./tools/release photos <message_id> [album number]
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
		parts := FormatTelegramParts(notes, cfg.Footer)
		fmt.Printf("version: %s\n", notes.Version)
		fmt.Printf("release_url: %s\n", notes.ReleaseURL)
		fmt.Printf("github_repo: %s\n", emptyAs(repo, "(default gh remote)"))
		fmt.Println("--- GitHub notes ---")
		fmt.Println(notes.GitHubBody)
		for i, html := range parts {
			n := telegramTextRunes(html)
			over := ""
			if n > telegramMaxMessageRunes {
				over = " — OVER LIMIT, shorten this part"
			}
			fmt.Printf("--- Telegram message %d/%d: %d/%d chars%s ---\n", i+1, len(parts), n, telegramMaxMessageRunes, over)
			fmt.Println(html)
		}
		albums, err := listReleaseAlbums(releasePhotosDir(root))
		if err != nil {
			return err
		}
		if len(albums) == 0 {
			fmt.Println("--- Telegram albums: none ---")
		}
		for i, photos := range albums {
			if albumIsCaption(parts, albums) {
				fmt.Printf("--- Telegram album: %d screenshot(s), post is their caption (%d/%d chars) ---\n", len(photos), telegramTextRunes(parts[0]), telegramMaxCaptionRunes)
			} else {
				fmt.Printf("--- Telegram album %d/%d: %d screenshot(s), reply to message %d ---\n", i+1, len(albums), len(photos), albumPost(i, len(parts))+1)
			}
			for _, p := range photos {
				fmt.Println(filepath.Base(p))
			}
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
	case "edit":
		// Правка уже отправленного поста: текст берётся из RELEASE_NOTES.md.
		if len(args) < 2 {
			return fmt.Errorf("usage: go run ./tools/release edit <message_id of the release post>")
		}
		messageID, err := strconv.Atoi(args[1])
		if err != nil || messageID <= 0 {
			return fmt.Errorf("invalid message_id %q", args[1])
		}
		cfg, err := loadTelegramConfig()
		if err != nil {
			return err
		}
		parts := FormatTelegramParts(notes, cfg.Footer)
		if len(parts) != 1 {
			return fmt.Errorf("edit works with a single-message post, got %d parts", len(parts))
		}
		if err := editTelegramHTML(cfg, messageID, parts[0]); err != nil {
			return err
		}
		fmt.Printf("telegram: message %d edited\n", messageID)
		return nil
	case "photos":
		// Повтор одного альбома, если пост ушёл, а картинки нет: второй
		// `telegram` отправил бы дубликат поста.
		if len(args) < 2 {
			return fmt.Errorf("usage: go run ./tools/release photos <message_id of the release post> [album number]")
		}
		replyTo, err := strconv.Atoi(args[1])
		if err != nil || replyTo <= 0 {
			return fmt.Errorf("invalid message_id %q", args[1])
		}
		cfg, err := loadTelegramConfig()
		if err != nil {
			return err
		}
		albums, err := listReleaseAlbums(releasePhotosDir(root))
		if err != nil {
			return err
		}
		if len(albums) == 0 {
			return fmt.Errorf("no screenshots in %s", releasePhotosDir(root))
		}
		album := 1
		if len(args) >= 3 {
			album, err = strconv.Atoi(args[2])
			if err != nil || album < 1 || album > len(albums) {
				return fmt.Errorf("invalid album number %q: there are %d album(s)", args[2], len(albums))
			}
		} else if len(albums) > 1 {
			return fmt.Errorf("%d albums: pass the album number, e.g. go run ./tools/release photos %d 2", len(albums), replyTo)
		}
		photos := albums[album-1]
		if _, err := sendTelegramPhotos(cfg, replyTo, photosCaption(notes.Version, album, len(albums)), "", photos); err != nil {
			return err
		}
		fmt.Printf("telegram: album %d sent (%d)\n", album, len(photos))
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

// albumIsCaption — пост из одного сообщения с одним альбомом и влезает в
// подпись: уходит одним сообщением, картинки прямо над текстом.
func albumIsCaption(parts []string, albums [][]string) bool {
	return len(parts) == 1 && len(albums) == 1 && fitsCaption(parts[0])
}

// announceTelegram — пост релиза и альбомы скриншотов. Короткий пост с одним
// альбомом уходит подписью к нему. Иначе сообщения поста идут по порядку, и
// за каждым — его альбом ответом на него: пост 1, альбом 1, пост 2, альбом 2.
func announceTelegram(root string, notes *ReleaseNotes, cfg telegramConfig) error {
	// Скриншоты и длина проверяются до первого сообщения: битая папка или
	// длинная вторая часть не должны оставить пост наполовину отправленным.
	albums, err := listReleaseAlbums(releasePhotosDir(root))
	if err != nil {
		return err
	}
	parts := FormatTelegramParts(notes, cfg.Footer)
	for i, html := range parts {
		if n := telegramTextRunes(html); n > telegramMaxMessageRunes {
			return fmt.Errorf("telegram message %d/%d is %d chars (limit %d); shorten it in RELEASE_NOTES.md", i+1, len(parts), n, telegramMaxMessageRunes)
		}
	}
	if albumIsCaption(parts, albums) {
		if strings.TrimSpace(cfg.Token) == "" || strings.TrimSpace(cfg.ChatID) == "" {
			return fmt.Errorf("RELEASE_TG_BOT_TOKEN and RELEASE_TG_CHAT_ID must be set")
		}
		messageID, err := sendTelegramPhotos(cfg, 0, parts[0], "HTML", albums[0])
		if err != nil {
			return fmt.Errorf("%w (retry: go run ./tools/release telegram)", err)
		}
		fmt.Printf("telegram: post sent with %d screenshot(s) attached (id %d)\n", len(albums[0]), messageID)
		return nil
	}
	for i, html := range parts {
		messageID, err := sendTelegramHTML(cfg, html)
		if err != nil {
			if i == 0 {
				return fmt.Errorf("%w (retry: go run ./tools/release telegram)", err)
			}
			return fmt.Errorf("message %d/%d failed after earlier ones were sent; send the rest by hand, a full retry would duplicate them: %w", i+1, len(parts), err)
		}
		fmt.Printf("telegram: message %d/%d sent (id %d)\n", i+1, len(parts), messageID)
		for a, photos := range albums {
			if albumPost(a, len(parts)) != i {
				continue
			}
			if _, err := sendTelegramPhotos(cfg, messageID, photosCaption(notes.Version, a+1, len(albums)), "", photos); err != nil {
				return fmt.Errorf("album %d failed (retry only it: go run ./tools/release photos %d %d): %w", a+1, messageID, a+1, err)
			}
			fmt.Printf("telegram: album %d/%d sent (%d)\n", a+1, len(albums), len(photos))
		}
	}
	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `usage:
  go run ./tools/release preview    # print parsed GitHub + Telegram HTML
  go run ./tools/release github     # gh release create from RELEASE_NOTES.md
  go run ./tools/release telegram   # send Telegram HTML (+ screenshots album) to forum topic
  go run ./tools/release edit ID        # replace the text of an already sent post
  go run ./tools/release photos ID [N]  # resend only album N as a reply to message ID
  go run ./tools/release publish    # github + telegram

Long post: a line "===" in the Telegram block starts the next message (each up to
4096 visible chars).

Screenshots: release-screenshots/*.png|jpg|webp in repo root (or RELEASE_TG_PHOTOS_DIR),
sent in file-name order as albums of up to 10. Subfolders 1/, 2/ … make one album
each; album N replies to message N. A single-message post of up to 1024 visible
chars with one album becomes the album's caption. No folder — no album.

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
