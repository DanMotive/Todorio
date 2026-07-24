package api

// Root-only web UI for server settings: reads/writes the same system_settings table (and the
// same keys) as `todorio server config|policy|limits|branding set` — single source of truth
// between the terminal and the root panel, per spec section 10.

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type settingDef struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Type    string   `json:"type"` // text | number | bool | select
	Default string   `json:"default"`
	Options []string `json:"options,omitempty"`
}

var knownSettings = []settingDef{
	{Key: "branding.site_name", Label: "Site name", Type: "text", Default: "Todorio"},
	{Key: "branding.browser_title", Label: "Browser tab title", Type: "text", Default: "Todorio"},
	{Key: "branding.developer_name", Label: "Developer credit", Type: "text", Default: "DanMotive"},
	{Key: "branding.footer_text", Label: "Footer text", Type: "text", Default: ""},
	{Key: "branding.default_color", Label: "Default accent color", Type: "select", Default: "blue",
		Options: []string{"red", "blue", "green", "yellow", "gray"}},
	{Key: "branding.default_scheme", Label: "Default light/dark", Type: "select", Default: "dark",
		Options: []string{"light", "dark"}},
	{Key: "branding.default_visual", Label: "Default visual mode", Type: "select", Default: "rich",
		Options: []string{"rich", "lite"}},
	{Key: "policy.registration.mode", Label: "Registration mode", Type: "select", Default: "open_approval",
		Options: []string{"open_approval", "invite_only", "closed"}},
	{Key: "policy.users.can_create_spaces", Label: "Users can create spaces", Type: "bool", Default: "true"},
	{Key: "policy.users.can_invite", Label: "Users can create invite codes", Type: "bool", Default: "false"},
	{Key: "policy.sharing.public_links", Label: "Public read-only links allowed", Type: "bool", Default: "true"},
	{Key: "policy.archive.retention_days", Label: "Archive auto-cleanup (days)", Type: "number", Default: "30"},
	{Key: "limits.uploads.max_file_size_mb", Label: "Max attachment size (MB)", Type: "number", Default: "10"},
	{Key: "limits.login.max_attempts", Label: "Max failed login attempts (10 min window)", Type: "number", Default: "10"},
	{Key: "limits.content.comment_max_len", Label: "Max comment length (characters)", Type: "number", Default: "4000"},
	{Key: "onboarding.demo", Label: "Offer the demo space during setup", Type: "select", Default: "on",
		Options: []string{"on", "off"}},
	{Key: "onboarding.quests", Label: "Onboarding quests for new users", Type: "select", Default: "on",
		Options: []string{"on", "off"}},
	{Key: "pulse.enabled", Label: "Space Pulse enabled", Type: "bool", Default: "true"},
}

var settingKeys = func() map[string]bool {
	m := map[string]bool{}
	for _, s := range knownSettings {
		m[s.Key] = true
	}
	return m
}()

// allLocales mirrors web/src/i18n.ts's SUPPORTED list — kept here too so the server can validate
// enable/disable requests without trusting arbitrary client input.
var allLocales = []string{
	"en-US", "ru-RU", "uk-UA", "be-BY", "kk-KZ",
	"es-ES", "pt-BR", "tr-TR",
	"zh-CN", "hi-IN", "bn-BD", "ja-JP", "ko-KR",
}

// GET /api/admin/settings — root only. Known settings with their current values, plus which
// locales are enabled (an empty stored list means "all of them", the out-of-the-box default).
func (a *API) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if u.Role != "root" {
		errJSON(w, http.StatusForbidden, "root only")
		return
	}
	out := make([]map[string]any, 0, len(knownSettings))
	for _, s := range knownSettings {
		out = append(out, map[string]any{
			"key": s.Key, "label": s.Label, "type": s.Type, "default": s.Default,
			"options": s.Options, "value": a.DB.Setting(r.Context(), s.Key, s.Default),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"settings":        out,
		"all_locales":     allLocales,
		"locales_enabled": a.enabledLocales(r),
	})
}

// POST /api/admin/settings {key, value} — root only; rejects unknown keys so the settings
// table can't be used to stash arbitrary data.
func (a *API) handleSetSetting(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if u.Role != "root" {
		errJSON(w, http.StatusForbidden, "root only")
		return
	}
	var in struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := readJSON(r, &in); err != nil || !settingKeys[in.Key] {
		errJSON(w, http.StatusBadRequest, "unknown setting key")
		return
	}
	if !validSettingValue(in.Key, in.Value) {
		errJSON(w, http.StatusBadRequest, "invalid value for "+in.Key)
		return
	}
	b, _ := json.Marshal(in.Value)
	if err := a.DB.SetSetting(r.Context(), in.Key, string(b)); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// validSettingValue checks a proposed value against its setting's declared type, so a typo (e.g.
// "closd" instead of "closed" for a security-relevant policy) fails loudly instead of silently
// falling through to whatever default behavior an unrecognized value happens to produce.
func validSettingValue(key, value string) bool {
	var def *settingDef
	for i := range knownSettings {
		if knownSettings[i].Key == key {
			def = &knownSettings[i]
			break
		}
	}
	if def == nil {
		return false
	}
	switch def.Type {
	case "bool":
		return value == "true" || value == "false"
	case "number":
		n, err := strconv.Atoi(value)
		return err == nil && n >= 0
	case "select":
		for _, o := range def.Options {
			if o == value {
				return true
			}
		}
		return false
	default: // "text" — anything goes (site name, footer text, ...)
		return true
	}
}

// POST /api/admin/locales {locale, enabled} — root only; toggles one locale on or off.
func (a *API) handleSetLocale(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if u.Role != "root" {
		errJSON(w, http.StatusForbidden, "root only")
		return
	}
	var in struct {
		Locale  string `json:"locale"`
		Enabled bool   `json:"enabled"`
	}
	valid := false
	if err := readJSON(r, &in); err == nil {
		for _, l := range allLocales {
			if l == in.Locale {
				valid = true
				break
			}
		}
	}
	if !valid {
		errJSON(w, http.StatusBadRequest, "unknown locale")
		return
	}
	list := a.enabledLocales(r)
	out := make([]string, 0, len(list)+1)
	found := false
	for _, l := range list {
		if l == in.Locale {
			found = true
			if !in.Enabled {
				continue
			}
		}
		out = append(out, l)
	}
	if in.Enabled && !found {
		out = append(out, in.Locale)
	}
	b, _ := json.Marshal(out)
	if err := a.DB.SetSetting(r.Context(), "locales.enabled", string(b)); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// enabledLocales reads locales.enabled (a JSON array). An empty/missing setting means "all
// supported locales" — this list only tracks explicit admin restrictions.
func (a *API) enabledLocales(r *http.Request) []string {
	raw := a.DB.Setting(r.Context(), "locales.enabled", "[]")
	var list []string
	if json.Unmarshal([]byte(raw), &list) != nil || len(list) == 0 {
		return append([]string{}, allLocales...)
	}
	return list
}
