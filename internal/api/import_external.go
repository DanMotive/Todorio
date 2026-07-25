package api

// Importing from outside Todorio.
//
// /api/spaces/import has always accepted exactly one thing: a JSON file that Todorio itself
// produced. That is fine for moving a space between installations and useless for the actual
// first-day problem, which is that everything the team owns is currently in a Trello board or a
// spreadsheet. An import that only reads its own output is a migration path from nowhere.
//
// Rather than write a second inserter, both new formats are translated into the same spaceExport
// document the JSON importer already consumes, and then handed to it. That importer is the piece
// with the transaction, the parent/child remapping, the username resolution and the space-quota
// check; duplicating any of that here would mean two code paths that drift apart, and the CSV one
// would be the one nobody remembers to fix.

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Import bodies are read directly rather than through readJSON: maxJSONBody is 1 MB, which a
// real Trello export blows past easily. These limits match the 32 MB the JSON importer already
// allows for its own format.
const (
	maxCSVImportBytes    = 8 << 20
	maxTrelloImportBytes = 32 << 20
	// maxImportRows stops a single request from creating an unbounded number of tasks. It is a
	// safety valve, not a product limit - a board this size is almost certainly a mistake.
	maxImportRows = 5000
)

// deliverImport hands a translated document to the existing JSON importer.
//
// The request is cloned so the session, the context and the client address all survive; only the
// body is swapped. From handleImportSpace's point of view the caller uploaded a Todorio export,
// which means every permission and quota check it performs applies unchanged to CSV and Trello.
func (a *API) deliverImport(w http.ResponseWriter, r *http.Request, doc spaceExport) {
	doc.FormatVersion = exportFormatVersion
	if doc.ExportedAt.IsZero() {
		doc.ExportedAt = time.Now().UTC()
	}
	if strings.TrimSpace(doc.SpaceName) == "" {
		doc.SpaceName = "Imported"
	}
	buf, err := json.Marshal(doc)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "could not prepare the import")
		return
	}
	r2 := r.Clone(r.Context())
	r2.Body = io.NopCloser(bytes.NewReader(buf))
	r2.ContentLength = int64(len(buf))
	r2.Header.Set("Content-Type", "application/json")
	a.handleImportSpace(w, r2)
}

// csvColumn maps a header cell to a field. Header names are matched loosely because the file is
// usually an export from someone else's tool or a hand-made spreadsheet, and rejecting a file
// over "Due date" versus "due_at" would be a pointless way to fail.
func csvColumn(header string) string {
	h := strings.ToLower(strings.TrimSpace(header))
	h = strings.ReplaceAll(h, " ", "_")
	switch h {
	case "list", "list_name", "column", "board_list", "category":
		return "list"
	case "title", "name", "task", "summary", "card_name":
		return "title"
	case "description", "desc", "notes", "body", "details":
		return "description"
	case "status", "state":
		return "status"
	case "priority":
		return "priority"
	case "assignee", "owner", "assigned_to", "member", "members":
		return "assignee"
	case "due", "due_at", "due_date", "deadline":
		return "due"
	case "start", "start_at", "start_date":
		return "start"
	case "weight", "estimate", "points", "size":
		return "weight"
	case "done", "completed", "is_done", "finished":
		return "done"
	}
	return ""
}

// parseImportDate accepts the handful of shapes spreadsheets and exports actually emit. A date
// that cannot be read is dropped rather than rejected: losing a deadline is recoverable, refusing
// the whole file because one row has a locale-formatted date is not.
func parseImportDate(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"02.01.2006 15:04",
		"02.01.2006",
		"01/02/2006",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, raw); err == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}

// truthy reads the many ways a spreadsheet says yes.
func truthy(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "done", "x", "da", "da.":
		return true
	}
	return false
}

