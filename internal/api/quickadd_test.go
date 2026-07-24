package api

import (
	"testing"
	"time"
)

// resolver stands in for the database lookup: "vlad" and "alex" are assignable, nobody else is.
func testResolver(name string) (int64, bool) {
	switch name {
	case "vlad":
		return 7, true
	case "alex":
		return 9, true
	}
	return 0, false
}

func TestParseQuickAddTitleAndTokens(t *testing.T) {
	got := parseQuickAdd("Купить домен #infra !high @vlad", testResolver)

	if got.Title != "Купить домен" {
		t.Errorf("title = %q, want %q", got.Title, "Купить домен")
	}
	if len(got.Tags) != 1 || got.Tags[0] != "infra" {
		t.Errorf("tags = %v, want [infra]", got.Tags)
	}
	if got.Priority != "high" {
		t.Errorf("priority = %q, want high", got.Priority)
	}
	if got.AssigneeID == nil || *got.AssigneeID != 7 {
		t.Errorf("assignee = %v, want 7", got.AssigneeID)
	}
}

// The parser must never silently delete text it couldn't resolve — the user would end up with
// a title different from what they typed and no indication why.
func TestParseQuickAddKeepsUnresolvedTokens(t *testing.T) {
	got := parseQuickAdd("Ping @nobody about !bogus thing", testResolver)

	if got.AssigneeID != nil {
		t.Errorf("assignee should be unset, got %v", got.AssigneeID)
	}
	if got.Priority != "" {
		t.Errorf("priority should be unset, got %q", got.Priority)
	}
	if got.Title != "Ping @nobody about !bogus thing" {
		t.Errorf("title = %q, unresolved tokens must survive verbatim", got.Title)
	}
	if len(got.Unresolved) != 1 || got.Unresolved[0] != "nobody" {
		t.Errorf("unresolved = %v, want [nobody]", got.Unresolved)
	}
}

func TestParseQuickAddRelativeDates(t *testing.T) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	for _, tc := range []struct {
		in       string
		wantDays int
	}{
		{"Позвонить сегодня", 0},
		{"Call the bank tomorrow", 1},
		{"Отчёт послезавтра", 2},
	} {
		got := parseQuickAdd(tc.in, testResolver)
		if got.DueAt == nil {
			t.Fatalf("%q: no due date parsed", tc.in)
		}
		wantDay := today.AddDate(0, 0, tc.wantDays)
		if got.DueAt.Year() != wantDay.Year() || got.DueAt.YearDay() != wantDay.YearDay() {
			t.Errorf("%q: due = %v, want the day %v", tc.in, got.DueAt, wantDay)
		}
		// A deadline should land at the end of its day, not midnight at the start, or the task
		// reads as overdue for the whole day it's actually due.
		if got.DueAt.Hour() != 23 {
			t.Errorf("%q: due hour = %d, want end of day", tc.in, got.DueAt.Hour())
		}
	}
}

// "monday" means the upcoming Monday, never today, even when typed on a Monday.
func TestParseQuickAddWeekdayIsAlwaysInFuture(t *testing.T) {
	got := parseQuickAdd("Standup monday", testResolver)
	if got.DueAt == nil {
		t.Fatal("no due date parsed")
	}
	if got.DueAt.Weekday() != time.Monday {
		t.Errorf("weekday = %v, want Monday", got.DueAt.Weekday())
	}
	if !got.DueAt.After(time.Now()) {
		t.Errorf("due %v should be in the future", got.DueAt)
	}
	if got.Title != "Standup" {
		t.Errorf("title = %q, want %q", got.Title, "Standup")
	}
}

func TestParseQuickAddExplicitDates(t *testing.T) {
	got := parseQuickAdd("Release 2027-03-14", testResolver)
	if got.DueAt == nil {
		t.Fatal("ISO date not parsed")
	}
	if got.DueAt.Year() != 2027 || got.DueAt.Month() != time.March || got.DueAt.Day() != 14 {
		t.Errorf("due = %v, want 2027-03-14", got.DueAt)
	}
	if got.Title != "Release" {
		t.Errorf("title = %q, want %q", got.Title, "Release")
	}

	// dd.mm.yyyy
	got = parseQuickAdd("Оплатить 05.09.2027", testResolver)
	if got.DueAt == nil || got.DueAt.Day() != 5 || got.DueAt.Month() != time.September || got.DueAt.Year() != 2027 {
		t.Errorf("dotted date = %v, want 2027-09-05", got.DueAt)
	}
	if got.Title != "Оплатить" {
		t.Errorf("title = %q", got.Title)
	}
}

// An impossible date must stay as literal text rather than rolling over into a real one:
// Go's time.Date would silently turn 31.02 into 03.03.
func TestParseQuickAddRejectsImpossibleDate(t *testing.T) {
	got := parseQuickAdd("Check 31.02", testResolver)
	if got.DueAt != nil {
		t.Errorf("31.02 should not parse, got %v", got.DueAt)
	}
	if got.Title != "Check 31.02" {
		t.Errorf("title = %q, want the text left alone", got.Title)
	}
}

// A version number is not a date.
func TestParseQuickAddIgnoresVersionLikeText(t *testing.T) {
	got := parseQuickAdd("Bump to v1.2.3 today", testResolver)
	if got.DueAt == nil {
		t.Fatal("'today' should still be parsed")
	}
	if got.Title != "Bump to v1.2.3" {
		t.Errorf("title = %q, want the version preserved", got.Title)
	}
}

// Only the first @mention becomes the assignee; a second stays as text.
func TestParseQuickAddFirstMentionWins(t *testing.T) {
	got := parseQuickAdd("Sync @vlad and @alex", testResolver)
	if got.AssigneeID == nil || *got.AssigneeID != 7 {
		t.Fatalf("assignee = %v, want vlad (7)", got.AssigneeID)
	}
	if got.Title != "Sync and @alex" {
		t.Errorf("title = %q, want the second mention kept", got.Title)
	}
}

func TestParseQuickAddMultipleTags(t *testing.T) {
	got := parseQuickAdd("#infra #urgent-fix Обновить сертификат", testResolver)
	if len(got.Tags) != 2 || got.Tags[0] != "infra" || got.Tags[1] != "urgent-fix" {
		t.Errorf("tags = %v, want [infra urgent-fix]", got.Tags)
	}
	if got.Title != "Обновить сертификат" {
		t.Errorf("title = %q", got.Title)
	}
}

// A line with no special tokens must survive completely untouched.
func TestParseQuickAddPlainText(t *testing.T) {
	const in = "Просто обычная задача без токенов"
	got := parseQuickAdd(in, testResolver)
	if got.Title != in {
		t.Errorf("title = %q, want %q", got.Title, in)
	}
	if got.DueAt != nil || got.Priority != "" || got.AssigneeID != nil || len(got.Tags) != 0 {
		t.Errorf("nothing should have been extracted: %+v", got)
	}
}
