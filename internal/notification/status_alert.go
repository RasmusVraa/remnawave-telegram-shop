package notification

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/adminalert"
	"remnawave-tg-shop-bot/internal/adminevents"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/statusprobe"
	"remnawave-tg-shop-bot/utils"
)

// StatusAlerts возвращает обработчик для statusprobe.Runner.OnAlerts: одно
// сообщение на проход проверок в вид «Статус серверов» «Оповещений админу».
// publicURL — адрес кабинета для ссылки на страницу статуса в админке, может быть пустым.
func StatusAlerts(publicURL string) func([]statusprobe.AlertEvent) {
	return func(events []statusprobe.AlertEvent) {
		emitStatusEvents(events)
		text := StatusAlertText(events, config.DefaultLanguage(), publicURL)
		if text == "" {
			return
		}
		utils.Go("status alert", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := adminalert.Send(ctx, config.AdminAlertServers, text, nil); err != nil {
				slog.Error("status alert: send failed", "error", err)
			}
		})
	}
}

// emitStatusEvents — сбои и восстановления со страницы статуса в «Мониторинг».
// Это взгляд снаружи (зонды из России и мира), он дополняет опрос панели.
func emitStatusEvents(events []statusprobe.AlertEvent) {
	for _, e := range events {
		typ := adminevents.NodeUp
		payload := map[string]any{"name": e.Name, "country": e.Country, "reason": e.Reason, "probe": true}
		if e.Open {
			typ = adminevents.NodeDown
			if e.Reason == statusprobe.AlertRussia {
				payload["ok"], payload["total"] = e.RussiaOK, e.RussiaTotal
			}
		} else if !e.Since.IsZero() {
			payload["down_minutes"] = int(e.At.Sub(e.Since).Minutes())
		}
		adminevents.Emit(adminevents.Event{Type: typ, Source: adminevents.SourceSystem, At: e.At, Payload: payload})
	}
}

type statusAlertWords struct {
	down, up, russia, world, operators, noAnswer, more, lasted, open string
	russiaRatio, worldSilent, operatorsSilent                        string
	registryListed, registryCDN, registryClear, registryWas          string
	lessMinute, minutes, hours                                       string
}

var statusAlertRU = statusAlertWords{
	down:            "🔴 <b>Сбой серверов</b>",
	up:              "✅ <b>Восстановились</b>",
	russia:          "из России",
	world:           "из мира",
	operators:       "операторы",
	noAnswer:        "не ответили",
	more:            "и ещё %d",
	lasted:          "сбой длился %s",
	open:            "Открыть статус",
	russiaRatio:     "из России %d из %d",
	worldSilent:     "из мира не отвечает",
	operatorsSilent: "операторы: не отвечают %s",
	registryListed:  "адрес найден в реестре РКН, из России может не открываться",
	registryCDN:     "адрес в диапазонах заблокированного CDN, из России может не открываться",
	registryClear:   "больше не в списках блокировок",
	registryWas:     "был в списках %s",
	lessMinute:      "меньше минуты",
	minutes:         "%d мин",
	hours:           "%d ч",
}

var statusAlertEN = statusAlertWords{
	down:            "🔴 <b>Servers down</b>",
	up:              "✅ <b>Back up</b>",
	russia:          "from Russia",
	world:           "from the world",
	operators:       "operators",
	noAnswer:        "no answer",
	more:            "and %d more",
	lasted:          "outage lasted %s",
	open:            "Open status",
	russiaRatio:     "from Russia %d of %d",
	worldSilent:     "unreachable from the world",
	operatorsSilent: "operators: no answer from %s",
	registryListed:  "address is in the Roskomnadzor registry, may not open from Russia",
	registryCDN:     "address is in a blocked CDN range, may not open from Russia",
	registryClear:   "no longer in the block lists",
	registryWas:     "was listed for %s",
	lessMinute:      "under a minute",
	minutes:         "%d min",
	hours:           "%d h",
}

// StatusAlertText собирает сообщение: сначала сбои, потом восстановившиеся.
// Пусто, если событий нет.
func StatusAlertText(events []statusprobe.AlertEvent, lang, publicURL string) string {
	if len(events) == 0 {
		return ""
	}
	w := statusAlertRU
	if strings.HasPrefix(strings.ToLower(lang), "en") {
		w = statusAlertEN
	}
	var down, up []string
	for _, ev := range events {
		head := strings.TrimSpace(countryFlag(ev.Country)+" <b>"+html.EscapeString(ev.Name)+"</b>") + " — "
		if !ev.Open {
			if ev.Reason == statusprobe.AlertRegistry {
				up = append(up, head+w.registryClear+", "+fmt.Sprintf(w.registryWas, outage(w, ev.At.Sub(ev.Since))))
				continue
			}
			up = append(up, head+reasonWord(w, ev.Reason)+", "+fmt.Sprintf(w.lasted, outage(w, ev.At.Sub(ev.Since))))
			continue
		}
		names := failedNames(w, ev)
		switch ev.Reason {
		case statusprobe.AlertRussia:
			line := head + fmt.Sprintf(w.russiaRatio, ev.RussiaOK, ev.RussiaTotal)
			if names != "" {
				line += "\n" + w.noAnswer + ": " + names
			}
			down = append(down, line)
		case statusprobe.AlertWorld:
			down = append(down, head+w.worldSilent)
		case statusprobe.AlertOperators:
			down = append(down, head+fmt.Sprintf(w.operatorsSilent, names))
		case statusprobe.AlertRegistry:
			if ev.Note == statusprobe.RegistryKindCDN {
				down = append(down, head+w.registryCDN)
			} else {
				down = append(down, head+w.registryListed)
			}
		}
	}
	var b strings.Builder
	if len(down) > 0 {
		b.WriteString(w.down + "\n\n" + strings.Join(down, "\n"))
	}
	if len(up) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(w.up + "\n\n" + strings.Join(up, "\n"))
	}
	if base := strings.TrimRight(strings.TrimSpace(publicURL), "/"); base != "" {
		b.WriteString("\n\n<a href=\"" + html.EscapeString(base+"/cabinet/admin/status") + "\">" + w.open + "</a>")
	}
	return b.String()
}

func reasonWord(w statusAlertWords, reason string) string {
	switch reason {
	case statusprobe.AlertRussia:
		return w.russia
	case statusprobe.AlertWorld:
		return w.world
	case statusprobe.AlertOperators:
		return w.operators
	}
	return reason
}

func failedNames(w statusAlertWords, ev statusprobe.AlertEvent) string {
	if len(ev.Failed) == 0 {
		return ""
	}
	escaped := make([]string, 0, len(ev.Failed))
	for _, name := range ev.Failed {
		escaped = append(escaped, html.EscapeString(name))
	}
	s := strings.Join(escaped, ", ")
	if ev.FailedMore > 0 {
		s += " " + fmt.Sprintf(w.more, ev.FailedMore)
	}
	return s
}

func outage(w statusAlertWords, d time.Duration) string {
	minutes := int(d / time.Minute)
	if minutes < 1 {
		return w.lessMinute
	}
	if minutes < 60 {
		return fmt.Sprintf(w.minutes, minutes)
	}
	s := fmt.Sprintf(w.hours, minutes/60)
	if rest := minutes % 60; rest > 0 {
		s += " " + fmt.Sprintf(w.minutes, rest)
	}
	return s
}

// countryFlag — эмодзи флага по коду страны из двух букв, иначе пусто.
func countryFlag(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 2 || code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' {
		return ""
	}
	return string([]rune{rune(code[0]-'A') + 0x1F1E6, rune(code[1]-'A') + 0x1F1E6})
}
