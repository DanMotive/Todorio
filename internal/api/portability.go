package api

// Space export and import.
//
// Not in the original spec, but a self-hosted product whose whole premise is "your data on your
// server" should never be the thing that traps that data. Export produces one JSON document that
// a human can read and a script can process; import reconstructs it.
//
// Deliberate scope: structure and text — lists, tasks, subtasks, comments, notes, and the space's
// own settings. Binary attachments are not embedded, because base64-inlining every image would
// turn a modest space into a hundred-megabyte JSON file. Attachment metadata is exported so it's
// clear what existed, and the files themselves live under UploadsDir for a filesystem backup
// (`todorio backup create`) to capture.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const exportFormatVersion = 1

type exportTask struct {
	// Local ids so parent/child and dependency links survive a round trip without colliding
	// with ids in the destination database.
	Ref          int64              `json:"ref"`
	ParentRef    *int64             `json:"parent_ref,omitempty"`
	Title        string             `json:"title"`
	Description  string             `json:"description"`
	Status       string             `json:"status"`
	Priority     string             `json:"priority"`
	Assignee     *string            `json:"assignee,omitempty"` // username, not id
	StartAt      *time.Time         `json:"start_at,omitempty"`
	DueAt        *time.Time         `json:"due_at,omitempty"`
	Weight       int                `json:"weight"`
	Progress     *int               `json:"progress,omitempty"`
	CustomFields json.RawMessage    `json:"custom_fields,omitempty"`
	CompletedAt  *time.Time         `json:"completed_at,omitempty"`
	Comments     []exportComment    `json:"comments,omitempty"`
	Attachments  []exportAttachment `json:"attachments,omitempty"`
}

type exportComment struct {
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	IsSystem  bool      `json:"is_system"`
	CreatedAt time.Time `json:"created_at"`
}

