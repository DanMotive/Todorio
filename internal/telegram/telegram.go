// Package telegram: optional Telegram notification delivery.
//
// Root supplies their own bot token (from @BotFather) — "дать сайту ключ, он будет сам
// сообщения слать". This keeps the feature consistent with the project's no-required-
// external-services stance: Telegram itself is external, but nothing about the app depends on
// it, nothing is configured out of the box, and the whole thing is inert until root pastes in a
// token and a user explicitly links their own account.
//
// Receiving the /start linking message uses long-polling (getUpdates), not a webhook. A webhook
// would require the instance be reachable over HTTPS with a certificate Telegram itself accepts
// — self-signed (the setup wizard's own default) is not one, and plenty of self-hosted installs
// sit behind a firewall/VPN with no inbound access at all. Long-polling only ever makes outbound
// HTTPS requests, which every deployment already needs just to be a working web app.
package telegram

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/DanMotive/Todorio/internal/db"
)

// apiBase is a var (not const) so tests can point it at an httptest.Server instead of the real
// Telegram API.
var apiBase = "https://api.telegram.org/bot"

// httpClient has a timeout comfortably longer than the getUpdates long-poll window itself, so
// the client never cuts the connection before Telegram's own timeout would have returned an
// empty result — that would look identical to a network failure and trigger a pointless retry.
var httpClient = &http.Client{Timeout: 40 * time.Second}

type apiResponse[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result"`
	Description string `json:"description"`
}

// getCall is for read-only, always-short calls (getMe, getUpdates): parameters go on the query
// string, kept as a GET so it's trivially cacheable/loggable and never confused with a mutation.
func getCall[T any](ctx context.Context, token, method string, query url.Values) (T, error) {
	u := apiBase + token + "/" + method
	if query != nil {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		var zero T
		return zero, err
	}
	return doCall[T](req)
}

// postCall is for sendMessage: a form body has no practical length ceiling the way a URL's
// query string does, which matters here because an announcement's free-form admin-written body
// (formatNotifText in internal/api) can run to a few paragraphs — long enough to risk tripping a
// proxy's URL-length limit if it rode along as a query parameter instead.
func postCall[T any](ctx context.Context, token, method string, form url.Values) (T, error) {
	u := apiBase + token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		var zero T
		return zero, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return doCall[T](req)
}

func doCall[T any](req *http.Request) (T, error) {
	var zero T
	resp, err := httpClient.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, err
	}
	var out apiResponse[T]
	if err := json.Unmarshal(body, &out); err != nil {
		return zero, fmt.Errorf("decoding Telegram response: %w (body: %s)", err, truncate(body, 200))
	}
	if !out.OK {
		return zero, fmt.Errorf("Telegram API error: %s", out.Description)
	}
	return out.Result, nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

type meResult struct {
	Username string `json:"username"`
}

// GetMe validates a bot token and returns the bot's @username, used to build the
// https://t.me/<username>?start=<code> deep link and to catch a pasted-wrong token immediately
// (at settings-save time) rather than leaving root to discover it's broken later, silently.
func GetMe(ctx context.Context, token string) (string, error) {
	res, err := getCall[meResult](ctx, token, "getMe", nil)
	if err != nil {
		return "", err
	}
	if res.Username == "" {
		return "", fmt.Errorf("Telegram accepted the token but returned no bot username")
	}
	return res.Username, nil
}

// SendMessage delivers one notification. Called from a detached goroutine (see api.go's
// notify()) so a slow or failing Telegram API call never adds latency to the request that
// triggered it; errors are logged, not surfaced anywhere a user would see them — a missed
// Telegram ping is not worth failing the action that caused it.
func SendMessage(ctx context.Context, token string, chatID int64, text string) error {
	form := url.Values{
		"chat_id": {strconv.FormatInt(chatID, 10)},
		"text":    {text},
	}
	_, err := postCall[json.RawMessage](ctx, token, "sendMessage", form)
	return err
}

// NewLinkCode generates a one-time code for the /start deep link. Hex of 12 random bytes: 24
// characters, all in Telegram's allowed start-parameter charset (A-Za-z0-9_-), well under its
// 64-character limit.
func NewLinkCode() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

type update struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

