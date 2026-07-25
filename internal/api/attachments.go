package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
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

// commentListID resolves the list a comment belongs to, for permission checks. Returns false
// when the comment doesn't exist or its task has been archived.
func (a *API) commentListID(r *http.Request, commentID int64) (int64, bool) {
	var listID int64
	err := a.DB.Pool.QueryRow(r.Context(), `
		SELECT t.list_id FROM comments c
		JOIN tasks t ON t.id = c.task_id
		WHERE c.id=$1 AND c.deleted_at IS NULL AND t.archived_at IS NULL`, commentID).Scan(&listID)
	return listID, err == nil
}

// POST /api/tasks/{id}/attachments — multipart image upload (field "file").
func (a *API) handleUploadAttachment(w http.ResponseWriter, r *http.Request) {
	a.uploadAttachment(w, r, "task")
}

// POST /api/comments/{id}/attachments — same upload flow, different owner (spec section 7:
// attachments belong to tasks *and* comments).
func (a *API) handleUploadCommentAttachment(w http.ResponseWriter, r *http.Request) {
	a.uploadAttachment(w, r, "comment")
}

// uploadAttachment handles both target types. Size limit comes from settings
// (limits.uploads.max_file_size_mb, default 10 MB); per-target count limits come from
// limits.uploads.max_per_task / max_per_comment (spec section 10 examples: 10 and 5).
func (a *API) uploadAttachment(w http.ResponseWriter, r *http.Request, targetType string) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if !a.featureEnabled(r.Context(), "attachments") {
		errJSON(w, http.StatusForbidden, "attachments are disabled on this server")
		return
	}
	targetID, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var listID int64
	var ok bool
	if targetType == "comment" {
		if listID, ok = a.commentListID(r, targetID); !ok {
			errJSON(w, http.StatusNotFound, "comment not found")
			return
		}
	} else {
		if a.DB.Pool.QueryRow(r.Context(), `SELECT list_id FROM tasks WHERE id=$1 AND archived_at IS NULL`, targetID).Scan(&listID) != nil {
			errJSON(w, http.StatusNotFound, "task not found")
			return
		}
	}
	if !permAtLeast(a.listPermission(r, u, listID), "editor") {
		errJSON(w, http.StatusForbidden, "no permission")
		return
	}

	// Per-target attachment count limit. 0 = unlimited, via intSetting so an explicit 0 is
	// distinguishable from "never configured".
	limitKey, defLimit := "limits.uploads.max_per_task", 10
	if targetType == "comment" {
		limitKey, defLimit = "limits.uploads.max_per_comment", 5
	}
	if maxN := a.intSetting(r.Context(), limitKey, defLimit); maxN > 0 {
		var count int
		_ = a.DB.Pool.QueryRow(r.Context(),
			`SELECT count(*) FROM attachments WHERE target_type=$1 AND target_id=$2`,
			targetType, targetID).Scan(&count)
		if count >= maxN {
			errJSON(w, http.StatusForbidden,
				fmt.Sprintf("attachment limit reached (%d per %s)", maxN, targetType))
			return
		}
	}

	if !a.enforceAction(w, r, u.ID, "upload", "limits.actions.uploads_per_hour", 0) {
		return
	}

	maxMB := a.intSetting(r.Context(), "limits.uploads.max_file_size_mb", 10)
	if maxMB > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, int64(maxMB)<<20)
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		msg := "file is too large"
		if maxMB > 0 {
			msg = fmt.Sprintf("file is larger than %d MB", maxMB)
		}
		errJSON(w, http.StatusRequestEntityTooLarge, msg)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		errJSON(w, http.StatusBadRequest, "expected a file field")
		return
	}
	defer file.Close()
	// header.Size is the length the client declared for this part, not what it actually sends,
	// so this is only a cheap early rejection. The quota is charged again below against the real
	// byte count.
	if err := a.checkStorageQuota(r.Context(), header.Size); err != nil {
		errJSON(w, http.StatusInsufficientStorage, err.Error())
		return
	}

	// Sniff the real file type — we don't trust the extension or Content-Type.
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	mime := http.DetectContentType(head[:n])
	ext, ok := imageExt[mime]
	if !ok {
		errJSON(w, http.StatusBadRequest, "images only: jpeg, png, webp, gif")
		return
	}

	// Clean the image before it reaches disk: strip EXIF/XMP (a phone photo carries GPS
	// coordinates, the device serial and the capture time, none of it visible in the UI) and
	// refuse decompression bombs. See imagemeta.go.
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
	rel := filepath.Join(targetType+"s", strconv.FormatInt(targetID, 10), hex.EncodeToString(rnd)+ext)
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
	size, err := io.Copy(dst, payload)
	if err != nil {
		_ = os.Remove(abs)
		errJSON(w, http.StatusInternalServerError, "write error")
		return
	}

	var id int64
	if err := a.DB.Pool.QueryRow(r.Context(), `
		INSERT INTO attachments(target_type, target_id, uploader_id, file_path, mime_type, size_bytes)
		VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
		targetType, targetID, u.ID, rel, mime, size).Scan(&id); err != nil {
		_ = os.Remove(abs)
		dbFail(r, "insert attachment", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	a.countAction(r.Context(), u.ID, "upload")
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "mime_type": mime, "size_bytes": size})
}

// GET /api/tasks/{id}/attachments
func (a *API) handleListAttachments(w http.ResponseWriter, r *http.Request) {
	a.listAttachments(w, r, "task")
}

// GET /api/comments/{id}/attachments
func (a *API) handleListCommentAttachments(w http.ResponseWriter, r *http.Request) {
	a.listAttachments(w, r, "comment")
}

func (a *API) listAttachments(w http.ResponseWriter, r *http.Request, targetType string) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	targetID, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var listID int64
	var ok bool
	if targetType == "comment" {
		if listID, ok = a.commentListID(r, targetID); !ok {
			errJSON(w, http.StatusNotFound, "comment not found")
			return
		}
	} else {
		if a.DB.Pool.QueryRow(r.Context(), `SELECT list_id FROM tasks WHERE id=$1`, targetID).Scan(&listID) != nil {
			errJSON(w, http.StatusNotFound, "task not found")
			return
		}
	}
	if !permAtLeast(a.listPermission(r, u, listID), "viewer") {
		errJSON(w, http.StatusForbidden, "no access")
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT id, mime_type, size_bytes, created_at FROM attachments
		WHERE target_type=$1 AND target_id=$2 ORDER BY id`, targetType, targetID)
	if err != nil {
		dbFail(r, "list attachments", err)
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
	var rel, mime, targetType string
	var targetID int64
	if a.DB.Pool.QueryRow(r.Context(), `
		SELECT file_path, mime_type, target_type, target_id FROM attachments WHERE id=$1`,
		id).Scan(&rel, &mime, &targetType, &targetID) != nil {
		errJSON(w, http.StatusNotFound, "attachment not found")
		return
	}
	// Files are served only after checking access to the owning list — never as a public
	// directory. A comment attachment resolves through its comment's task to the same check.
	var listID int64
	var ok bool
	if targetType == "comment" {
		listID, ok = a.commentListID(r, targetID)
	} else {
		ok = a.DB.Pool.QueryRow(r.Context(), `SELECT list_id FROM tasks WHERE id=$1`, targetID).Scan(&listID) == nil
	}
	if !ok || !permAtLeast(a.listPermission(r, u, listID), "viewer") {
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
