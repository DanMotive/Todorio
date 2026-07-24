package api

// User avatars (spec section 18): "no avatar -> circle with initials; in the profile:
// auto-initials / upload / remove". Images are stored the same way as task attachments (sniffed
// mime type, random filename) but scoped under {UploadsDir}/avatars and keyed off users.avatar_path
// instead of the attachments table, since an avatar is 1-per-user and not really a discussion
// attachment.

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

// POST /api/me/avatar — multipart image upload (field "file"), replaces any existing avatar.
func (a *API) handleUploadAvatar(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}

	maxMB := a.intSetting(r.Context(), "limits.uploads.max_file_size_mb", 10)
	if maxMB > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, int64(maxMB)<<20)
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
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
	ext, ok := imageExt[mime]
	if !ok {
		errJSON(w, http.StatusBadRequest, "images only: jpeg, png, webp, gif")
		return
	}

	rnd := make([]byte, 8)
	_, _ = rand.Read(rnd)
	rel := filepath.Join("avatars", strconv.FormatInt(u.ID, 10)+"-"+hex.EncodeToString(rnd)+ext)
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

	var oldPath *string
	_ = a.DB.Pool.QueryRow(r.Context(), `SELECT avatar_path FROM users WHERE id=$1`, u.ID).Scan(&oldPath)
	if _, err := a.DB.Pool.Exec(r.Context(), `UPDATE users SET avatar_path=$2 WHERE id=$1`, u.ID, rel); err != nil {
		_ = os.Remove(abs)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if oldPath != nil && *oldPath != "" {
		_ = os.Remove(filepath.Join(a.Cfg.UploadsDir, *oldPath))
	}
	writeJSON(w, http.StatusOK, map[string]any{"avatar_path": rel})
}

// DELETE /api/me/avatar — remove the uploaded avatar; the UI falls back to initials.
func (a *API) handleDeleteAvatar(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	var oldPath *string
	_ = a.DB.Pool.QueryRow(r.Context(), `SELECT avatar_path FROM users WHERE id=$1`, u.ID).Scan(&oldPath)
	if _, err := a.DB.Pool.Exec(r.Context(), `UPDATE users SET avatar_path=NULL WHERE id=$1`, u.ID); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if oldPath != nil && *oldPath != "" {
		_ = os.Remove(filepath.Join(a.Cfg.UploadsDir, *oldPath))
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/users/{id}/avatar — any active, authenticated user can view a teammate's avatar (not
// sensitive, same trust level as seeing their username in a task/comment). 404 if none set, so
// the frontend's <img onerror> can fall back to the initials circle.
func (a *API) handleGetAvatar(w http.ResponseWriter, r *http.Request) {
	if a.requireUser(w, r) == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var rel *string
	if a.DB.Pool.QueryRow(r.Context(), `SELECT avatar_path FROM users WHERE id=$1`, id).Scan(&rel) != nil || rel == nil {
		http.NotFound(w, r)
		return
	}
	abs := filepath.Join(a.Cfg.UploadsDir, *rel)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, abs)
}