// Run polls for incoming messages for as long as ctx is alive. It re-reads the bot token from
// the database on every iteration (not just once at startup) so saving, changing, or clearing
// the token in the root settings panel takes effect without a server restart — the loop simply
// idles (checking every 20s) whenever no token is configured, and starts polling for real the
// moment one appears.
func Run(ctx context.Context, d *db.DB) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		token := d.Setting(ctx, "telegram.bot_token", "")
		if token == "" {
			sleep(ctx, 20*time.Second)
			continue
		}
		if err := pollOnce(ctx, d, token); err != nil {
			log.Printf("telegram: poll: %v", err)
			sleep(ctx, 5*time.Second) // back off on error, but keep trying — a blip shouldn't need a restart
		}
	}
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// pollOnce blocks in getUpdates for up to ~25s (Telegram's own long-poll window) and processes
// whatever comes back. The offset is persisted in system_settings so a server restart resumes
// where it left off instead of replaying (or permanently skipping) old /start messages.
func pollOnce(ctx context.Context, d *db.DB, token string) error {
	offsetStr := d.Setting(ctx, "telegram.last_update_id", "0")
	offset, _ := strconv.ParseInt(offsetStr, 10, 64)

	q := url.Values{
		"timeout": {"25"},
		"offset":  {strconv.FormatInt(offset, 10)},
	}
	updates, err := getCall[[]update](ctx, token, "getUpdates", q)
	if err != nil {
		return err
	}
	for _, up := range updates {
		if up.UpdateID >= offset {
			offset = up.UpdateID + 1
		}
		processUpdate(ctx, d, up)
	}
	if len(updates) > 0 {
		_ = d.SetSetting(ctx, "telegram.last_update_id", strconv.FormatInt(offset, 10))
	}
	return nil
}

// processUpdate looks for "/start <code>" (the deep link's payload) and, if the code matches a
// pending link on some account, records that chat id against it. Anything else — a bare
// "/start", small talk, whatever — is silently ignored; this bot has no other commands.
// parseStartCode extracts the deep link's code from a "/start <code>" message, the only command
// this bot understands. A bare "/start" (no code — someone opened the bot without a deep link)
// and anything not starting with "/start" at all both report ok=false; there's nothing to do
// with either.
func parseStartCode(text string) (code string, ok bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/start") {
		return "", false
	}
	code = strings.TrimSpace(strings.TrimPrefix(text, "/start"))
	return code, code != ""
}

func processUpdate(ctx context.Context, d *db.DB, up update) {
	if up.Message == nil {
		return
	}
	code, ok := parseStartCode(up.Message.Text)
	if !ok {
		return
	}
	// The code is single-use: this also clears it, so a leaked/guessed code can't be replayed
	// against the same pending link after the fact, and a stale code from an abandoned link
	// attempt can't later collide with a fresh one for a different account.
	_, err := d.Pool.Exec(ctx, `
		UPDATE users SET telegram_chat_id=$2, telegram_link_code=NULL, telegram_link_code_at=NULL
		WHERE telegram_link_code=$1
		  AND telegram_link_code_at > now() - interval '15 minutes'`,
		code, up.Message.Chat.ID)
	if err != nil {
		log.Printf("telegram: linking chat %d: %v", up.Message.Chat.ID, err)
	}
}

// WaitForStart watches one specific bot for a "/start <code>" message and returns the chat id it
// came from.
//
// This exists for personal bots: a user pastes their own token, and their own bot has to be
// polled to catch their /start. Run() above cannot do it - it polls the single server-wide token,
// and each token is a different bot with a different update queue.
//
// Deliberately short-lived and caller-driven rather than a goroutine per user. Once a chat id is
// known, delivery is pure outbound sendMessage, so a personal bot needs listening exactly once:
// during linking. Keeping a permanent poller per user would mean one long-lived HTTPS connection
// for every account that ever tried this, most of them for bots that are never messaged again -
// paid forever to receive nothing. The caller supplies the deadline via ctx.
//
// The offset starts at 0 so a /start that arrived before this call (the user pressing Start in
// Telegram first, then clicking confirm in the browser) is still found in the backlog.
func WaitForStart(ctx context.Context, token, code string) (int64, error) {
	var offset int64
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		q := url.Values{
			// Short poll window: the request has to come back often enough to notice ctx being
			// cancelled and to stay well inside the HTTP client's own 40s timeout.
			"timeout": {"10"},
			"offset":  {strconv.FormatInt(offset, 10)},
		}
		updates, err := getCall[[]update](ctx, token, "getUpdates", q)
		if err != nil {
			// A cancelled context surfaces here as a transport error; report the context's reason
			// instead, so the caller can tell "user never pressed start" from "the token is bad".
			if ctxErr := ctx.Err(); ctxErr != nil {
				return 0, ctxErr
			}
			return 0, err
		}
		for _, up := range updates {
			if up.UpdateID >= offset {
				offset = up.UpdateID + 1
			}
			if up.Message == nil {
				continue
			}
			got, ok := parseStartCode(up.Message.Text)
			if ok && got == code {
				return up.Message.Chat.ID, nil
			}
		}
	}
}
