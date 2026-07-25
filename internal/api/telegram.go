package api

// Telegram notification linking (spec follow-up, not in the original ТЗ): root configures a bot
// token once (settings.go handles that side, including validating it against Telegram's own
// getMe); each user who wants delivery links their own account here. See internal/telegram for
// the bot API client and the long-poll loop that actually captures the /start message, and
// notif_text.go + api.go's notify() for how a linked account actually receives messages.

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/DanMotive/Todorio/internal/telegram"
)

// GET /api/telegram/status — whether the feature is even configured, and whether the caller has
// linked their own account. Deliberately two different booleans: "not configured" (root hasn't
// set a token) and "not linked" (you haven't connected yet) call for different UI messages.
func (a *API) handleTelegramStatus(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	token := a.DB.Setting(r.Context(), "telegram.bot_token", "")
	if token == "" {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "linked": false})
		return
	}
	var linked bool
	_ = a.DB.Pool.QueryRow(r.Context(),
		`SELECT telegram_chat_id IS NOT NULL FROM users WHERE id=$1`, u.ID).Scan(&linked)
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "linked": linked})
}

// POST /api/telegram/link — issues a fresh one-time code and the deep link to hand it to the
// bot. Calling this again before the user finishes simply replaces the pending code (the old one
// stops working) rather than accumulating stale ones.
func (a *API) handleTelegramLink(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	token := a.DB.Setting(r.Context(), "telegram.bot_token", "")
	if token == "" {
		errJSON(w, http.StatusBadRequest, "Telegram is not configured on this server")
		return
	}
	botUsername := a.DB.Setting(r.Context(), "telegram.bot_username", "")
	if botUsername == "" {
		errJSON(w, http.StatusInternalServerError, "the bot token is set but its username could not be resolved — ask an administrator to re-save it")
		return
	}
	code, err := telegram.NewLinkCode()
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "could not generate a link code")
		return
	}
	if _, err := a.DB.Pool.Exec(r.Context(),
		`UPDATE users SET telegram_link_code=$2, telegram_link_code_at=now() WHERE id=$1`,
		u.ID, code); err != nil {
		dbFail(r, "telegram link", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code":         code,
		"bot_username": botUsername,
		"deep_link":    "https://t.me/" + botUsername + "?start=" + code,
	})
}

// POST /api/telegram/unlink — forgets the chat id. Purely local: there's no API call that
// revokes anything on Telegram's side, nor does it need one — the bot simply stops being told
// to message this chat.
func (a *API) handleTelegramUnlink(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if _, err := a.DB.Pool.Exec(r.Context(),
		`UPDATE users SET telegram_chat_id=NULL, telegram_link_code=NULL, telegram_link_code_at=NULL WHERE id=$1`,
		u.ID); err != nil {
		dbFail(r, "telegram unlink", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// sendTelegram fires a Telegram DM for a freshly-created (non-collapsed) notification — see the
// call site in notify(), which only reaches here on the branch that actually inserts a new row,
// not the one that refreshes an already-collapsed one. A burst of edits to the same task
// collapses to one bell entry; it should also only ever ping the phone once.
//
// Runs detached from the request: r.Context() is cancelled the instant the HTTP response is
// written, long before a Telegram API round trip would finish, so the outbound call gets its own
// short-lived context instead of inheriting one that's already ending.
func (a *API) sendTelegram(ctx context.Context, userID int64, kind string, payload map[string]any) {
	token := a.DB.Setting(ctx, "telegram.bot_token", "")
	if token == "" {
		return
	}
	chatID, locale := a.telegramTarget(ctx, userID)
	if chatID == 0 {
		return
	}
	text := formatNotifText(locale, kind, payload)
	go func() {
		sendCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := telegram.SendMessage(sendCtx, token, chatID, text); err != nil {
			log.Printf("telegram: sending to user %d: %v", userID, err)
		}
	}()
}

// telegramTarget resolves the chat to deliver to (0 means "don't send": never linked, or the
// user turned Telegram delivery off) and the locale to render the message in, in one query.
func (a *API) telegramTarget(ctx context.Context, userID int64) (int64, string) {
	var chatID *int64
	var prefsRaw, rawLocale *string
	err := a.DB.Pool.QueryRow(ctx,
		`SELECT telegram_chat_id, notify_prefs #>> '{telegram}', locale FROM users WHERE id=$1`, userID).
		Scan(&chatID, &prefsRaw, &rawLocale)
	if err != nil || chatID == nil {
		return 0, "en-US"
	}
	if prefsRaw != nil && *prefsRaw == "false" {
		return 0, "en-US"
	}
	loc := ""
	if rawLocale != nil {
		loc = *rawLocale
	}
	return *chatID, a.normalizeLocale(loc)
}
