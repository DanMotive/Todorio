package api

// Outgoing webhooks: when something happens in a space, POST it to a URL the owner registered.
//
// Delivery hangs off the event bus rather than off the handlers. The bus was built for SSE, so
// its Publish call already runs at every point worth reporting — task created, task updated,
// comment created — and events.Tap lets this file observe all of them without a single edit to
// tasks.go, social.go or anything else that publishes. The alternative was a dispatcher call at
// every one of those sites, which is a lot of churn in large files for a feature none of them
// care about.
//
// Nothing is configured out of the box and nothing runs until somebody enters a URL. That is not
// just "no rows means no deliveries": webhooksConfigured is an atomic flag, so on an install that
// never opens this screen the tap returns immediately and never touches the database, no matter
// how busy the bus is.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DanMotive/Todorio/internal/events"
)

const (
	// A receiving endpoint that has not answered in ten seconds is not going to.
	webhookDeliveryTimeout = 10 * time.Second
	// Whole-tap budget: one event may have several endpoints to visit, each with its own timeout.
	webhookTapBudget = 2 * time.Minute
	webhookMaxErrorLen = 400
	webhookMaxPerSpace = 20
	webhookMaxEvents   = 20
	// Consecutive failures before the endpoint is switched off. A receiver that has been dead for
	// this long is not coming back on its own, and hammering it forever costs a goroutine and a
	// ten-second wait on every single event in the space.
	webhookFailureLimit = 25
)

// Event names offered in the UI. Not enforced on save: the bus can grow new event types, and a
// subscription to a name this build does not know yet should lie dormant rather than be rejected.
var webhookEventTypes = []string{"task.created", "task.updated", "comment.created"}

var webhookClient = &http.Client{Timeout: webhookDeliveryTimeout}

// webhooksConfigured is the "is this feature in use at all" flag — see the package comment.
var webhooksConfigured atomic.Bool

type webhookView struct {
	ID             int64      `json:"id"`
	URL            string     `json:"url"`
	Events         []string   `json:"events"`
	IsActive       bool       `json:"is_active"`
	HasSecret      bool       `json:"has_secret"` // the secret itself is never sent back
	LastStatus     *int       `json:"last_status"`
	LastError      string     `json:"last_error"`
	LastDeliveryAt *time.Time `json:"last_delivery_at"`
	FailureCount   int        `json:"failure_count"`
	CreatedAt      time.Time  `json:"created_at"`
}

// ---------- dispatch ----------

// startWebhooks installs the bus observer. Called from Routes, which runs once at startup.
func (a *API) startWebhooks() {
	if a.Bus == nil || a.DB == nil {
		return
	}
	a.refreshWebhookFlag(context.Background())
	a.Bus.SetTap(a.webhookTap)
}

