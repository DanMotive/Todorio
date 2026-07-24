package api

// Instance logo (spec section 18: "логотип (текст/инициалы/картинка)"). Root can replace the
// built-in SVG with an uploaded image; everything else falls back to /icons/logo.svg shipped
// with the frontend.
//
// Stored under {UploadsDir}/branding and pointed at by the branding.logo_path setting, which
// mirrors how avatars work — one current file, replacing it deletes the previous one. Unlike
// avatars, the logo is served without authentication: it appears on the login screen, before
// any session exists.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// jsonString encodes s as a JSON string literal for system_settings.value, which is jsonb —
// a bare path would not be valid JSON and the insert would fail.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// logoExt allows SVG on top of the raster types used elsewhere — a logo is exactly the case
// where a vector file is the right answer, and the default shipped logo is itself an SVG.
//
// SVG can carry scripts, so it is never served inline: handleGetLogo sends it with a
// restrictive CSP and nosniff, and the upload is limited to root, who can already change
// server settings. That keeps a stored-XSS vector from being opened up to ordinary users.
var logoExt = map[string]string{
	"image/jpeg":    ".jpg",
	"image/png":     ".png",
	"image/webp":    ".webp",
	"image/gif":     ".gif",
	"image/svg+xml": ".svg",
}

// POST /api/admin/logo — multipart image upload (field "file"), root only.
func (a *API) handleUploadLogo(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if u.Role != "root" {
		errJSON(w, http.StatusForbidden, "root only")
		return
	}

	maxMB := a.intSetting(r.Context(), "limits.uploads.max_file_size_mb", 10)
	if maxMB > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, int64(maxMB)<<20)
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		errJSON(w, http.StatusRequestEntityTooLarge, "file is too large")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		errJSON(w, http.StatusBadRequest, "expected a file field")
		return
	}
	defer file.Close()
	if err := a.checkStorageQuota(r.Context(), header.Size); err != nil {
		errJSON(w, http.StatusInsufficientStorage, err.Error())
		return
	}

	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	mime := http.DetectContentType(head[:n])
	// DetectContentType reports SVG as text/xml or text/plain, so accept those when the
	// uploaded filename says .svg — the content is still served non-inline under a CSP.
	if ext := filepath.Ext(header.Filename); ext == ".svg" &&
		(mime == "text/xml; charset=utf-8" || mime == "text/plain; charset=utf-8" || mime == "text/xml") {
		mime = "image/svg+xml"
	}
	ext, ok := logoExt[mime]
	if !ok {
		errJSON(w, http.StatusBadRequest, "images only: jpeg, png, webp, gif, svg")
		return
	}

	rnd := make([]byte, 8)
	_, _ = rand.Read(rnd)
	rel := filepath.Join("branding", "logo-"+hex.EncodeToString(rnd)+ext)
	abs := filepath.Join(a.Cfg.UploadsDir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		errJSON(w, http.StatusInternalServerError, "storage unavailable")
		return
	}
	dst, err := os.Create(abs)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "storage unavailable")
		return
	}
	if _, err := io.Copy(dst, io.MultiReader(newBytesReader(head[:n]), file)); err != nil {
		dst.Close()
		_ = os.Remove(abs)
		errJSON(w, http.StatusInternalServerError, "write error")
		return
	}
	dst.Close()

	old := a.DB.Setting(r.Context(), "branding.logo_path", "")
	if err := a.DB.SetSetting(r.Context(), "branding.logo_path", jsonString(rel)); err != nil {
		_ = os.Remove(abs)
		dbFail(r, "set logo path", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if old != "" && old != rel {
		_ = os.Remove(filepath.Join(a.Cfg.UploadsDir, old))
	}
	writeJSON(w, http.StatusOK, map[string]any{"logo_path": rel})
}

// DELETE /api/admin/logo — revert to the built-in logo.
func (a *API) handleDeleteLogo(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if u.Role != "root" {
		errJSON(w, http.StatusForbidden, "root only")
		return
	}
	old := a.DB.Setting(r.Context(), "branding.logo_path", "")
	if err := a.DB.SetSetting(r.Context(), "branding.logo_path", jsonString("")); err != nil {
		dbFail(r, "clear logo path", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if old != "" {
		_ = os.Remove(filepath.Join(a.Cfg.UploadsDir, old))
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/logo — public: the login screen needs it before a session exists. 404 when no custom
// logo is set so the frontend's <img onerror> falls back to the bundled SVG.
func (a *API) handleGetLogo(w http.ResponseWriter, r *http.Request) {
	rel := a.DB.Setting(r.Context(), "branding.logo_path", "")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	// Uploaded content is never executed in the site's origin: an SVG logo is rendered by the
	// browser as an image, but a direct visit must not run any script it contains.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=300")
	http.ServeFile(w, r, filepath.Join(a.Cfg.UploadsDir, rel))
}