// csvToExport turns a parsed CSV table into the internal document. Kept separate from the
// handler, and free of any request or database access, so the column-mapping rules can be tested
// directly - they are the part most likely to need adjusting later.
func csvToExport(records [][]string, spaceName string) (spaceExport, string) {
	doc := spaceExport{SpaceName: spaceName}
	if len(records) < 2 {
		return doc, "the file has no data rows"
	}
	cols := map[string]int{}
	for i, h := range records[0] {
		if key := csvColumn(h); key != "" {
			if _, seen := cols[key]; !seen {
				cols[key] = i
			}
		}
	}
	if _, ok := cols["title"]; !ok {
		return doc, "no title column found - the first row must name the columns, including one called title or name"
	}

	get := func(row []string, key string) string {
		i, ok := cols[key]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	// Lists are kept in first-seen order so the imported space reads like the source file rather
	// than like a map iteration.
	order := []string{}
	byName := map[string]*exportList{}
	var ref int64
	for _, row := range records[1:] {
		title := get(row, "title")
		if title == "" {
			continue
		}
		if ref >= maxImportRows {
			break
		}
		listName := get(row, "list")
		if listName == "" {
			listName = "Imported"
		}
		if _, ok := byName[listName]; !ok {
			byName[listName] = &exportList{Name: listName}
			order = append(order, listName)
		}
		ref++
		status := get(row, "status")
		done := truthy(get(row, "done")) || strings.EqualFold(status, "done")
		if status == "" {
			if done {
				status = "done"
			} else {
				status = "open"
			}
		}
		t := exportTask{
			Ref:         ref,
			Title:       title,
			Description: get(row, "description"),
			Status:      status,
			Priority:    strings.ToLower(get(row, "priority")),
			DueAt:       parseImportDate(get(row, "due")),
			StartAt:     parseImportDate(get(row, "start")),
			Weight:      1,
		}
		if wRaw := get(row, "weight"); wRaw != "" {
			if n, err := strconv.Atoi(wRaw); err == nil && n > 0 && n < 1000 {
				t.Weight = n
			}
		}
		// The assignee is carried as a username and resolved by the JSON importer, which only
		// matches active accounts and leaves the task unassigned otherwise. An unknown name in a
		// spreadsheet must never silently land work on a real person with a similar login.
		if who := get(row, "assignee"); who != "" {
			name := strings.TrimPrefix(who, "@")
			t.Assignee = &name
		}
		if done {
			now := time.Now().UTC()
			t.CompletedAt = &now
		}
		byName[listName].Tasks = append(byName[listName].Tasks, t)
	}
	if ref == 0 {
		return doc, "no rows with a title were found"
	}
	for _, name := range order {
		doc.Lists = append(doc.Lists, *byName[name])
	}
	return doc, ""
}

// POST /api/import/csv?name=Space%20name
//
// The body is the raw file. Accepting it as a plain body rather than wrapped in JSON means the
// browser can post the File object straight through, and a user with a terminal can pipe the file
// in without escaping it first.
func (a *API) handleImportCSV(w http.ResponseWriter, r *http.Request) {
	if a.requireUser(w, r) == nil {
		return
	}
	body := http.MaxBytesReader(w, r.Body, maxCSVImportBytes)
	reader := csv.NewReader(body)
	// Rows in real files disagree about how many columns they have; short and long rows are
	// handled per-field instead of being treated as corruption.
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	records, err := reader.ReadAll()
	if err != nil {
		errJSON(w, http.StatusBadRequest, "could not read the CSV file")
		return
	}
	doc, problem := csvToExport(records, strings.TrimSpace(r.URL.Query().Get("name")))
	if problem != "" {
		errJSON(w, http.StatusBadRequest, problem)
		return
	}
	a.deliverImport(w, r, doc)
}

// trelloBoard is the subset of a Trello board export that maps onto anything here. Everything
// else in that file (power-ups, actions, plugin data, colour schemes) has no counterpart and is
// ignored rather than stuffed into descriptions.
type trelloBoard struct {
	Name  string `json:"name"`
	Lists []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Closed bool   `json:"closed"`
		Pos    float64 `json:"pos"`
	} `json:"lists"`
	Cards []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Desc   string `json:"desc"`
		IDList string `json:"idList"`
		Closed bool   `json:"closed"`
		Due    *time.Time `json:"due"`
		Start  *time.Time `json:"start"`
		DueComplete bool  `json:"dueComplete"`
		Pos    float64 `json:"pos"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	} `json:"cards"`
	Checklists []struct {
		IDCard     string `json:"idCard"`
		Name       string `json:"name"`
		CheckItems []struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"checkItems"`
	} `json:"checklists"`
}

