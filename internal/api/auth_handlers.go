package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/DanMotive/Todorio/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

// POST /api/register — registration, gated by policy.registration.mode:
//   - closed         — no self-registration at all (an admin must create/invite users another way).
//   - invite_only     — a valid invite code is required.
//   - open_approval  — anyone can request an account; it stays "pending" until an admin approves it
//     (the default). A valid invite code always skips the pending queue, in any non-closed mode.
//
// The first registered user becomes root (covers a dev bootstrap without running `todorio setup`).
func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	mode := a.DB.Setting(r.Context(), "policy.registration.mode", "open_approval")
	if mode == "closed" {
		errJSON(w, http.StatusForbidden, "registration is closed")
		return
	}
	var in struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		InviteCode string `json:"invite_code"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	if !usernameRe.MatchString(in.Username) {
		errJSON(w, http.StatusBadRequest, "username: 3–32 characters, letters/digits/_ only")
		return
	}
	if len(in.Password) < 8 {
		errJSON(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	// Resolve the invite code up front for a clear error before doing the expensive password hash.
	// This is only a friendly pre-check: the real use is claimed atomically inside the registration
	// transaction below, so a concurrent request cannot redeem the same final use.
	inviteCode := strings.TrimSpace(in.InviteCode)
	if inviteCode != "" {
		if _, err := a.lookupInvite(r.Context(), inviteCode); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid or expired invite code")
			return
		}
	} else if mode == "invite_only" {
		errJSON(w, http.StatusForbidden, "an invite code is required to register")
		return
	}

	// Reject a name that differs from an existing one only by case before spending 64 MB on a
	// hash. The unique index from migration 0012 is the actual guarantee (this check races); it
	// is here so the common case returns a clear 409 rather than a constraint violation.
	var taken bool
	_ = a.DB.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM users WHERE lower(username)=lower($1))`, in.Username).Scan(&taken)
	if taken {
		errJSON(w, http.StatusConflict, "username is already taken")
		return
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "server error")
		return
	}

	// Registration is one transaction for both privilege decisions:
	//   * an advisory transaction lock serialises the "first account becomes root" check;
	//   * an invite use is claimed with a conditional UPDATE before the user is inserted.
	// A rollback returns the invite use if the username INSERT subsequently fails.
	tx, err := a.DB.Pool.Begin(r.Context())
	if err != nil {
		dbFail(r, "begin registration", err)
		errJSON(w, http.StatusInternalServerError, "server error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(743646746)`); err != nil {
		dbFail(r, "lock registration bootstrap", err)
		errJSON(w, http.StatusInternalServerError, "server error")
		return
	}

	var total int
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM users`).Scan(&total); err != nil {
		dbFail(r, "count users during registration", err)
		errJSON(w, http.StatusInternalServerError, "server error")
		return
	}
	role, status := "user", "pending"
	if total == 0 {
		// Dev bootstrap: the very first account on the server becomes root.
		if a.DB.Setting(r.Context(), "policy.registration.bootstrap_root", "true") != "true" {
			errJSON(w, http.StatusForbidden, "registration is closed until an administrator completes setup")
			return
		}
		log.Printf("register: granting root to the first account %q (bootstrap)", in.Username)
		role, status = "root", "active"
	} else if inviteCode != "" {
		err = tx.QueryRow(r.Context(), `
			UPDATE invites
			SET used_count = used_count + 1
			WHERE code=$1 AND used_count < max_uses
			  AND (expires_at IS NULL OR expires_at > now())
			RETURNING role`, inviteCode).Scan(&role)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				errJSON(w, http.StatusBadRequest, "invalid or expired invite code")
			} else {
				dbFail(r, "claim invite", err)
				errJSON(w, http.StatusInternalServerError, "server error")
			}
			return
		}
		status = "active"
	}

	var id int64
	err = tx.QueryRow(r.Context(),
		`INSERT INTO users(username, password_hash, role, status) VALUES($1,$2,$3,$4)
		 ON CONFLICT (username) DO NOTHING RETURNING id`,
		in.Username, hash, role, status).Scan(&id)
	if err != nil {
		// Every failure used to be reported as "username is already taken", so a database that was
		// out of disk, mid-failover or missing a column looked to the user like a name collision —
		// and left nothing in the log to say otherwise. Only the two genuine collision cases map to
		// 409 now; anything else is a server error and gets recorded.
		var pgErr *pgconn.PgError
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// ON CONFLICT DO NOTHING matched: the exact username already exists.
			errJSON(w, http.StatusConflict, "username is already taken")
		case errors.As(err, &pgErr) && pgErr.Code == "23505":
			// Unique violation from the case-insensitive index (migration 0012).
			errJSON(w, http.StatusConflict, "username is already taken")
		default:
			log.Printf("register: inserting user %q: %v", in.Username, err)
			errJSON(w, http.StatusInternalServerError, "server error")
		}
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		dbFail(r, "commit registration", err)
		errJSON(w, http.StatusInternalServerError, "server error")
		return
	}
	if status == "active" && total != 0 {
		a.postApprove(r.Context(), id, in.Username, role)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": status})
}

// POST /api/login — login by username/password; totp_code is required when 2FA is enabled.
// A recovery code is accepted in the same field as the TOTP code.
func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TOTPCode string `json:"totp_code"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	// Rate limit by IP+username so a lockout on one account can't be used to lock out everyone
	// sharing an IP, while still stopping a brute-force attempt against a single account.
	// Lower-cased so that alternating the capitalisation of the name cannot buy extra attempts.
	limitKey := clientIP(r) + ":" + strings.ToLower(in.Username)
	if !loginLimiter.begin(limitKey, a.maxLoginAttempts(r)) {
		errJSON(w, http.StatusTooManyRequests, "too many failed login attempts — try again in a few minutes")
		return
	}
	// begin() has claimed a slot; end() must run exactly once to release it. Anything that leaves
	// this handler without having authenticated the user counts as a failed attempt.
	failed := true
	defer func() { loginLimiter.end(limitKey, failed) }()

	var (
		id                      int64
		hash, role, status      string
		mustChange, totpEnabled bool
		totpSecret              *string
		totpLastCounter         *int64
	)
	err := a.DB.Pool.QueryRow(r.Context(),
		`SELECT id, password_hash, role, status, must_change_password, totp_secret, totp_enabled, totp_last_counter
		 FROM users WHERE username=$1 AND archived_at IS NULL`,
		in.Username).Scan(&id, &hash, &role, &status, &mustChange, &totpSecret, &totpEnabled, &totpLastCounter)
	if err != nil {
		// No such user. Burn the same argon2 work a real verification would have cost before
		// answering — otherwise this branch returns in microseconds while a valid username takes
		// tens of milliseconds, and the identical error message stops hiding anything.
		auth.BurnPasswordTime(in.Password)
		errJSON(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if !auth.VerifyPassword(in.Password, hash) {
		errJSON(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if totpEnabled {
		if in.TOTPCode == "" {
			// Not a failed attempt: the password was right and the client is being asked for the
			// second factor. Counting it would let a normal two-step login burn through the quota.
			failed = false
			writeJSON(w, http.StatusUnauthorized, map[string]any{"totp_required": true})
			return
		}
		// Per-account lockout, on top of the per-IP limiter: the code space is only 10^6 and an
		// attacker who already has the password can otherwise spread guesses across addresses.
		if locked, _ := a.totpLocked(r.Context(), id); locked {
			failed = false // already being counted by the account-level lock
			errJSON(w, http.StatusTooManyRequests, "too many invalid two-factor codes — try again later")
			return
		}
		switch {
		case a.verifyUserTOTP(r.Context(), id, totpSecret, totpLastCounter, in.TOTPCode):
			// ok
		case a.consumeRecoveryCode(r.Context(), id, in.TOTPCode):
			log.Printf("login: user %d signed in with a recovery code", id)
		default:
			a.totpFail(r.Context(), id)
			errJSON(w, http.StatusUnauthorized, "invalid two-factor code")
			return
		}
	}
	if status == "blocked" || status == "rejected" {
		failed = false
		errJSON(w, http.StatusForbidden, "access disabled by the administrator")
		return
	}
	a.enforceSessionLimit(r.Context(), id)
	if err := auth.CreateSession(r.Context(), a.DB, w, id, r.UserAgent(), auth.SecureRequest(r, a.Cfg.HTTPS)); err != nil {
		errJSON(w, http.StatusInternalServerError, "session error")
		return
	}
	failed = false
	loginLimiter.reset(limitKey)
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "username": in.Username, "role": role, "status": status,
		"must_change_password": mustChange,
	})
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	auth.DestroySession(r.Context(), a.DB, w, r, auth.SecureRequest(r, a.Cfg.HTTPS))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/me — reachable by pending users too (for the waiting page). Includes the full profile
// (locale/theme/avatar/notify_prefs) so a client on a NEW device picks up the user's own saved
// settings instead of just the server-wide defaults from /api/bootstrap.
func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errJSON(w, http.StatusUnauthorized, "login required")
		return
	}
	var unread int
	_ = a.DB.Pool.QueryRow(r.Context(),
		`SELECT count(*) FROM notifications WHERE user_id=$1 AND read_at IS NULL`, u.ID).Scan(&unread)

	var displayName, locale, themeColor, themeVisual, avatarPath *string
	var notifyPrefs json.RawMessage
	_ = a.DB.Pool.QueryRow(r.Context(), `
		SELECT display_name, locale, theme_color, theme_visual, avatar_path, notify_prefs
		FROM users WHERE id=$1`, u.ID).Scan(
		&displayName, &locale, &themeColor, &themeVisual, &avatarPath, &notifyPrefs)

	writeJSON(w, http.StatusOK, map[string]any{
		"user": u, "unread_notifications": unread,
		"profile": map[string]any{
			"display_name": displayName, "locale": locale,
			"theme_color": themeColor, "theme_visual": themeVisual,
			"avatar_path": avatarPath, "notify_prefs": notifyPrefs,
		},
	})
}

// PATCH /api/me — display name, locale, theme, notification preferences.
// notify_prefs is shallow-merged (jsonb ||), not replaced: sending {"sound":false} only touches
// that key and leaves dnd/types/reminders exactly as they were.
func (a *API) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errJSON(w, http.StatusUnauthorized, "login required")
		return
	}
	var in struct {
		DisplayName *string         `json:"display_name"`
		Locale      *string         `json:"locale"`
		ThemeColor  *string         `json:"theme_color"`
		ThemeVisual *string         `json:"theme_visual"`
		NotifyPrefs *map[string]any `json:"notify_prefs"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	var notifyJSON *string
	if in.NotifyPrefs != nil {
		b, err := json.Marshal(in.NotifyPrefs)
		if err != nil {
			errJSON(w, http.StatusBadRequest, "invalid notify_prefs")
			return
		}
		s := string(b)
		notifyJSON = &s
	}
	_, err := a.DB.Pool.Exec(r.Context(), `
		UPDATE users SET
			display_name = COALESCE($2, display_name),
			locale       = COALESCE($3, locale),
			theme_color  = COALESCE($4, theme_color),
			theme_visual = COALESCE($5, theme_visual),
			notify_prefs = CASE WHEN $6::text IS NULL THEN notify_prefs ELSE notify_prefs || $6::jsonb END
		WHERE id=$1`,
		u.ID, in.DisplayName, in.Locale, in.ThemeColor, in.ThemeVisual, notifyJSON)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid value (check the theme: red/blue/green/yellow/gray)")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/me/password — change password (available to pending users and must_change_password too).
func (a *API) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errJSON(w, http.StatusUnauthorized, "login required")
		return
	}
	var in struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := readJSON(r, &in); err != nil || len(in.NewPassword) < 8 {
		errJSON(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}
	var hash string
	if err := a.DB.Pool.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id=$1`, u.ID).Scan(&hash); err != nil {
		errJSON(w, http.StatusInternalServerError, "server error")
		return
	}
	if !auth.VerifyPassword(in.OldPassword, hash) {
		errJSON(w, http.StatusForbidden, "old password is incorrect")
		return
	}
	newHash, err := auth.HashPassword(in.NewPassword)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "server error")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(),
		`UPDATE users SET password_hash=$2, must_change_password=false WHERE id=$1`, u.ID, newHash)
	// Sign the other devices out. Someone changing their password because it may have leaked
	// gains nothing while whoever holds the old one is still sitting on a valid 30-day cookie —
	// that session would simply carry on. The current session is kept so the user is not thrown
	// out of the tab they just used, which also matches what the forced-change screen expects.
	if err := auth.DeleteOtherSessions(r.Context(), a.DB, u.ID, auth.CurrentSessionID(r)); err != nil {
		log.Printf("password change: revoking other sessions for user %d: %v", u.ID, err)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
