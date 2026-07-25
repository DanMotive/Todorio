package api

// Personal Telegram bots.
//
// Until now Telegram delivery worked only if root pasted a bot token into the server settings.
// On a self-hosted instance that is one person's decision for everyone: if root never does it,
// nobody gets notifications, and asking root to run a bot on behalf of the whole team is exactly
// the kind of shared dependency this project otherwise avoids. So any user may now bring their
// own token from @BotFather and be notified through their own bot, with no server-wide setup at
// all. If both exist, the personal bot wins - it is the more specific choice, and the user made
// it themselves.
//
// Storage note: the token sits in users.telegram_bot_token in plain text, the same way the
// server-wide token already sits in system_settings. There is no key management in this app to
// encrypt it with that would not just move the problem to another plaintext secret on the same
// disk. What is enforced instead is that the token is never read back to the browser, never
// written to the audit log, and never included in an export.

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/DanMotive/Todorio/internal/telegram"
)

// personalLinkWait bounds the confirm call. The user is watching a spinner, so this cannot be
// long; it only has to cover "press the button in Telegram now", and a /start sent before the
// call is picked up from the backlog anyway.
const personalLinkWait = 30 * time.Second

// personalBotToken returns the caller's own bot token, or "" if they have not set one.
func (a *API) personalBotToken(ctx context.Context, userID int64) string {
	var token string
	if a.DB.Pool.QueryRow(ctx,
		`SELECT telegram_bot_token FROM users WHERE id=$1`, userID).Scan(&token) != nil {
		return ""
	}
	return token
}

// personalBotUsername returns the @name of the caller's own bot, or "" if there is none. Unlike
// the token, the username is safe to show: it is public the moment the bot exists.
func (a *API) personalBotUsername(ctx context.Context, userID int64) string {
	var name string
	if a.DB.Pool.QueryRow(ctx,
		`SELECT telegram_bot_username FROM users WHERE id=$1`, userID).Scan(&name) != nil {
		return ""
	}
	return name
}

// looksLikeBotToken rejects the obvious mistakes (an empty box, a pasted deep link, a whole
// message copied out of BotFather) before a network call is made. It deliberately does not try
// to be a full validator - Telegram itself is the authority on whether a token works, and
// getMe is called right after.
func looksLikeBotToken(token string) bool {
	if len(token) < 20 || len(token) > 200 {
		return false
	}
	if strings.ContainsAny(token, " \t\n\r") {
		return false
	}
	id, rest, found := strings.Cut(token, ":")
	if !found || id == "" || len(rest) < 10 {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// POST /api/me/telegram/bot {token}
//
// Saves the token after checking it against Telegram, and hands back the deep link the user
// should open. Nothing is linked yet at this point: the token proves the bot exists, not that
// this person can talk to it.
func (a *API) handleSetPersonalBot(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	token := strings.TrimSpace(in.Token)
	if !looksLikeBotToken(token) {
		errJSON(w, http.StatusBadRequest, "that does not look like a bot token - it should look like 123456789:AA... , exactly as BotFather sent it")
		return
	}

	// Verify before storing, so a typo is caught while the user is still looking at the field
	// rather than silently producing an account that never receives anything.
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	botName, err := telegram.GetMe(ctx, token)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "Telegram rejected that token")
		return
	}

	code, err := telegram.NewLinkCode()
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "could not generate a link code")
		return
	}
	// Saving a new token drops any chat id linked through the previous bot: that chat belongs to
	// the old bot and messages sent from the new one would never arrive there.
	if _, err := a.DB.Pool.Exec(r.Context(), `
		UPDATE users
		SET telegram_bot_token=$2, telegram_bot_username=$3,
		    telegram_chat_id=NULL, telegram_link_code=$4, telegram_link_code_at=now()
		WHERE id=$1`, u.ID, token, botName, code); err != nil {
		dbFail(r, "personal bot save", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"bot_username": botName,
		"deep_link":    "https://t.me/" + botName + "?start=" + code,
		"code":         code,
	})
}

// POST /api/me/telegram/bot/confirm
//
// Listens to the user's own bot for their /start and records the chat id.
//
// The server-wide bot has a background loop for this; a personal bot cannot join it, because
// each token is a separate bot with its own update queue. Rather than run a permanent poller per
// user - a long-lived connection each, kept open forever for bots that will only ever be written
// to - the listening happens here, once, while the user is actively linking. After that,
// delivery is outbound only and needs no listener at all.
func (a *API) handleConfirmPersonalBot(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	var token, code string
	var codeAt *time.Time
	if a.DB.Pool.QueryRow(r.Context(), `
		SELECT telegram_bot_token, COALESCE(telegram_link_code, ''), telegram_link_code_at
		FROM users WHERE id=$1`, u.ID).Scan(&token, &code, &codeAt) != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if token == "" {
		errJSON(w, http.StatusBadRequest, "no personal bot token saved")
		return
	}
	// The same 15-minute window the server-wide flow uses: a code left lying around in a browser
	// tab overnight should not still be usable.
	if code == "" || codeAt == nil || time.Since(*codeAt) > 15*time.Minute {
		errJSON(w, http.StatusBadRequest, "the link code has expired - save the token again to get a new one")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), personalLinkWait)
	defer cancel()
	chatID, err := telegram.WaitForStart(ctx, token, code)
	if err != nil {
		// Not an error condition worth a 500: by far the most likely cause is that the user has
		// not pressed Start yet. The client can simply call again.
		writeJSON(w, http.StatusOK, map[string]any{"linked": false})
		return
	}
	if _, err := a.DB.Pool.Exec(r.Context(), `
		UPDATE users SET telegram_chat_id=$2, telegram_link_code=NULL, telegram_link_code_at=NULL
		WHERE id=$1`, u.ID, chatID); err != nil {
		dbFail(r, "personal bot confirm", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}

	// A first message doubles as proof to the user that delivery works end to end, before they
	// go and wait for a real notification that may not come for hours.
	go func(token string, chatID int64) {
		sendCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = telegram.SendMessage(sendCtx, token, chatID, "Todorio: notifications are connected.")
	}(token, chatID)

	writeJSON(w, http.StatusOK, map[string]any{"linked": true})
}

// DELETE /api/me/telegram/bot - forget the personal bot entirely.
//
// The chat id goes with it: it was a chat with that bot, and leaving it behind would mean the
// server-wide bot inherits a chat id that is not its own and silently fails on every send.
func (a *API) handleDeletePersonalBot(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if _, err := a.DB.Pool.Exec(r.Context(), `
		UPDATE users
		SET telegram_bot_token='', telegram_bot_username='',
		    telegram_chat_id=NULL, telegram_link_code=NULL, telegram_link_code_at=NULL
		WHERE id=$1`, u.ID); err != nil {
		dbFail(r, "personal bot delete", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