// trelloToExport maps a board onto the internal document.
//
// Trello lists become lists, cards become tasks, and checklist items become subtasks of their
// card - the one structural idea the two tools genuinely share. Archived lists and cards are
// skipped: they are the other tool's trash, and importing someone's trash into a fresh space is
// never what they meant.
func trelloToExport(b trelloBoard, spaceName string) (spaceExport, string) {
	if spaceName == "" {
		spaceName = strings.TrimSpace(b.Name)
	}
	doc := spaceExport{SpaceName: spaceName}

	// Checklist items are grouped by card up front so each card can pick up its own without
	// rescanning the whole board.
	itemsByCard := map[string][]struct {
		Title string
		Done  bool
	}{}
	for _, cl := range b.Checklists {
		for _, item := range cl.CheckItems {
			name := strings.TrimSpace(item.Name)
			if name == "" {
				continue
			}
			itemsByCard[cl.IDCard] = append(itemsByCard[cl.IDCard], struct {
				Title string
				Done  bool
			}{name, strings.EqualFold(item.State, "complete")})
		}
	}

	listIndex := map[string]int{}
	for _, l := range b.Lists {
		if l.Closed {
			continue
		}
		name := strings.TrimSpace(l.Name)
		if name == "" {
			name = "Imported"
		}
		listIndex[l.ID] = len(doc.Lists)
		doc.Lists = append(doc.Lists, exportList{Name: name})
	}
	if len(doc.Lists) == 0 {
		return doc, "the board has no open lists"
	}

	var ref int64
	var cards int
	for _, c := range b.Cards {
		if c.Closed || ref >= maxImportRows {
			continue
		}
		idx, ok := listIndex[c.IDList]
		if !ok {
			continue
		}
		title := strings.TrimSpace(c.Name)
		if title == "" {
			continue
		}
		ref++
		cardRef := ref
		desc := c.Desc
		// Labels have no equivalent yet, so they are preserved as a line of text rather than
		// dropped. Losing them silently would make the import look lossless when it is not.
		var labels []string
		for _, l := range c.Labels {
			if n := strings.TrimSpace(l.Name); n != "" {
				labels = append(labels, n)
			}
		}
		if len(labels) > 0 {
			if desc != "" {
				desc += "\n\n"
			}
			desc += "Trello labels: " + strings.Join(labels, ", ")
		}
		status := "open"
		t := exportTask{
			Ref:         cardRef,
			Title:       title,
			Description: desc,
			Status:      status,
			DueAt:       c.Due,
			StartAt:     c.Start,
			Weight:      1,
		}
		// dueComplete is Trello's only per-card "finished" flag; a card in a list called Done is
		// still just a card in a list, and guessing from list names would be wrong as often as
		// right.
		if c.DueComplete {
			t.Status = "done"
			now := time.Now().UTC()
			t.CompletedAt = &now
		}
		doc.Lists[idx].Tasks = append(doc.Lists[idx].Tasks, t)
		cards++

		for _, item := range itemsByCard[c.ID] {
			if ref >= maxImportRows {
				break
			}
			ref++
			parent := cardRef
			sub := exportTask{
				Ref:       ref,
				ParentRef: &parent,
				Title:     item.Title,
				Status:    "open",
				Weight:    1,
			}
			if item.Done {
				sub.Status = "done"
				now := time.Now().UTC()
				sub.CompletedAt = &now
			}
			doc.Lists[idx].Tasks = append(doc.Lists[idx].Tasks, sub)
		}
	}
	if cards == 0 {
		return doc, "the board has no open cards"
	}
	return doc, ""
}

// POST /api/import/trello?name=Space%20name
//
// Takes the board JSON that Trello's own "Export as JSON" produces, unmodified.
func (a *API) handleImportTrello(w http.ResponseWriter, r *http.Request) {
	if a.requireUser(w, r) == nil {
		return
	}
	body := http.MaxBytesReader(w, r.Body, maxTrelloImportBytes)
	var board trelloBoard
	dec := json.NewDecoder(body)
	if err := dec.Decode(&board); err != nil {
		errJSON(w, http.StatusBadRequest, "could not read the Trello export")
		return
	}
	doc, problem := trelloToExport(board, strings.TrimSpace(r.URL.Query().Get("name")))
	if problem != "" {
		errJSON(w, http.StatusBadRequest, problem)
		return
	}
	a.deliverImport(w, r, doc)
}
