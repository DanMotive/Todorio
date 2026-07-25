package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/DanMotive/Todorio/internal/auth"
)

// Per-account throttling for code entry.
//
// The login endpoint is covered by the IP+username limiter, but /api/me/totp/* is reached with
// a valid session and had no limit of any kind: six digits with a ±1 window means three of the
// 10^6 codes are valid at any moment, so an unthrottled loop finds one in minutes. Five wrong
// codes now park the account's second factor for a quarter of an hour.
const (
	totpMaxFails   = 5
	totpLockWindow = 15 * time.Minute
)

// POST /api/me/totp/setup — generate a secret. Available to any active user: the spec calls TOTP
// out as "especially important for root", not root/admin-exclusive, so any account can opt in.
func (a *API) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	secret, err := auth.NewTOTPSecret()
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "generation error")
		return
	}
	// The new secret goes to totp_pending_secret and nothing else is touched.
	//
	// This used to write totp_secret and set totp_enabled=false in one statement, which meant
	// merely *calling* setup switched off two-factor authentication for an account that already
	// had it on — no code required. Anyone who got hold of a session could disable 2FA with a
	// single request and skip the code check in the disable handler entirely. The live secret is
	// now only replaced in enable, after a code proves the new one works.
	if _, err := a.DB.Pool.Exec(r.Context(),
		`UPDATE users SET totp_pending_secret=$2 WHERE id=$1`, u.ID, secret); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	siteName := a.DB.Setting(r.Context(), "branding.site_name", "Todorio")
	writeJSON(w, http.StatusOK, map[string]string{
		"secret":  secret,
		"otpauth": auth.TOTPURL(secret, u.Username, siteName),
	})
}

