package api

import (
	"encoding/json"
	"testing"
)

// These exercise formatNotifText/trServer without a database — the only dependency is the
// embedded web/src/locales/*.json, which is always present (source, not build output).

func TestFormatNotifTextBasicKind(t *testing.T) {
	got := formatNotifText("en-US", "task_assigned", map[string]any{"task_id": int64(1), "title": "Ship it", "by": "dan"})
	want := "Task assigned to you · «Ship it» · by @dan"
	if got != want {
		t.Errorf("formatNotifText = %q, want %q", got, want)
	}
}

func TestFormatNotifTextCommentUsesTaskTitleField(t *testing.T) {
	// "comment" notifications carry task_title, not title (see social.go) — a formatter that
	// only checked one of the two would silently drop the task name for this kind.
	got := formatNotifText("en-US", "comment", map[string]any{"task_title": "Ship it", "by": "dan"})
	want := "New comment · «Ship it» · by @dan"
	if got != want {
		t.Errorf("formatNotifText = %q, want %q", got, want)
	}
}

func TestFormatNotifTextReaction(t *testing.T) {
	got := formatNotifText("en-US", "reaction", map[string]any{"by": "dan", "emoji": "👍"})
	want := "New reaction · by @dan 👍"
	if got != want {
		t.Errorf("formatNotifText = %q, want %q", got, want)
	}
}

func TestFormatNotifTextDaysSuffixHandlesPlainIntPayload(t *testing.T) {
	// notify() calls formatNotifText with the original in-process Go map, where "days" is a
	// plain int (see worker.go) — never a float64. toInt must accept that, not just the
	// float64 shape a JSON round trip would have produced.
	got := formatNotifText("en-US", "due_soon", map[string]any{"task_id": int64(1), "title": "Ship it", "days": 3})
	want := "Deadline coming up (3d) · «Ship it»"
	if got != want {
		t.Errorf("formatNotifText = %q, want %q", got, want)
	}
}

// TestFormatNotifTextRespectsLocale is the direct regression test for the bug this session
// started from: captions (and now Telegram text) silently defaulting to English regardless of
// the recipient's own language. If this ever starts returning the English string again, the
// locale-loading wiring broke the same way stats.go's did.
func TestFormatNotifTextRespectsLocale(t *testing.T) {
	got := formatNotifText("ru-RU", "task_assigned", map[string]any{"by": "dan"})
	if got == "Task assigned to you · by @dan" {
		t.Fatal("formatNotifText returned English text for a ru-RU recipient — locale is not being applied")
	}
	want := "Вам назначена задача · от @dan"
	if got != want {
		t.Errorf("formatNotifText(ru-RU) = %q, want %q", got, want)
	}
}

func TestFormatNotifTextFallsBackToEnglishForUnknownLocale(t *testing.T) {
	// Defensive only: normalizeLocale should never actually hand formatNotifText something
	// outside allLocales, but the formatter itself shouldn't crash or return a raw key either.
	got := formatNotifText("xx-XX", "task_assigned", nil)
	want := "Task assigned to you"
	if got != want {
		t.Errorf("formatNotifText(xx-XX) = %q, want %q (en-US fallback)", got, want)
	}
}

func TestNotificationItemAddsLocalizedTextAndTaskID(t *testing.T) {
	got := notificationItem("ru-RU", 7, "task_assigned", json.RawMessage(`{"task_id":42,"title":"Выпустить релиз","by":"dan"}`), nil, "now")
	if got["text"] != "Вам назначена задача · «Выпустить релиз» · от @dan" {
		t.Errorf("notificationItem text = %q", got["text"])
	}
	if got["task_id"] != 42 {
		t.Errorf("notificationItem task_id = %#v, want 42", got["task_id"])
	}
	payload, ok := got["payload"].(map[string]any)
	if !ok || payload["title"] != "Выпустить релиз" {
		t.Errorf("notificationItem payload = %#v", got["payload"])
	}
}

func TestNotificationItemSurvivesMalformedPayload(t *testing.T) {
	got := notificationItem("ru-RU", 8, "approved", json.RawMessage(`{not-json`), nil, nil)
	if got["text"] != "Аккаунт одобрен" {
		t.Errorf("notificationItem malformed payload text = %q, want localized kind", got["text"])
	}
	if _, ok := got["task_id"]; ok {
		t.Errorf("notificationItem unexpectedly added task_id: %#v", got["task_id"])
	}
}

func TestNotificationItemUnknownKindIsStillVisible(t *testing.T) {
	got := notificationItem("ru-RU", 9, "future_event", json.RawMessage(`{}`), nil, nil)
	if got["text"] != "notif.kind.future_event" {
		t.Errorf("notificationItem unknown kind text = %q, want visible fallback key", got["text"])
	}
}

func TestNormalizeLocaleStripsInformalOverlay(t *testing.T) {
	a := &API{}
	if got := a.normalizeLocale("ru-RU-it"); got != "ru-RU" {
		t.Errorf("normalizeLocale(ru-RU-it) = %q, want ru-RU", got)
	}
	if got := a.normalizeLocale(""); got != "en-US" {
		t.Errorf("normalizeLocale(\"\") = %q, want en-US (Cfg.DefaultLocale's zero value)", got)
	}
	if got := a.normalizeLocale("not-a-real-locale"); got != "en-US" {
		t.Errorf("normalizeLocale(garbage) = %q, want en-US", got)
	}
}