// exportAttachment records what was attached without the bytes — see the package comment.
type exportAttachment struct {
	FilePath  string `json:"file_path"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
}

type exportList struct {
	Name      string       `json:"name"`
	IsPrivate bool         `json:"is_private"`
	Tasks     []exportTask `json:"tasks"`
}

type exportNote struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type spaceExport struct {
	FormatVersion int             `json:"format_version"`
	ExportedAt    time.Time       `json:"exported_at"`
	AppVersion    string          `json:"app_version"`
	SpaceName     string          `json:"space_name"`
	Settings      json.RawMessage `json:"settings,omitempty"`
	Lists         []exportList    `json:"lists"`
	Notes         []exportNote    `json:"notes,omitempty"`
}

// GET /api/spaces/{id}/export — owner (or admin) downloads the whole space as JSON.
func (a *API) handleExportSpace(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	// Export is the entire contents of a space, including private lists, so it requires owner
	// rights — not mere membership.
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) != "owner" {
		errJSON(w, http.StatusForbidden, "space owner permission required")
		return
	}

	out := spaceExport{
		FormatVersion: exportFormatVersion,
		ExportedAt:    time.Now(),
		AppVersion:    a.Version,
		Lists:         []exportList{},
	}
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT name, settings FROM spaces WHERE id=$1 AND archived_at IS NULL`, spaceID).
		Scan(&out.SpaceName, &out.Settings) != nil {
		errJSON(w, http.StatusNotFound, "space not found")
		return
	}

	// Lists, then tasks per list. Two passes rather than one giant join: the nesting is easier
	// to get right, and an export is not a hot path.
	listRows, err := a.DB.Pool.Query(r.Context(),
		`SELECT id, name, is_private FROM lists WHERE space_id=$1 AND archived_at IS NULL ORDER BY position, id`,
		spaceID)
	if err != nil {
		dbFail(r, "export lists", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	type listRef struct {
		id   int64
		name string
		priv bool
	}
	var lists []listRef
	for listRows.Next() {
		var lr listRef
		if listRows.Scan(&lr.id, &lr.name, &lr.priv) == nil {
			lists = append(lists, lr)
		}
	}
	listRows.Close()

	for _, lr := range lists {
		el := exportList{Name: lr.name, IsPrivate: lr.priv, Tasks: []exportTask{}}
		taskRows, err := a.DB.Pool.Query(r.Context(), `
			SELECT t.id, t.parent_id, t.title, t.description, t.status, COALESCE(t.priority,'normal'),
				u.username, t.start_at, t.due_at, t.weight, t.progress, t.custom_fields, t.completed_at
			FROM tasks t
			LEFT JOIN users u ON u.id = t.assignee_id
			WHERE t.list_id=$1 AND t.archived_at IS NULL
			ORDER BY t.position, t.id`, lr.id)
		if err != nil {
			dbFail(r, "export tasks", err)
			continue
		}
		var taskIDs []int64
		for taskRows.Next() {
			var et exportTask
			if taskRows.Scan(&et.Ref, &et.ParentRef, &et.Title, &et.Description, &et.Status, &et.Priority,
				&et.Assignee, &et.StartAt, &et.DueAt, &et.Weight, &et.Progress, &et.CustomFields,
				&et.CompletedAt) == nil {
				el.Tasks = append(el.Tasks, et)
				taskIDs = append(taskIDs, et.Ref)
			}
		}
		taskRows.Close()

		// Comments and attachment metadata for those tasks.
		for i := range el.Tasks {
			tid := el.Tasks[i].Ref
			cRows, err := a.DB.Pool.Query(r.Context(), `
				SELECT u.username, c.body, c.is_system, c.created_at
				FROM comments c JOIN users u ON u.id = c.author_id
				WHERE c.task_id=$1 AND c.deleted_at IS NULL ORDER BY c.created_at`, tid)
			if err == nil {
				for cRows.Next() {
					var ec exportComment
					if cRows.Scan(&ec.Author, &ec.Body, &ec.IsSystem, &ec.CreatedAt) == nil {
						el.Tasks[i].Comments = append(el.Tasks[i].Comments, ec)
					}
				}
				cRows.Close()
			}
			aRows, err := a.DB.Pool.Query(r.Context(),
				`SELECT file_path, mime_type, size_bytes FROM attachments WHERE target_type='task' AND target_id=$1`, tid)
			if err == nil {
				for aRows.Next() {
					var ea exportAttachment
					if aRows.Scan(&ea.FilePath, &ea.MimeType, &ea.SizeBytes) == nil {
						el.Tasks[i].Attachments = append(el.Tasks[i].Attachments, ea)
					}
				}
				aRows.Close()
			}
		}
		_ = taskIDs
		out.Lists = append(out.Lists, el)
	}

	// Notes attached to the space.
	nRows, err := a.DB.Pool.Query(r.Context(),
		`SELECT title, body FROM notes WHERE space_id=$1 AND archived_at IS NULL ORDER BY id`, spaceID)
	if err == nil {
		for nRows.Next() {
			var en exportNote
			if nRows.Scan(&en.Title, &en.Body) == nil {
				out.Notes = append(out.Notes, en)
			}
		}
		nRows.Close()
	}

	// Content-Disposition so a browser saves it as a file instead of rendering JSON in a tab.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=\"todorio-space-%d.json\"", spaceID))
	_ = json.NewEncoder(w).Encode(out)
}

// POST /api/spaces/import — create a NEW space from an exported document.
//
// Always creates a new space rather than merging into an existing one. A merge would have to
// guess whether a same-named list is the same list, and guessing wrong means silently mangling
// real data; a fresh space is unambiguous and the user can move things afterwards.
func (a *API) handleImportSpace(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	// Importing creates a space, so it needs the same permission as creating one.
	if !u.IsAdmin() && a.DB.Setting(r.Context(), "policy.users.can_create_spaces", "true") == "false" {
		errJSON(w, http.StatusForbidden, "you are not allowed to create spaces")
		return
	}

	var in spaceExport
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20)) // 32 MB ceiling
	if err := dec.Decode(&in); err != nil {
		errJSON(w, http.StatusBadRequest, "could not read the import file: "+err.Error())
		return
	}
	if in.FormatVersion != exportFormatVersion {
		errJSON(w, http.StatusBadRequest,
			fmt.Sprintf("unsupported export format %d (this server reads %d)", in.FormatVersion, exportFormatVersion))
		return
	}
	if in.SpaceName == "" {
		errJSON(w, http.StatusBadRequest, "the import file has no space name")
		return
	}

	// One transaction: a partially imported space is worse than a failed import, because the
	// user cannot tell what is missing.
	tx, err := a.DB.Pool.Begin(r.Context())
	if err != nil {
		dbFail(r, "import begin", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	settings := "{}"
	if len(in.Settings) > 0 {
		settings = string(in.Settings)
	}
	var spaceID int64
	if err := tx.QueryRow(r.Context(),
		`INSERT INTO spaces(name, owner_id, settings) VALUES($1,$2,$3::jsonb) RETURNING id`,
		in.SpaceName+" (imported)", u.ID, settings).Scan(&spaceID); err != nil {
		dbFail(r, "import space", err)
		errJSON(w, http.StatusInternalServerError, "could not create the space")
		return
	}
	if _, err := tx.Exec(r.Context(),
		`INSERT INTO space_members(space_id,user_id,role) VALUES($1,$2,'owner')`, spaceID, u.ID); err != nil {
		dbFail(r, "import space member", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}

	// Usernames in the file are resolved against THIS server. An assignee who doesn't exist here
	// is dropped rather than invented — the task arrives unassigned, which is honest.
	resolve := func(name *string) *int64 {
		if name == nil || *name == "" {
			return nil
		}
		var id int64
		if tx.QueryRow(r.Context(),
			`SELECT id FROM users WHERE username=$1 AND status='active'`, *name).Scan(&id) != nil {
			return nil
		}
		return &id
	}

	counts := map[string]int{"lists": 0, "tasks": 0, "comments": 0, "notes": 0}
	for _, el := range in.Lists {
		var listID int64
		if err := tx.QueryRow(r.Context(),
			`INSERT INTO lists(space_id, name, is_private) VALUES($1,$2,$3) RETURNING id`,
			spaceID, el.Name, el.IsPrivate).Scan(&listID); err != nil {
			dbFail(r, "import list", err)
			errJSON(w, http.StatusInternalServerError, "could not import a list")
			return
		}
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO list_members(list_id,user_id,permission) VALUES($1,$2,'owner')`, listID, u.ID); err != nil {
			dbFail(r, "import list member", err)
			errJSON(w, http.StatusInternalServerError, "database error")
			return
		}
		counts["lists"]++

		// Parents before children, so a subtask's parent_id can be remapped. Two passes over the
		// list's tasks: roots first, then everything that referenced one.
		newID := map[int64]int64{}
		for pass := 0; pass < 2; pass++ {
			for _, et := range el.Tasks {
				isRoot := et.ParentRef == nil
				if (pass == 0) != isRoot {
					continue
				}
				var parent *int64
				if et.ParentRef != nil {
					if mapped, ok := newID[*et.ParentRef]; ok {
						parent = &mapped
					} // an unresolvable parent becomes a root task rather than being dropped
				}
				cf := "{}"
				if len(et.CustomFields) > 0 {
					cf = string(et.CustomFields)
				}
				var tid int64
				if err := tx.QueryRow(r.Context(), `
					INSERT INTO tasks(list_id, parent_id, title, description, status, priority,
						assignee_id, start_at, due_at, weight, progress, custom_fields, completed_at, creator_id)
					VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13,$14) RETURNING id`,
					listID, parent, et.Title, et.Description, et.Status, et.Priority,
					resolve(et.Assignee), et.StartAt, et.DueAt, maxInt(et.Weight, 1), et.Progress,
					cf, et.CompletedAt, u.ID).Scan(&tid); err != nil {
					dbFail(r, "import task", err)
					errJSON(w, http.StatusInternalServerError, "could not import a task")
					return
				}
				newID[et.Ref] = tid
				counts["tasks"]++

				for _, ec := range et.Comments {
					author := resolve(&ec.Author)
					if author == nil {
						author = &u.ID // unknown author: attributed to the importer, never dropped
					}
					if _, err := tx.Exec(r.Context(),
						`INSERT INTO comments(task_id, author_id, body, is_system, created_at) VALUES($1,$2,$3,$4,$5)`,
						tid, *author, ec.Body, ec.IsSystem, ec.CreatedAt); err == nil {
						counts["comments"]++
					}
				}
			}
		}
	}

	for _, en := range in.Notes {
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO notes(space_id, title, body, created_by) VALUES($1,$2,$3,$4)`,
			spaceID, en.Title, en.Body, u.ID); err == nil {
			counts["notes"]++
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		dbFail(r, "import commit", err)
		errJSON(w, http.StatusInternalServerError, "could not save the import")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"space_id": spaceID, "imported": counts})
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