// POST /api/me/totp/enable {code} — confirm and enable. Returns one-time recovery codes.
func (a *API) handleTOTPEnable(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	if locked, _ := a.totpLocked(r.Context(), u.ID); locked {
		errJSON(w, http.StatusTooManyRequests, "too many invalid codes — try again later")
		return
	}
	var pending *string
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT totp_pending_secret FROM users WHERE id=$1`, u.ID).Scan(&pending) != nil || pending == nil {
		errJSON(w, http.StatusBadRequest, "run setup first")
		return
	}
	counter, ok := auth.VerifyTOTPAt(*pending, in.Code, 0)
	if !ok {
		a.totpFail(r.Context(), u.ID)
		errJSON(w, http.StatusForbidden, "invalid code")
		return
	}

	codes, err := auth.NewRecoveryCodes(auth.RecoveryCodeCount)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "generation error")
		return
	}

	// Promote the pending secret and replace the recovery codes in one transaction, so a failure
	// half-way cannot leave 2FA switched on with the codes of the previous enrolment.
	tx, err := a.DB.Pool.Begin(r.Context())
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	if _, err := tx.Exec(r.Context(), `
		UPDATE users SET
			totp_secret=totp_pending_secret,
			totp_pending_secret=NULL,
			totp_enabled=true,
			totp_last_counter=$2,
			totp_fail_count=0,
			totp_locked_until=NULL
		WHERE id=$1`, u.ID, counter); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM totp_recovery_codes WHERE user_id=$1`, u.ID); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	for _, c := range codes {
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO totp_recovery_codes(user_id, code_hash) VALUES($1,$2)`,
			u.ID, auth.HashRecoveryCode(c)); err != nil {
			errJSON(w, http.StatusInternalServerError, "database error")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}

	// The only time the plaintext codes ever leave the server.
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "recovery_codes": codes})
}

// POST /api/me/totp/disable {code} — a valid TOTP or recovery code is required while 2FA is on.
func (a *API) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	var secret *string
	var lastCounter *int64
	var enabled bool
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT totp_secret, totp_enabled, totp_last_counter FROM users WHERE id=$1`,
		u.ID).Scan(&secret, &enabled, &lastCounter) != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if enabled {
		if locked, _ := a.totpLocked(r.Context(), u.ID); locked {
			errJSON(w, http.StatusTooManyRequests, "too many invalid codes — try again later")
			return
		}
		if !a.verifyUserTOTP(r.Context(), u.ID, secret, lastCounter, in.Code) &&
			!a.consumeRecoveryCode(r.Context(), u.ID, in.Code) {
			a.totpFail(r.Context(), u.ID)
			errJSON(w, http.StatusForbidden, "invalid code")
			return
		}
	}
	// When 2FA is not enabled this is just "cancel the enrolment I started", which needs no code.
	tx, err := a.DB.Pool.Begin(r.Context())
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if _, err := tx.Exec(r.Context(), `
		UPDATE users SET
			totp_secret=NULL, totp_pending_secret=NULL, totp_enabled=false,
			totp_last_counter=NULL, totp_fail_count=0, totp_locked_until=NULL
		WHERE id=$1`, u.ID); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM totp_recovery_codes WHERE user_id=$1`, u.ID); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// verifyUserTOTP checks a code against the user's live secret and, on success, records the
// counter it used so the same code cannot be presented twice.
func (a *API) verifyUserTOTP(ctx context.Context, userID int64, secret *string, lastCounter *int64, code string) bool {
	if secret == nil {
		return false
	}
	var min int64
	if lastCounter != nil {
		min = *lastCounter
	}
	counter, ok := auth.VerifyTOTPAt(*secret, code, min)
	if !ok {
		return false
	}
	// The guard on totp_last_counter makes the write itself the point of no return: two requests
	// racing with the same code both pass VerifyTOTPAt, but only one updates a row.
	ct, err := a.DB.Pool.Exec(ctx, `
		UPDATE users SET totp_last_counter=$2, totp_fail_count=0, totp_locked_until=NULL
		WHERE id=$1 AND (totp_last_counter IS NULL OR totp_last_counter < $2)`, userID, counter)
	if err != nil {
		log.Printf("totp: recording counter for user %d: %v", userID, err)
		return false
	}
	return ct.RowsAffected() == 1
}

// consumeRecoveryCode spends a single-use recovery code, if it matches an unused one.
//
// Marking it used is the same statement that matches it, with `used_at IS NULL` in the WHERE
// clause, so two simultaneous attempts with the same code cannot both succeed — exactly one
// UPDATE reports a row.
func (a *API) consumeRecoveryCode(ctx context.Context, userID int64, code string) bool {
	norm := auth.NormalizeRecoveryCode(code)
	if len(norm) < 8 {
		return false // too short to be one of ours; don't bother the database
	}
	ct, err := a.DB.Pool.Exec(ctx, `
		UPDATE totp_recovery_codes SET used_at=now()
		WHERE user_id=$1 AND code_hash=$2 AND used_at IS NULL`, userID, auth.HashRecoveryCode(norm))
	if err != nil {
		log.Printf("totp: consuming recovery code for user %d: %v", userID, err)
		return false
	}
	return ct.RowsAffected() == 1
}

// totpLocked reports whether code entry is currently parked for this account.
func (a *API) totpLocked(ctx context.Context, userID int64) (bool, error) {
	var locked bool
	err := a.DB.Pool.QueryRow(ctx,
		`SELECT COALESCE(totp_locked_until > now(), false) FROM users WHERE id=$1`, userID).Scan(&locked)
	return locked, err
}

// totpFail records a wrong code and starts a lockout once the allowance is used up.
// The counter resets when the lock is applied, so the next window starts clean.
func (a *API) totpFail(ctx context.Context, userID int64) {
	_, err := a.DB.Pool.Exec(ctx, `
		UPDATE users SET
			totp_fail_count = CASE WHEN totp_fail_count + 1 >= $2 THEN 0 ELSE totp_fail_count + 1 END,
			totp_locked_until = CASE WHEN totp_fail_count + 1 >= $2
				THEN now() + make_interval(secs => $3) ELSE totp_locked_until END
		WHERE id=$1`, userID, totpMaxFails, totpLockWindow.Seconds())
	if err != nil {
		log.Printf("totp: recording failure for user %d: %v", userID, err)
	}
}
