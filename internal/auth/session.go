// Package auth — сессии (HttpOnly+Secure+SameSite cookie) и middleware.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/DanMotive/Todorio/internal/db"
)

const CookieName = "todorio_session"
const sessionTTL = 30 * 24 * time.Hour

type User struct {
	ID                 int64  `json:"id"`
	Username           string `json:"username"`
	Role               string `json:"role"`   // root | admin | user | viewer
	Status             string `json:"status"` // pending | active | blocked | rejected
	MustChangePassword bool   `json:"must_change_password"`
}

func (u *User) IsAdmin() bool { return u.Role == "root" || u.Role == "admin" }

type ctxKey struct{}

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
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: id, Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(sessionTTL.Seconds()),
	})
	return nil
}

func DestroySession(ctx context.Context, d *db.DB, w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		_, _ = d.Pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1`, c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1})
}

// Middleware кладёт текущего пользователя в контекст (если сессия валидна).
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
					// Фиксируем возврат после паузы ≥6 часов — для дайджеста «пока вас не было».
					_, _ = d.Pool.Exec(r.Context(), `
						UPDATE users SET
							prev_seen_at = CASE
								WHEN last_seen_at IS NOT NULL AND last_seen_at < now() - interval '6 hours'
								THEN last_seen_at ELSE prev_seen_at END,
							last_seen_at = now()
						WHERE id=$1`, u.ID)
					r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, &u))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func FromContext(ctx context.Context) *User {
	u, _ := ctx.Value(ctxKey{}).(*User)
	return u
}
