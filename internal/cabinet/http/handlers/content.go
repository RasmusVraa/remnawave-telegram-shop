package handlers

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	"remnawave-tg-shop-bot/internal/translation"
	translationfs "remnawave-tg-shop-bot/translations"
)

// CabinetContentHandler отдаёт runtime-контент кабинета из файлов /translations/*.
// Это позволяет менять FAQ/тексты без пересборки Docker-образа.
type CabinetContentHandler struct {
	faqCandidates       []string
	appConfigCandidates []string
	i18nCandidates      []string
}

func NewCabinetContentHandler() *CabinetContentHandler {
	return &CabinetContentHandler{
		faqCandidates: []string{
			"/translations/cabinet/FAQ.json",
			filepath.Join("translations", "cabinet", "FAQ.json"),
		},
		appConfigCandidates: []string{
			"/translations/cabinet/app-config.json",
			filepath.Join("translations", "cabinet", "app-config.json"),
		},
		i18nCandidates: []string{
			"/translations/cabinet/i18n",
			filepath.Join("translations", "cabinet", "i18n"),
			filepath.Join("web", "cabinet", "src", "i18n"),
		},
	}
}

// FAQ — GET /cabinet/api/content/faq
func (h *CabinetContentHandler) FAQ(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := h.readMerged("cabinet/FAQ.json", h.faqCandidates)
	if err != nil {
		http.Error(w, "faq config not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

// I18n — GET /cabinet/api/content/i18n/{lang}
func (h *CabinetContentHandler) I18n(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	lang := strings.TrimSpace(r.PathValue("lang"))
	if lang == "" {
		http.Error(w, "language required", http.StatusBadRequest)
		return
	}
	if strings.Contains(lang, "..") || strings.ContainsAny(lang, `/\`) {
		http.Error(w, "invalid language", http.StatusBadRequest)
		return
	}

	body, err := h.readFirstExisting(h.i18nFileCandidates(lang))
	if err != nil {
		http.Error(w, "i18n bundle not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

func (h *CabinetContentHandler) i18nFileCandidates(lang string) []string {
	fileName := lang + ".json"
	out := make([]string, 0, len(h.i18nCandidates))
	for _, dir := range h.i18nCandidates {
		out = append(out, filepath.Join(dir, fileName))
	}
	return out
}

// AppConfig — GET /cabinet/api/content/app-config
func (h *CabinetContentHandler) AppConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := h.readMerged("cabinet/app-config.json", h.appConfigCandidates)
	if err != nil {
		http.Error(w, "app config not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

// PWAManifest — GET /cabinet/api/public/pwa-manifest.webmanifest
func (h *CabinetContentHandler) PWAManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	iconSrc := "/cabinet/favicon.svg"
	iconType := "image/svg+xml"
	if u := strings.TrimSpace(cabcfg.BrandLogoURLForClient()); u != "" {
		iconSrc = u
		ext := strings.ToLower(filepath.Ext(u))
		if mt := mime.TypeByExtension(ext); mt != "" {
			iconType = mt
		}
	}
	manifest := map[string]any{
		"name":             cabcfg.PWAAppName(),
		"short_name":       cabcfg.PWAShortName(),
		"start_url":        "/cabinet/",
		"scope":            "/cabinet/",
		"display":          "standalone",
		"background_color": "#0a0f1e",
		"theme_color":      "#0a0f1e",
		"icons": []map[string]any{
			{
				"src":   iconSrc,
				"sizes": "any",
				"type":  iconType,
			},
		},
	}
	w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(manifest)
}

func (h *CabinetContentHandler) readFirstExisting(candidates []string) ([]byte, error) {
	var lastErr error
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err == nil {
			return b, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	if errors.Is(lastErr, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	return nil, lastErr
}

// readMerged: вшитый файл из образа как база, диск поверх.
// На томе могут быть старые FAQ/app-config — недостающие ключи/приложения
// (например RabbitHole) подтянутся из бинарника, правки на диске сохранятся.
func (h *CabinetContentHandler) readMerged(bundledPath string, candidates []string) ([]byte, error) {
	bundled, bundErr := translationfs.FS.ReadFile(bundledPath)
	disk, diskErr := h.readFirstExisting(candidates)
	switch {
	case bundErr == nil && diskErr == nil:
		merged, err := translation.DeepMergeJSON(bundled, disk)
		if err != nil {
			// Битый диск — лучше отдать то, что в образе, чем упасть 500.
			return bundled, nil
		}
		return merged, nil
	case bundErr == nil:
		return bundled, nil
	case diskErr == nil:
		return disk, nil
	default:
		return nil, diskErr
	}
}