// refreshWebhookFlag re-reads whether any active webhook exists anywhere. Cheap, and only called
// at startup and after a webhook is saved, deleted or auto-disabled.
func (a *API) refreshWebhookFlag(ctx context.Context) {
	var exists bool
	// An error here (most likely the table not existing yet on a server mid-migration) leaves the
	// flag alone, which means the feature stays off. Off is the safe direction.
	if a.DB.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM webhooks WHERE is_active)`).Scan(&exists) != nil {
		return
	}
	webhooksConfigured.Store(exists)
}

func (a *API) webhookTap(_ []int64, e events.Event) {
	if !webhooksConfigured.Load() {
		return
	}
	// Personal traffic, not space activity: a notification is addressed to one person and an
	// announcement is instance-wide. Neither belongs to a space, and forwarding somebody's bell
	// to a third-party endpoint is not what registering a space webhook asks for.
	if e.Type == "notification" || e.Type == "announcement" {
		return
	}
	data, ok := e.Data.(map[string]any)
	if !ok {
		return
	}
	// The request that caused the event is long gone by the time this runs, so the delivery gets
	// its own context rather than borrowing one that is already cancelled.
	ctx, cancel := context.WithTimeout(context.Background(), webhookTapBudget)
	defer cancel()

	spaceID, ok := a.spaceForEvent(ctx, data)
	if !ok {
		return
	}
	a.deliverToSpace(ctx, spaceID, e.Type, data)
}

// eventID pulls an id out of an event payload. The maps are built in Go, so the values arrive as
// int64, but a float64 is accepted too in case a payload ever makes a round trip through JSON.
func eventID(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case float64:
		return int64(n), true
	}
	return 0, false
}

// spaceForEvent finds the space an event belongs to. Every task and comment event carries a
// list_id, a task_id, or both — anything else is not space activity and is skipped.
func (a *API) spaceForEvent(ctx context.Context, data map[string]any) (int64, bool) {
	if id, ok := eventID(data["list_id"]); ok {
		var spaceID int64
		if a.DB.Pool.QueryRow(ctx, `SELECT space_id FROM lists WHERE id=$1`, id).Scan(&spaceID) == nil {
			return spaceID, true
		}
	}
	if id, ok := eventID(data["task_id"]); ok {
		var spaceID int64
		if a.DB.Pool.QueryRow(ctx,
			`SELECT l.space_id FROM tasks t JOIN lists l ON l.id = t.list_id WHERE t.id=$1`,
			id).Scan(&spaceID) == nil {
			return spaceID, true
		}
	}
	return 0, false
}

type webhookTarget struct {
	id     int64
	url    string
	secret string
	events []string
}

func (a *API) deliverToSpace(ctx context.Context, spaceID int64, eventType string, data map[string]any) {
	rows, err := a.DB.Pool.Query(ctx,
		`SELECT id, url, secret, events FROM webhooks WHERE space_id=$1 AND is_active`, spaceID)
	if err != nil {
		// No dbFail here: there is no *http.Request behind a bus event.
		log.Printf("webhook: loading endpoints for space %d: %v", spaceID, err)
		return
	}
	// Collected before sending, so the connection goes back to the pool instead of being held
	// open for however long a stranger's server takes to answer.
	var targets []webhookTarget
	for rows.Next() {
		var t webhookTarget
		var raw []byte
		if rows.Scan(&t.id, &t.url, &t.secret, &raw) != nil {
			continue
		}
		_ = json.Unmarshal(raw, &t.events)
		targets = append(targets, t)
	}
	rows.Close()

	for _, t := range targets {
		if !webhookWants(t.events, eventType) {
			continue
		}
		body, err := json.Marshal(map[string]any{
			"event":        eventType,
			"space_id":     spaceID,
			"delivered_at": time.Now().UTC().Format(time.RFC3339),
			"data":         data,
		})
		if err != nil {
			continue
		}
		status, derr := webhookPost(ctx, t.url, t.secret, eventType, body)
		a.recordWebhookResult(ctx, t.id, status, derr)
	}
}

// webhookWants reports whether this endpoint asked for this event. An empty subscription list
// means everything: a webhook added without ticking any box should do something useful rather
// than sit there silently never firing.
func webhookWants(subscribed []string, eventType string) bool {
	if len(subscribed) == 0 {
		return true
	}
	for _, s := range subscribed {
		if s == eventType {
			return true
		}
	}
	return false
}

// webhookPost sends one delivery and reports the HTTP status (0 if the request never got an
// answer) plus an error describing what went wrong.
//
// The signature is an HMAC-SHA256 of the exact bytes sent, so the receiver can tell a real
// delivery from anyone who happens to know the URL. Without a secret the header is simply absent
// rather than being faked with something predictable.
func webhookPost(ctx context.Context, target, secret, eventType string, body []byte) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, webhookDeliveryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "Todorio")
	req.Header.Set("X-Todorio-Event", eventType)
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		req.Header.Set("X-Todorio-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	resp, err := webhookClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	// Read and discard a little of the body so the connection can be reused; the content is of
	// no interest, only the status is.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, fmt.Errorf("the endpoint answered %s", resp.Status)
	}
	return resp.StatusCode, nil
}

// recordWebhookResult stores the outcome so a dead endpoint is visible in the UI instead of
// looking healthy while nothing has arrived for a week.
func (a *API) recordWebhookResult(ctx context.Context, id int64, status int, derr error) {
	msg := ""
	if derr != nil {
		msg = derr.Error()
		if len(msg) > webhookMaxErrorLen {
			// Cutting bytes can split a multi-byte character in half, and Postgres rejects
			// invalid UTF-8 outright — which would turn "record a failure" into a second failure.
			msg = strings.ToValidUTF8(msg[:webhookMaxErrorLen], "")
		}
	}
	var statusArg any
	if status > 0 {
		statusArg = status
	}
	ok := derr == nil
	if _, err := a.DB.Pool.Exec(ctx, `
		UPDATE webhooks SET
			last_status = $2,
			last_error = $3,
			last_delivery_at = now(),
			failure_count = CASE WHEN $4 THEN 0 ELSE failure_count + 1 END,
			is_active = CASE WHEN $4 OR failure_count + 1 < $5 THEN is_active ELSE FALSE END
		WHERE id=$1`, id, statusArg, msg, ok, webhookFailureLimit); err != nil {
		log.Printf("webhook: recording result for %d: %v", id, err)
		return
	}
	if !ok {
		// That update may have switched off the last active endpoint.
		a.refreshWebhookFlag(ctx)
	}
}

// ---------- validation ----------

func validateWebhookURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("enter the address events should be sent to")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("that does not look like a URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("the address has to start with http:// or https://")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("the address has no host")
	}
	// Loopback and private addresses are deliberately allowed. The usual advice is to block them
	// against SSRF, but this is self-hosted software and the realistic target is another service
	// on the same machine or network; blocking those would reject the main use case to defend
	// against space owners, who already run the server this would be attacking.
	return parsed.String(), nil
}

// cleanEvents trims, de-duplicates and bounds a subscription list.
func cleanEvents(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, e := range in {
		e = strings.TrimSpace(e)
		if e == "" || len(e) > 64 || seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
		if len(out) >= webhookMaxEvents {
			break
		}
	}
	return out
}

// webhookOwner resolves the space behind a webhook id and checks the caller owns it.
func (a *API) webhookOwner(w http.ResponseWriter, r *http.Request) (int64, bool) {
	u := a.requireUser(w, r)
	if u == nil {
		return 0, false
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "bad webhook id")
		return 0, false
	}
	var spaceID int64
	if a.DB.Pool.QueryRow(r.Context(), `SELECT space_id FROM webhooks WHERE id=$1`, id).Scan(&spaceID) != nil {
		// Same answer as "not yours", so this cannot be used to find out which ids exist.
		errJSON(w, http.StatusForbidden, "space owner permission required")
		return 0, false
	}
	if a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) != "owner" {
		errJSON(w, http.StatusForbidden, "space owner permission required")
		return 0, false
	}
	return id, true
}

// ---------- handlers ----------

// GET /api/spaces/{id}/webhooks — owner only. A webhook can see everything that happens in the
// space, including private lists, so this is the same bar as export.
func (a *API) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) != "owner" {
		errJSON(w, http.StatusForbidden, "space owner permission required")
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT id, url, secret <> '', events, is_active, last_status, last_error,
			last_delivery_at, failure_count, created_at
		FROM webhooks WHERE space_id=$1 ORDER BY id`, spaceID)
	if err != nil {
		dbFail(r, "list webhooks", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	out := []webhookView{}
	for rows.Next() {
		var v webhookView
		var raw []byte
		var lastErr *string
		if rows.Scan(&v.ID, &v.URL, &v.HasSecret, &raw, &v.IsActive, &v.LastStatus, &lastErr,
			&v.LastDeliveryAt, &v.FailureCount, &v.CreatedAt) != nil {
			continue
		}
		_ = json.Unmarshal(raw, &v.Events)
		if v.Events == nil {
			v.Events = []string{}
		}
		if lastErr != nil {
			v.LastError = *lastErr
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"webhooks": out,
		// So the UI does not have to keep its own copy of the list in sync with the server's.
		"event_types": webhookEventTypes,
	})
}

// POST /api/spaces/{id}/webhooks
func (a *API) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) != "owner" {
		errJSON(w, http.StatusForbidden, "space owner permission required")
		return
	}
	var in struct {
		URL    string   `json:"url"`
		Secret string   `json:"secret"`
		Events []string `json:"events"`
	}
	if readJSON(r, &in) != nil {
		errJSON(w, http.StatusBadRequest, "bad request")
		return
	}
	target, verr := validateWebhookURL(in.URL)
	if verr != nil {
		errJSON(w, http.StatusBadRequest, verr.Error())
		return
	}

	var count int
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT count(*) FROM webhooks WHERE space_id=$1`, spaceID).Scan(&count) == nil &&
		count >= webhookMaxPerSpace {
		errJSON(w, http.StatusBadRequest,
			fmt.Sprintf("a space can have at most %d webhooks", webhookMaxPerSpace))
		return
	}

	evs, _ := json.Marshal(cleanEvents(in.Events))
	var id int64
	if err := a.DB.Pool.QueryRow(r.Context(), `
		INSERT INTO webhooks(space_id, url, secret, events, created_by)
		VALUES($1,$2,$3,$4::jsonb,$5) RETURNING id`,
		spaceID, target, strings.TrimSpace(in.Secret), string(evs), u.ID).Scan(&id); err != nil {
		dbFail(r, "create webhook", err)
		errJSON(w, http.StatusInternalServerError, "could not save the webhook")
		return
	}
	a.refreshWebhookFlag(r.Context())
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// PATCH /api/webhooks/{id} — every field optional; an omitted one is left alone.
func (a *API) handleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := a.webhookOwner(w, r)
	if !ok {
		return
	}
	var in struct {
		URL      *string   `json:"url"`
		Secret   *string   `json:"secret"`
		Events   *[]string `json:"events"`
		IsActive *bool     `json:"is_active"`
	}
	if readJSON(r, &in) != nil {
		errJSON(w, http.StatusBadRequest, "bad request")
		return
	}

	if in.URL != nil {
		target, verr := validateWebhookURL(*in.URL)
		if verr != nil {
			errJSON(w, http.StatusBadRequest, verr.Error())
			return
		}
		if _, err := a.DB.Pool.Exec(r.Context(), `UPDATE webhooks SET url=$2 WHERE id=$1`, id, target); err != nil {
			dbFail(r, "update webhook url", err)
			errJSON(w, http.StatusInternalServerError, "database error")
			return
		}
	}
	if in.Secret != nil {
		// An empty string clears the secret, which stops the signature header being sent. Leaving
		// the field out entirely keeps the existing one — the UI cannot show it, so it has no way
		// to send it back unchanged.
		if _, err := a.DB.Pool.Exec(r.Context(),
			`UPDATE webhooks SET secret=$2 WHERE id=$1`, id, strings.TrimSpace(*in.Secret)); err != nil {
			dbFail(r, "update webhook secret", err)
			errJSON(w, http.StatusInternalServerError, "database error")
			return
		}
	}
	if in.Events != nil {
		evs, _ := json.Marshal(cleanEvents(*in.Events))
		if _, err := a.DB.Pool.Exec(r.Context(),
			`UPDATE webhooks SET events=$2::jsonb WHERE id=$1`, id, string(evs)); err != nil {
			dbFail(r, "update webhook events", err)
			errJSON(w, http.StatusInternalServerError, "database error")
			return
		}
	}
	if in.IsActive != nil {
		// Switching one back on clears the failure history: otherwise an endpoint that was
		// auto-disabled after a long outage would be one failure away from switching off again,
		// and the stale error text would sit next to a working webhook.
		if _, err := a.DB.Pool.Exec(r.Context(), `
			UPDATE webhooks SET is_active=$2,
				failure_count = CASE WHEN $2 THEN 0 ELSE failure_count END,
				last_error = CASE WHEN $2 THEN '' ELSE last_error END
			WHERE id=$1`, id, *in.IsActive); err != nil {
			dbFail(r, "update webhook active", err)
			errJSON(w, http.StatusInternalServerError, "database error")
			return
		}
	}
	a.refreshWebhookFlag(r.Context())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /api/webhooks/{id}
func (a *API) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := a.webhookOwner(w, r)
	if !ok {
		return
	}
	if _, err := a.DB.Pool.Exec(r.Context(), `DELETE FROM webhooks WHERE id=$1`, id); err != nil {
		dbFail(r, "delete webhook", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	a.refreshWebhookFlag(r.Context())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/webhooks/{id}/test — send one delivery now and report exactly what came back.
//
// Synchronous on purpose: the entire point is to find out whether the endpoint works while the
// person who typed the URL is still looking at the screen. It ignores the subscription list, and
// it works on a switched-off endpoint too, which is how you check a fix before re-enabling it.
func (a *API) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := a.webhookOwner(w, r)
	if !ok {
		return
	}
	var target, secret string
	var spaceID int64
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT url, secret, space_id FROM webhooks WHERE id=$1`, id).Scan(&target, &secret, &spaceID) != nil {
		errJSON(w, http.StatusNotFound, "webhook not found")
		return
	}
	body, _ := json.Marshal(map[string]any{
		"event":        "webhook.test",
		"space_id":     spaceID,
		"delivered_at": time.Now().UTC().Format(time.RFC3339),
		"data":         map[string]any{"message": "Test delivery from Todorio"},
	})
	status, derr := webhookPost(r.Context(), target, secret, "webhook.test", body)
	a.recordWebhookResult(r.Context(), id, status, derr)

	res := map[string]any{"ok": derr == nil, "status": status}
	if derr != nil {
		// The real error verbatim: "connection refused" or "no such host" is the whole answer,
		// and replacing it with something tidy would throw away the one useful fact.
		res["error"] = derr.Error()
	}
	writeJSON(w, http.StatusOK, res)
}
