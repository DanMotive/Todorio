package api

// Custom wallpapers: one uploaded picture per user, shown behind the app (see
// web/src/wallpaper.tsx for the layer itself and the built-in gradients).
//
// Modelled on avatars — same sniffing, same sanitising, same quota and rate limiting, one
// current file per user with the previous one deleted on replace — with one deliberate
// difference: a wallpaper is private. Avatars are readable by any authenticated user because
// they appear next to every task and comment; nobody but the owner ever needs to see a
// wallpaper, so there is no route for reading someone else's, and it is served with a private
// cache header.
//
// The choice of a *built-in* wallpaper stays in localStorage (per device). Only an uploaded
// picture lives on the server: a file has to be stored somewhere, and re-uploading it on every
// device would be worse than syncing it.

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

// POST /api/me/wallpaper — multipart image upload (field "file"), replaces any existing one.
func (a *API) handleUploadWallpaper(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}

	// Shares the hourly upload budget with attachments and avatars. A wallpaper is the largest
	// image a user uploads by far, so leaving it unmetered would undo the point of that limit.
	if !a.enforceAction(w, r, u.ID, "upload", "limits.actions.uploads_per_hour", 0) {
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

	// Same metadata stripping and canvas-size ceiling as every other upload. A wallpaper is
	// typically a photo straight off a phone, which is exactly the kind of file that carries
	// GPS coordinates.
	payload, exactSize, err := sanitizeUpload(mime, io.MultiReader(newBytesReader(head[:n]), file))
	if err != nil {
		if errors.Is(err, ErrImageTooLarge) {
			errJSON(w, http.StatusBadRequest, err.Error())
			return
		}
		errJSON(w, http.StatusInternalServerError, "read error")
		return
	}
	if exactSize >= 0 {
		if err := a.checkStorageQuota(r.Context(), exactSize); err != nil {
			errJSON(w, http.StatusInsufficientStorage, err.Error())
			return
		}
	}

	rnd := make([]byte, 8)
	_, _ = rand.Read(rnd)
	rel := filepath.Join("wallpapers", strconv.FormatInt(u.ID, 10)+"-"+hex.EncodeToString(rnd)+ext)
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
	if _, err := io.Copy(dst, payload); err != nil {
		dst.Close()
		_ = os.Remove(abs)
		errJSON(w, http.StatusInternalServerError, "write error")
		return
	}
	dst.Close()

	var oldPath *string
	_ = a.DB.Pool.QueryRow(r.Context(), `SELECT wallpaper_path FROM users WHERE id=$1`, u.ID).Scan(&oldPath)
	if _, err := a.DB.Pool.Exec(r.Context(), `UPDATE users SET wallpaper_path=$2 WHERE id=$1`, u.ID, rel); err != nil {
		_ = os.Remove(abs)
		dbFail(r, "set wallpaper path", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	// Only after the row points at the new file: losing the old picture while the update failed
	// would leave the user with a broken wallpaper and nothing to fall back to.
	if oldPath != nil && *oldPath != "" && *oldPath != rel {
		_ = os.Remove(filepath.Join(a.Cfg.UploadsDir, *oldPath))
	}
	a.countAction(r.Context(), u.ID, "upload")
	writeJSON(w, http.StatusOK, map[string]any{"wallpaper_path": rel})
}

// DELETE /api/me/wallpaper — remove the uploaded picture; the picker falls back to the
// built-in wallpapers.
func (a *API) handleDeleteWallpaper(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	var oldPath *string
	_ = a.DB.Pool.QueryRow(r.Context(), `SELECT wallpaper_path FROM users WHERE id=$1`, u.ID).Scan(&oldPath)
	if _, err := a.DB.Pool.Exec(r.Context(), `UPDATE users SET wallpaper_path=NULL WHERE id=$1`, u.ID); err != nil {
		dbFail(r, "clear wallpaper path", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if oldPath != nil && *oldPath != "" {
		_ = os.Remove(filepath.Join(a.Cfg.UploadsDir, *oldPath))
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/me/wallpaper — the owner's own picture. 404 when none is set so the frontend can
// treat a missing wallpaper the same way it treats a deleted one.
func (a *API) handleGetWallpaper(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	var rel *string
	if a.DB.Pool.QueryRow(r.Context(), `SELECT wallpaper_path FROM users WHERE id=$1`, u.ID).Scan(&rel) != nil || rel == nil || *rel == "" {
		http.NotFound(w, r)
		return
	}
	// "private" keeps shared caches from holding a copy: unlike an avatar, this image is only
	// ever meant for one account.
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, filepath.Join(a.Cfg.UploadsDir, *rel))
}
