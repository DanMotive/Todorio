// Package auth: sessions (HttpOnly+Secure+SameSite cookie) and middleware.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/DanMotive/Todorio/internal/db"
)

const CookieName = "todorio_session"
const sessionTTL = 30 * 24 * time.Hour

// sessionTouchAfter is how stale last_used_at has to be before a request writes to the
// sessions row. Recording every request would add a write to every single API call for no
// benefit; five minutes is precise enough for sliding expiry and for an "active sessions"
// listing, and it means the update statement below matches nothing on almost every request.
const sessionTouchAfter = 5 * time.Minute

// sessionRenewWithin extends a session that is being used and is within this much of expiring.
// A session in daily use no longer drops its owner at the 30-day mark, while one that has gone
// quiet still expires exactly on schedule.
const sessionRenewWithin = 5 * 24 * time.Hour

type User struct {
	ID                 int64  `json:"id"`
	Username           string `json:"username"`
	Role               string `json:"role"`   // root | admin | user | viewer
	Status             string `json:"status"` // pending | active | blocked | rejected
	MustChangePassword bool   `json:"must_change_password"`
}

func (u *User) IsAdmin() bool { return u.Role == "root" || u.Role == "admin" }

// IsViewer reports the workspace-wide read-only role.
//
// This is separate from the per-list "viewer" permission: the global role is a ceiling that
// applies everywhere, including lists the user was explicitly granted "editor" on. Enforcement
// lives in listPermission and spaceRole, which cap whatever the membership tables say.
func (u *User) IsViewer() bool { return u.Role == "viewer" }

type ctxKey struct{}

// sessionCookie builds the session cookie.
//
// Clearing a cookie has to repeat every attribute it was set with: a browser only replaces a
// cookie when the name, path and domain match, so a deletion written with a different Path or
// SameSite can leave the original in place — the user is told they have been signed out while
// the cookie is still on disk. Having one builder for both directions makes that impossible.
//
// SameSite stays Lax rather than Strict on purpose. Strict would drop the cookie on any
// top-level navigation that starts elsewhere, which is exactly what a task link in a Telegram
// notification is, and every such link would land on the login screen. Cross-site request
// forgery is handled instead by the Origin check in the server middleware, which does not have
// that side effect.
func sessionCookie(value string, maxAge int, secure bool) *http.Cookie {
	return &http.Cookie{
		Name: CookieName, Value: value, Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
		MaxAge: maxAge,
	}
}

// SecureRequest reports whether the session cookie should carry the Secure attribute.
//
// cfgHTTPS covers the binary terminating TLS itself and r.TLS covers the same case at request
// level; X-Forwarded-Proto covers the far more common deployment where nginx or Caddy
// terminates TLS and forwards plain HTTP to localhost. Keying Secure off the config flag alone
// meant every proxied instance — i.e. most of them — handed out a cookie that a downgrade to
// http:// could strip and read.
//
// A forged X-Forwarded-Proto only ever *adds* the Secure flag, which fails safe: the worst an
// attacker achieves is a cookie their victim's browser refuses to send over plain HTTP.
func SecureRequest(r *http.Request, cfgHTTPS bool) bool {
	if cfgHTTPS || r.TLS != nil {
		return true
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if i := strings.IndexByte(proto, ','); i >= 0 {
		proto = proto[:i] // "https, http" — the client-facing hop is first
	}
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

func CreateSession(ctx context.Context, d *db.DB, w http.ResponseWriter, userID int64, userAgent string, secure bool) error {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	id := hex.EncodeToString(b)
	if _, err := d.Pool.Exec(ctx, `INSERT INTO sessions(id,user_id,expires_at,user_agent) VALUES($1,$2,$3,$4)`,
		id, userID, time.Now().Add(sessionTTL), userAgent); err != nil {
		return err
	}
	http.SetCookie(w, sessionCookie(id, int(sessionTTL.Seconds()), secure))
	return nil
}

func DestroySession(ctx context.Context, d *db.DB, w http.ResponseWriter, r *http.Request, secure bool) {
	if c, err := r.Cookie(CookieName); err == nil {
		_, _ = d.Pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1`, c.Value)
	}
	http.SetCookie(w, sessionCookie("", -1, secure))
}

// CurrentSessionID returns the session id carried by this request, or "".
// Used by the paths that need to revoke every session *except* the one making the call.
func CurrentSessionID(r *http.Request) string {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// DeleteOtherSessions revokes every session of a user except keepID (pass "" to revoke all).
//
// This is what makes a password change mean something. Changing a password because it may have
// leaked is pointless if whoever has the old one is already holding a valid 30-day cookie:
// their session simply carries on. Signing the other devices out closes that window, while
// keeping the current session means the user is not logged out of the tab they are typing in.
func DeleteOtherSessions(ctx context.Context, d *db.DB, userID int64, keepID string) error {
	_, err := d.Pool.Exec(ctx,
		`DELETE FROM sessions WHERE user_id=$1 AND ($2 = '' OR id <> $2)`, userID, keepID)
	return err
}

// Middleware puts the current user into the context (if the session is valid).
func Middleware(d *db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie(CookieName); err == nil {
				var u User
				err := d.Pool.QueryRow(r.Context(), `
					SELECT u.id, u.username, u.role, u.status, u.must_change_password
					FROM sessions s JOIN users u ON u.id = s.user_id
					WHERE s.id=$1 AND s.expires_at > now() AND u.archived_at IS NULL`,
					c.Value).Scan(&u.ID, &u.Username, &u.Role, &u.Status, &u.MustChangePassword)
				if err == nil {
					// Record the return visit after a pause of ≥6 hours — for the "while you were away" digest.
					_, _ = d.Pool.Exec(r.Context(), `
						UPDATE users SET
							prev_seen_at = CASE
								WHEN last_seen_at IS NOT NULL AND last_seen_at < now() - interval '6 hours'
								THEN last_seen_at ELSE prev_seen_at END,
							last_seen_at = now()
						WHERE id=$1`, u.ID)
					touchSession(r.Context(), d, c.Value)
					r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, &u))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// touchSession records activity and slides the expiry forward.
//
// The last_used_at guard keeps this to at most one write per session per sessionTouchAfter;
// the rest of the time the statement matches no rows. Note that the cookie's own MaxAge is not
// refreshed here — the browser keeps the original 30-day cookie and the server-side row is the
// authority, so a renewed session simply keeps working.
func touchSession(ctx context.Context, d *db.DB, id string) {
	// make_interval(secs => ...) rather than a string cast: Go renders a Duration as
	// "120h0m0s", which Postgres does not accept as an interval literal.
	_, _ = d.Pool.Exec(ctx, `
		UPDATE sessions SET
			last_used_at = now(),
			expires_at = CASE
				WHEN expires_at < now() + make_interval(secs => $2) THEN now() + make_interval(secs => $3)
				ELSE expires_at END
		WHERE id=$1 AND last_used_at < now() - make_interval(secs => $4)`,
		id,
		sessionRenewWithin.Seconds(),
		sessionTTL.Seconds(),
		sessionTouchAfter.Seconds(),
	)
}

func FromContext(ctx context.Context) *User {
	u, _ := ctx.Value(ctxKey{}).(*User)
	return u
}
