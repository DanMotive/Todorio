package api

import (
	"net/http"

	"github.com/DanMotive/Todorio/internal/auth"
)

// POST /api/me/totp/setup — generate a secret (root/admin only — 2FA for privileged accounts).
func (a *API) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	u := a.requireAdmin(w, r)
	if u == nil {
		return
	}
	secret, err := auth.NewTOTPSecret()
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "generation error")
		return
	}
	// The secret is saved but not activated until confirmed with a code.
	if _, err := a.DB.Pool.Exec(r.Context(),
		`UPDATE users SET totp_secret=$2, totp_enabled=false WHERE id=$1`, u.ID, secret); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	siteName := a.DB.Setting(r.Context(), "branding.site_name", "Todorio")
	writeJSON(w, http.StatusOK, map[string]string{
		"secret":  secret,
		"otpauth": auth.TOTPURL(secret, u.Username, siteName),
	})
}

// POST /api/me/totp/enable {code} — confirm and enable.
func (a *API) handleTOTPEnable(w http.ResponseWriter, r *http.Request) {
	u := a.requireAdmin(w, r)
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
	if a.DB.Pool.QueryRow(r.Context(), `SELECT totp_secret FROM users WHERE id=$1`, u.ID).Scan(&secret) != nil || secret == nil {
		errJSON(w, http.StatusBadRequest, "run setup first")
		return
	}
	if !auth.VerifyTOTP(*secret, in.Code) {
		errJSON(w, http.StatusForbidden, "invalid code")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE users SET totp_enabled=true WHERE id=$1`, u.ID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/me/totp/disable {code}
func (a *API) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	u := a.requireAdmin(w, r)
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
	var enabled bool
	if a.DB.Pool.QueryRow(r.Context(), `SELECT totp_secret, totp_enabled FROM users WHERE id=$1`, u.ID).Scan(&secret, &enabled) != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if enabled && (secret == nil || !auth.VerifyTOTP(*secret, in.Code)) {
		errJSON(w, http.StatusForbidden, "invalid code")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE users SET totp_secret=NULL, totp_enabled=false WHERE id=$1`, u.ID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
