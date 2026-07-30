package api

// Telegram delivery needs to turn a notification's (kind, payload) into readable text on the
// server side — there's no frontend tr() to call from Go. Rather than maintaining a second,
// separately-translated copy of "New comment" / "Status changed" / etc. (which could drift from
// what the bell already shows for the exact same event), this loads the frontend's own
// web/src/locales/*.json files — embedded as source, always present regardless of whether
// `npm run build` has run — and replays NotificationsPage's own rendering logic (views.tsx)
// line-for-line. Keep the two in sync if that rendering ever changes.

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	assets "github.com/DanMotive/Todorio"
)

var (
	localeStringsOnce sync.Once
	localeStrings     map[string]map[string]string // locale -> key -> value
)

func loadLocaleStrings() map[string]map[string]string {
	localeStringsOnce.Do(func() {
		localeStrings = map[string]map[string]string{}
		entries, err := assets.Locales.ReadDir("web/src/locales")
		if err != nil {
			log.Printf("notification locales: read directory: %v", err)
			return
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".json") {
				continue
			}
			b, err := assets.Locales.ReadFile("web/src/locales/" + name)
			if err != nil {
				log.Printf("notification locales: read %s: %v", name, err)
				continue
			}
			var m map[string]string
			if err := json.Unmarshal(b, &m); err != nil {
				log.Printf("notification locales: parse %s: %v", name, err)
				continue
			}
			localeStrings[strings.TrimSuffix(name, ".json")] = m
		}
	})
	return localeStrings
}

// trServer is the backend's equivalent of the frontend's tr(): looks up key in locale, falling
// back to en-US (mirroring i18n.ts's own fallback chain), and finally the bare key if even
// en-US doesn't have it — this only ever feeds a Telegram message, never worth a hard failure.
func trServer(locale, key string) string {
	strs := loadLocaleStrings()
	if m, ok := strs[locale]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	if m, ok := strs["en-US"]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}

// toInt accepts whatever numeric type a payload value happens to be. Payloads built in-process
// (this file's caller) hold plain Go int/int64; anything that made a round trip through JSON
// (there isn't one on this path today, but defending against it costs nothing) would be
// float64. Never worth failing a notification over a type mismatch — just skip the suffix.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

// formatNotifText mirrors NotificationsPage's rendering in views.tsx: the kind label, then
// title/task_title in « » quotes, "by @username", and the reaction emoji — same keys, same
// punctuation, so a Telegram message reads exactly like the bell entry for the same event.
func formatNotifText(locale, kind string, payload map[string]any) string {
	text := trServer(locale, "notif.kind."+kind)
	str := func(k string) string {
		v, _ := payload[k].(string)
		return v
	}

	switch kind {
	case "due_soon":
		if d, ok := toInt(payload["days"]); ok {
			text += strings.ReplaceAll(trServer(locale, "notif.days_suffix"), "{days}", fmt.Sprint(d))
		}
	case "archive_expiring":
		if d, ok := toInt(payload["days_left"]); ok {
			text += strings.ReplaceAll(trServer(locale, "notif.days_suffix"), "{days}", fmt.Sprint(d))
		}
	}
	if t := str("title"); t != "" {
		text += " · «" + t + "»"
	}
	if t := str("task_title"); t != "" {
		text += " · «" + t + "»"
	}
	if by := str("by"); by != "" {
		text += " · " + trServer(locale, "notif.by") + " @" + by
	}
	if emoji := str("emoji"); emoji != "" {
		text += " " + emoji
	}
	// Announcements carry free-form admin-written text that can't be templated — append it
	// verbatim under the (translated) "Announcement" label rather than dropping it.
	if kind == "announcement" {
		if body := str("body"); body != "" {
			text += "\n" + body
		}
	}
	return text
}
