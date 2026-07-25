package api

// Global search across tasks, notes, and comments the user has access to.
//
// Two passes. The first uses the tsvector columns and GIN indexes added in migration 0013 and
// is the one that runs in practice. The second is the original ILIKE scan, kept as a fallback
// for the cases full-text search genuinely cannot serve:
//
//   - a match in the middle of a word ("deploy" inside "redeployment"), since a prefix query
//     anchors at token start;
//   - languages the 'simple' configuration does not segment into words, notably Chinese and
//     Japanese, where the whole run of text becomes one token;
//   - a query that survives sanitisation as nothing at all (punctuation, emoji).
//
// The fallback only runs when the indexed pass found nothing, so the slow path costs a
// sequential scan exactly in the situations that used to require one anyway — and results
// stay identical to the old behaviour rather than quietly getting narrower.

import (
	"net/http"
	"strings"
	"unicode"

	"github.com/DanMotive/Todorio/internal/auth"
)

const (
	searchLimit = 20
	// Postgres parses the whole tsquery before matching anything, so an absurdly long query is
	// work done for nothing. Ten terms is far past what anyone types into a search box.
	searchMaxTerms = 10
)

// searchTSQuery converts raw user input into a to_tsquery expression.
//
// Everything that is not a letter or a digit becomes a separator, which does double duty: it
// splits the phrase into terms, and it strips every character tsquery treats as syntax
// (`&`, `|`, `!`, `<->`, parentheses, quotes). A user searching for "a & b" gets two terms,
// not a syntax error — and cannot inject operators into the query.
//
// Each term is turned into a prefix match. That is what compensates for indexing with the
// 'simple' configuration, which does no stemming: "задач" matches "задачи", "deploy"
// matches "deployment". Terms are ANDed, so more words narrow the result.
//
// Returns "" when the input contains no usable term; the caller then skips the indexed pass.
func searchTSQuery(q string) string {
	terms := []string{}
	var term strings.Builder
	flush := func() {
		if term.Len() > 0 && len(terms) < searchMaxTerms {
			terms = append(terms, term.String()+":*")
		}
		term.Reset()
	}
	for _, r := range q {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			term.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	if len(terms) == 0 {
		return ""
	}
	return strings.Join(terms, " & ")
}

// searchSQL holds the three queries of one search pass. The only difference between the
// full-text and the fallback variant is how $1 is matched and how rows are ordered, so the
// access-control clauses stay written once.
type searchSQL struct {
	tasks    string
	notes    string
	comments string
}

// $1 = tsquery, $2 = caller is admin, $3 = caller id.
var searchFTS = searchSQL{
	tasks: `
		SELECT t.id, t.list_id, t.title
		FROM tasks t
		WHERE t.archived_at IS NULL AND t.search_tsv @@ to_tsquery('simple', $1)
			AND ($2 OR t.list_id IN (SELECT list_id FROM list_members WHERE user_id=$3)
				OR t.list_id IN (SELECT id FROM lists WHERE is_private=false))
		ORDER BY ts_rank(t.search_tsv, to_tsquery('simple', $1)) DESC, t.updated_at DESC LIMIT $4`,
	notes: `
		SELECT n.id, n.space_id, n.title
		FROM notes n
		WHERE n.archived_at IS NULL AND n.search_tsv @@ to_tsquery('simple', $1)
			AND ($2 OR n.space_id IN (SELECT space_id FROM space_members WHERE user_id=$3))
		ORDER BY ts_rank(n.search_tsv, to_tsquery('simple', $1)) DESC, n.updated_at DESC LIMIT $4`,
	comments: `
		SELECT c.id, c.task_id, t.title, c.body
		FROM comments c JOIN tasks t ON t.id = c.task_id
		WHERE c.deleted_at IS NULL AND t.archived_at IS NULL AND c.search_tsv @@ to_tsquery('simple', $1)
			AND ($2 OR t.list_id IN (SELECT list_id FROM list_members WHERE user_id=$3)
				OR t.list_id IN (SELECT id FROM lists WHERE is_private=false))
		ORDER BY ts_rank(c.search_tsv, to_tsquery('simple', $1)) DESC, c.created_at DESC LIMIT $4`,
}

// $1 = LIKE pattern, $2 = caller is admin, $3 = caller id. The original queries, unchanged.
var searchLike = searchSQL{
	tasks: `
		SELECT t.id, t.list_id, t.title
		FROM tasks t
		WHERE t.archived_at IS NULL AND (t.title ILIKE $1 OR t.description ILIKE $1)
			AND ($2 OR t.list_id IN (SELECT list_id FROM list_members WHERE user_id=$3)
				OR t.list_id IN (SELECT id FROM lists WHERE is_private=false))
		ORDER BY t.updated_at DESC LIMIT $4`,
	notes: `
		SELECT n.id, n.space_id, n.title
		FROM notes n
		WHERE n.archived_at IS NULL AND (n.title ILIKE $1 OR n.body ILIKE $1)
			AND ($2 OR n.space_id IN (SELECT space_id FROM space_members WHERE user_id=$3))
		ORDER BY n.updated_at DESC LIMIT $4`,
	comments: `
		SELECT c.id, c.task_id, t.title, c.body
		FROM comments c JOIN tasks t ON t.id = c.task_id
		WHERE c.deleted_at IS NULL AND t.archived_at IS NULL AND c.body ILIKE $1
			AND ($2 OR t.list_id IN (SELECT list_id FROM list_members WHERE user_id=$3)
				OR t.list_id IN (SELECT id FROM lists WHERE is_private=false))
		ORDER BY c.created_at DESC LIMIT $4`,
}

// GET /api/search?q=...
func (a *API) handleSearch(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		errJSON(w, http.StatusBadRequest, "query must be at least 2 characters")
		return
	}

	results := []map[string]any{}
	if tsq := searchTSQuery(q); tsq != "" {
		results = a.runSearch(r, u, searchFTS, tsq)
	}
	if len(results) == 0 {
		results = a.runSearch(r, u, searchLike, "%"+q+"%")
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// runSearch executes one pass over the three searchable kinds. A failing query yields no rows
// of that kind rather than failing the whole search, which is how this endpoint always behaved.
func (a *API) runSearch(r *http.Request, u *auth.User, sql searchSQL, match string) []map[string]any {
	ctx := r.Context()
	results := []map[string]any{}

	taskRows, err := a.DB.Pool.Query(ctx, sql.tasks, match, u.IsAdmin(), u.ID, searchLimit)
	if err == nil {
		for taskRows.Next() {
			var id, listID int64
			var title string
			if taskRows.Scan(&id, &listID, &title) == nil {
				results = append(results, map[string]any{"type": "task", "id": id, "list_id": listID, "title": title})
			}
		}
		taskRows.Close()
	} else {
		dbFail(r, "search tasks", err)
	}

	noteRows, err := a.DB.Pool.Query(ctx, sql.notes, match, u.IsAdmin(), u.ID, searchLimit)
	if err == nil {
		for noteRows.Next() {
			var id, spaceID int64
			var title string
			if noteRows.Scan(&id, &spaceID, &title) == nil {
				results = append(results, map[string]any{"type": "note", "id": id, "space_id": spaceID, "title": title})
			}
		}
		noteRows.Close()
	} else {
		dbFail(r, "search notes", err)
	}

	commentRows, err := a.DB.Pool.Query(ctx, sql.comments, match, u.IsAdmin(), u.ID, searchLimit)
	if err == nil {
		for commentRows.Next() {
			var id, taskID int64
			var taskTitle, body string
			if commentRows.Scan(&id, &taskID, &taskTitle, &body) == nil {
				results = append(results, map[string]any{
					"type": "comment", "id": id, "task_id": taskID, "task_title": taskTitle, "snippet": body,
				})
			}
		}
		commentRows.Close()
	} else {
		dbFail(r, "search comments", err)
	}

	return results
}
