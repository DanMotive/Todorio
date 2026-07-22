package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

var imageExt = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// POST /api/tasks/{id}/attachments — multipart image upload (field "file").
// Size limit comes from settings (limits.uploads.max_file_size_mb, default 10 MB).
func (a *API) handleUploadAttachment(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	taskID, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var listID int64
	if a.DB.Pool.QueryRow(r.Context(), `SELECT list_id FROM tasks WHERE id=$1 AND archived_at IS NULL`, taskID).Scan(&listID) != nil {
		errJSON(w, http.StatusNotFound, "task not found")
		return
	}
	if !permAtLeast(a.listPermission(r, u, listID), "editor") {
		errJSON(w, http.StatusForbidden, "no permission")
		return
	}

	maxMB, _ := strconv.Atoi(a.DB.Setting(r.Context(), "limits.uploads.max_file_size_mb", "10"))
	if maxMB <= 0 {
		maxMB = 10
	}
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxMB)<<20)
	if err := r.ParseMultipartForm(int64(maxMB) << 20); err != nil {
		errJSON(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file is larger than %d MB", maxMB))
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		errJSON(w, http.StatusBadRequest, "expected a file field")
		return
	}
	defer file.Close()

	// Sniff the real file type — we don't trust the extension or Content-Type.
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
	rel := filepath.Join("tasks", strconv.FormatInt(taskID, 10), hex.EncodeToString(rnd)+ext)
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
	defer dst.Close()
	size, err := io.Copy(dst, io.MultiReader(newBytesReader(head[:n]), file))
	if err != nil {
		_ = os.Remove(abs)
		errJSON(w, http.StatusInternalServerError, "write error")
		return
	}

	var id int64
	if err := a.DB.Pool.QueryRow(r.Context(), `
		INSERT INTO attachments(target_type, target_id, uploader_id, file_path, mime_type, size_bytes)
		VALUES('task',$1,$2,$3,$4,$5) RETURNING id`,
		taskID, u.ID, rel, mime, size).Scan(&id); err != nil {
		_ = os.Remove(abs)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "mime_type": mime, "size_bytes": size})
}

// GET /api/tasks/{id}/attachments
func (a *API) handleListAttachments(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	taskID, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var listID int64
	if a.DB.Pool.QueryRow(r.Context(), `SELECT list_id FROM tasks WHERE id=$1`, taskID).Scan(&listID) != nil {
		errJSON(w, http.StatusNotFound, "task not found")
		return
	}
	if !permAtLeast(a.listPermission(r, u, listID), "viewer") {
		errJSON(w, http.StatusForbidden, "no access")
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT id, mime_type, size_bytes, created_at FROM attachments
		WHERE target_type='task' AND target_id=$1 ORDER BY id`, taskID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, size int64
		var mime string
		var createdAt any
		if rows.Scan(&id, &mime, &size, &createdAt) == nil {
			list = append(list, map[string]any{"id": id, "mime_type": mime, "size_bytes": size, "created_at": createdAt})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"attachments": list})
}

// GET /api/attachments/{id} — serves the file after checking access to the task.
func (a *API) handleGetAttachment(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var rel, mime string
	var taskID int64
	if a.DB.Pool.QueryRow(r.Context(), `
		SELECT file_path, mime_type, target_id FROM attachments WHERE id=$1 AND target_type='task'`,
		id).Scan(&rel, &mime, &taskID) != nil {
		errJSON(w, http.StatusNotFound, "attachment not found")
		return
	}
	var listID int64
	if a.DB.Pool.QueryRow(r.Context(), `SELECT list_id FROM tasks WHERE id=$1`, taskID).Scan(&listID) != nil ||
		!permAtLeast(a.listPermission(r, u, listID), "viewer") {
		errJSON(w, http.StatusForbidden, "no access")
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, filepath.Join(a.Cfg.UploadsDir, rel))
}

// DELETE /api/attachments/{id} — uploader or admin.
func (a *API) handleDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var rel string
	err = a.DB.Pool.QueryRow(r.Context(), `
		DELETE FROM attachments WHERE id=$1 AND ($2 OR uploader_id=$3) RETURNING file_path`,
		id, u.IsAdmin(), u.ID).Scan(&rel)
	if err != nil {
		errJSON(w, http.StatusForbidden, "you can only delete your own attachments")
		return
	}
	_ = os.Remove(filepath.Join(a.Cfg.UploadsDir, rel))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// newBytesReader — tiny helper so we don't pull in bytes just for one Reader.
type bytesReader struct {
	b []byte
	i int
}

func newBytesReader(b []byte) *bytesReader { return &bytesReader{b: b} }

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
