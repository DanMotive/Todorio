package api

// Tests for the pure parts of the fourth batch of features: note extraction, the two foreign
// import formats, and personal bot token shape. Everything here is deliberately request- and
// database-free - these are the rules most likely to be adjusted later, and they should be
// checkable without a Postgres instance.

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseNoteTaskLinesTakesOnlyUncheckedBoxes(t *testing.T) {
	body := "# Meeting\n\nSome prose that is not a task.\n\n" +
		"- [ ] call the supplier\n" +
		"- [x] already done, must be skipped\n" +
		"- [X] also done\n" +
		"* [ ] second one with a star bullet\n" +
		"+ [ ]   spaces around the title   \n" +
		"- a plain bullet, which is context and not a commitment\n" +
		"- [ ]\n" // empty title: nothing to create

	got := parseNoteTaskLines(body)
	want := []string{"call the supplier", "second one with a star bullet", "spaces around the title"}
	if len(got) != len(want) {
		t.Fatalf("got %d titles %q, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("title %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseNoteTaskLinesEmptyNote(t *testing.T) {
	if got := parseNoteTaskLines(""); len(got) != 0 {
		t.Fatalf("expected nothing from an empty note, got %q", got)
	}
	if got := parseNoteTaskLines("just prose\nand more prose"); len(got) != 0 {
		t.Fatalf("expected nothing from prose, got %q", got)
	}
}

func TestParseNoteTaskLinesRespectsCap(t *testing.T) {
	body := ""
	for i := 0; i < noteTaskMax+50; i++ {
		body += "- [ ] task\n"
	}
	if got := parseNoteTaskLines(body); len(got) != noteTaskMax {
		t.Fatalf("got %d titles, want the cap %d", len(got), noteTaskMax)
	}
}

func TestCSVColumnAliases(t *testing.T) {
	cases := map[string]string{
		"Title": "title", "name": "title", "Task": "title",
		"Due Date": "due", "due_at": "due", "DEADLINE": "due",
		"Assigned To": "assignee", "owner": "assignee",
		"List": "list", "column": "list",
		"something else": "",
	}
	for header, want := range cases {
		if got := csvColumn(header); got != want {
			t.Errorf("csvColumn(%q) = %q, want %q", header, got, want)
		}
	}
}

func TestParseImportDateFormats(t *testing.T) {
	for _, raw := range []string{
		"2026-03-04T10:00:00Z",
		"2026-03-04 10:00",
		"2026-03-04",
		"04.03.2026",
	} {
		got := parseImportDate(raw)
		if got == nil {
			t.Errorf("parseImportDate(%q) = nil, want a date", raw)
			continue
		}
		if got.Year() != 2026 || got.Month() != time.March || got.Day() != 4 {
			t.Errorf("parseImportDate(%q) = %v, want 2026-03-04", raw, got)
		}
	}
	// An unreadable date is dropped, not fatal: one odd cell must not cost the whole file.
	if got := parseImportDate("next tuesday"); got != nil {
		t.Errorf("expected nil for unparseable input, got %v", got)
	}
	if got := parseImportDate("   "); got != nil {
		t.Errorf("expected nil for blank input, got %v", got)
	}
}

func TestCSVToExportGroupsByList(t *testing.T) {
	records := [][]string{
		{"List", "Title", "Description", "Assignee", "Due date", "Weight", "Done"},
		{"Backlog", "first", "desc one", "@vlad", "2026-03-04", "3", ""},
		{"Backlog", "second", "", "", "", "", "yes"},
		{"", "third with no list", "", "", "", "", ""},
		{"Backlog", "", "row with no title is skipped", "", "", "", ""},
	}
	doc, problem := csvToExport(records, "Board")
	if problem != "" {
		t.Fatalf("unexpected problem: %s", problem)
	}
	if doc.SpaceName != "Board" {
		t.Errorf("space name = %q", doc.SpaceName)
	}
	if len(doc.Lists) != 2 {
		t.Fatalf("got %d lists, want 2 (Backlog + the default)", len(doc.Lists))
	}
	if doc.Lists[0].Name != "Backlog" || doc.Lists[1].Name != "Imported" {
		t.Errorf("list order/names = %q, %q", doc.Lists[0].Name, doc.Lists[1].Name)
	}
	backlog := doc.Lists[0].Tasks
	if len(backlog) != 2 {
		t.Fatalf("got %d tasks in Backlog, want 2", len(backlog))
	}
	if backlog[0].Weight != 3 {
		t.Errorf("weight = %d, want 3", backlog[0].Weight)
	}
	if backlog[0].Assignee == nil || *backlog[0].Assignee != "vlad" {
		t.Errorf("assignee not carried as a bare username: %v", backlog[0].Assignee)
	}
	if backlog[0].DueAt == nil {
		t.Error("due date not parsed")
	}
	if backlog[1].Status != "done" || backlog[1].CompletedAt == nil {
		t.Errorf("a done row must arrive completed: status=%q completed=%v", backlog[1].Status, backlog[1].CompletedAt)
	}
	// Refs must be unique across the whole document - the importer remaps parents by them.
	seen := map[int64]bool{}
	for _, l := range doc.Lists {
		for _, task := range l.Tasks {
			if seen[task.Ref] {
				t.Fatalf("duplicate ref %d", task.Ref)
			}
			seen[task.Ref] = true
		}
	}
}

func TestCSVToExportRejectsFileWithoutTitleColumn(t *testing.T) {
	_, problem := csvToExport([][]string{{"foo", "bar"}, {"1", "2"}}, "")
	if problem == "" {
		t.Fatal("expected a complaint about the missing title column")
	}
	_, problem = csvToExport([][]string{{"title"}}, "")
	if problem == "" {
		t.Fatal("expected a complaint about a header-only file")
	}
}

func TestTrelloToExportMapsCardsAndChecklists(t *testing.T) {
	// Built from JSON rather than Go literals: this is the shape Trello actually exports, so the
	// field tags are exercised too, and the test does not have to restate the anonymous structs.
	const raw = `{
		"name": "Roadmap",
		"lists": [
			{"id": "L1", "name": "Doing", "closed": false},
			{"id": "L2", "name": "Archived list", "closed": true}
		],
		"cards": [
			{"id": "C1", "name": "ship it", "desc": "body", "idList": "L1", "closed": false,
			 "labels": [{"name": "urgent"}]},
			{"id": "C2", "name": "archived card", "idList": "L1", "closed": true},
			{"id": "C3", "name": "card in a closed list", "idList": "L2", "closed": false}
		],
		"checklists": [
			{"idCard": "C1", "name": "steps", "checkItems": [
				{"name": "step one", "state": "complete"},
				{"name": "step two", "state": "incomplete"}
			]}
		]
	}`
	var board trelloBoard
	if err := json.Unmarshal([]byte(raw), &board); err != nil {
		t.Fatalf("the export shape no longer parses: %v", err)
	}

	doc, problem := trelloToExport(board, "")
	if problem != "" {
		t.Fatalf("unexpected problem: %s", problem)
	}
	if doc.SpaceName != "Roadmap" {
		t.Errorf("space name = %q, want the board name", doc.SpaceName)
	}
	if len(doc.Lists) != 1 || doc.Lists[0].Name != "Doing" {
		t.Fatalf("closed lists must be skipped, got %+v", doc.Lists)
	}
	tasks := doc.Lists[0].Tasks
	if len(tasks) != 3 {
		t.Fatalf("got %d tasks, want the card plus its two checklist items", len(tasks))
	}
	if tasks[0].Title != "ship it" || tasks[0].ParentRef != nil {
		t.Errorf("first task should be the card itself: %+v", tasks[0])
	}
	for _, sub := range tasks[1:] {
		if sub.ParentRef == nil || *sub.ParentRef != tasks[0].Ref {
			t.Errorf("checklist item %q is not parented to the card", sub.Title)
		}
	}
	if tasks[1].Status != "done" {
		t.Errorf("a complete check item should arrive done, got %q", tasks[1].Status)
	}
	if tasks[2].Status != "open" {
		t.Errorf("an incomplete check item should arrive open, got %q", tasks[2].Status)
	}
}

func TestTrelloToExportEmptyBoard(t *testing.T) {
	if _, problem := trelloToExport(trelloBoard{}, ""); problem == "" {
		t.Fatal("expected a complaint about a board with no lists")
	}
}

func TestLooksLikeBotToken(t *testing.T) {
	good := "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"
	if !looksLikeBotToken(good) {
		t.Errorf("a real-shaped token was rejected")
	}
	bad := []string{
		"",
		"nonsense",
		"123456789",
		":AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw",
		"abc:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw",
		"123456789:short",
		"123456789:AAHdqTcvCH1vGWJxfSeof SAs0K5PALDsaw",
		"https://t.me/mybot?start=123456789:AAHdqTcvCH1vGWJxfSeofSAs0",
	}
	for _, tok := range bad {
		if looksLikeBotToken(tok) {
			t.Errorf("accepted a bad token: %q", tok)
		}
	}
}

func TestEvenShare(t *testing.T) {
	if got := evenShare(10, 0); got != 0 {
		t.Errorf("no people should give no reference line, got %d", got)
	}
	if got := evenShare(10, 4); got != 2 {
		t.Errorf("evenShare(10,4) = %d, want 2", got)
	}
}
