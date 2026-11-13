package service

import (
	"bytes"
	"encoding/binary"
	"errors"
	"slices"
)

// Изображения в чате поддержки. Лимиты и форматы — те же, что проверяет support-bot
// (telegram-support-bot, app/services/cabinet_media.py): проверяем и здесь, чтобы
// не гонять до него заведомо негодные файлы и отвечать клиенту понятной ошибкой.
const (
	SupportMaxFiles      = 5
	SupportMaxFileBytes  = 10 << 20 // лимит sendPhoto в Telegram
	SupportMaxTotalBytes = 25 << 20
)

var (
	ErrSupportTooManyFiles     = errors.New("support: too many files")
	ErrSupportFileTooLarge     = errors.New("support: file too large")
	ErrSupportUnsupportedMedia = errors.New("support: unsupported media")
)

// SupportUpload — файл, который клиент приложил к сообщению.
type SupportUpload struct {
	Name string
	Data []byte
}

// detectSupportImageMime — MIME по первым байтам; "" — не изображение из списка.
// Форматы — как у фото и картинок, которые клиент шлёт боту в Telegram.
// http.DetectContentType не годится: он не знает HEIC, AVIF и TIFF.
func detectSupportImageMime(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg"
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	case bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")):
		return "image/gif"
	case len(data) >= 18 && string(data[0:2]) == "BM" && isBMPHeaderSize(binary.LittleEndian.Uint32(data[14:18])):
		return "image/bmp"
	case bytes.HasPrefix(data, []byte("II*\x00")) || bytes.HasPrefix(data, []byte("MM\x00*")):
		return "image/tiff"
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		return isobmffImageMime(data)
	}
	return ""
}

func isBMPHeaderSize(n uint32) bool {
	switch n {
	case 12, 40, 52, 56, 64, 108, 124:
		return true
	}
	return false
}

// isobmffImageMime — HEIC/HEIF/AVIF: основной бренд ftyp и совместимые за ним.
func isobmffImageMime(data []byte) string {
	end := int(binary.BigEndian.Uint32(data[0:4]))
	end = min(max(end, 16), len(data), 64)
	brands := []string{string(data[8:12])}
	for i := 16; i+4 <= end; i += 4 {
		brands = append(brands, string(data[i:i+4]))
	}
	has := func(set ...string) bool {
		for _, b := range brands {
			if slices.Contains(set, b) {
				return true
			}
		}
		return false
	}
	switch {
	case has("avif", "avis"):
		return "image/avif"
	case has("heic", "heix", "heim", "heis", "hevc", "hevx"):
		return "image/heic"
	case has("mif1", "msf1"):
		return "image/heif"
	}
	return ""
}

// validateSupportUploads проверяет число, размер и формат файлов.
func validateSupportUploads(files []SupportUpload) error {
	if len(files) > SupportMaxFiles {
		return ErrSupportTooManyFiles
	}
	total := 0
	for _, f := range files {
		if len(f.Data) == 0 {
			return ErrSupportUnsupportedMedia
		}
		if len(f.Data) > SupportMaxFileBytes {
			return ErrSupportFileTooLarge
		}
		total += len(f.Data)
		if total > SupportMaxTotalBytes {
			return ErrSupportFileTooLarge
		}
		if detectSupportImageMime(f.Data) == "" {
			return ErrSupportUnsupportedMedia
		}
	}
	return nil
}
