package api

import "testing"

// captionCategory decides which set of phrases the statistics card shows. The bug it was
// rewritten for was silent: three of the seven categories shipped in migration 0010 could never
// be selected, so 312 of the 728 phrases were dead weight in the database. These tests pin both
// the individual rules and the property that every category stays reachable.

func TestCaptionCategoryRules(t *testing.T) {
	cases := []struct {
		name                string
		done, overdue, open int
		want                string
	}{
		// Missing more than you finish outranks everything else.
		{"more overdue than done", 2, 5, 10, "overdue"},
		{"overdue with nothing done", 0, 1, 3, "overdue"},
		{"overdue equal to done is not overdue", 5, 5, 0, "success"},

		// perfect_day claims an empty board, so it requires one.
		{"cleared the board", 4, 0, 0, "perfect_day"},
		{"cleared a big board", 30, 0, 0, "perfect_day"},
		{"empty board but nothing done", 0, 0, 0, "inactive"},
		{"would be perfect but one task is overdue", 4, 1, 0, "overdue"},

		// A quiet period is not a failure.
		{"nothing done, work waiting", 0, 0, 12, "inactive"},

		// High output with work still open.
		{"lots done, lots left", 12, 0, 40, "success"},

		// Steady progress that outpaces what remains.
		{"done outpaces open", 6, 0, 4, "project"},
		{"done equals open", 5, 0, 5, "project"},

		// Progress exists but the backlog dominates.
		{"backlog dominates", 4, 0, 30, "focus"},
		{"three done against a big backlog", 3, 0, 100, "focus"},

		// Too little to characterise.
		{"one task", 1, 0, 9, "neutral"},
		{"two tasks", 2, 0, 9, "neutral"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := captionCategory(tc.done, tc.overdue, tc.open)
			if got != tc.want {
				t.Fatalf("captionCategory(done=%d, overdue=%d, open=%d) = %q, want %q",
					tc.done, tc.overdue, tc.open, got, tc.want)
			}
		})
	}
}

// The regression guard: every category that has phrases in the database must be reachable from
// some plausible state. If a future edit to the thresholds strands one again, this fails.
func TestCaptionCategoryAllCategoriesReachable(t *testing.T) {
	stored := []string{"overdue", "perfect_day", "inactive", "success", "project", "focus", "neutral"}
	seen := map[string]bool{}
	for done := 0; done <= 30; done++ {
		for overdue := 0; overdue <= 30; overdue++ {
			for open := 0; open <= 30; open++ {
				seen[captionCategory(done, overdue, open)] = true
			}
		}
	}
	for _, c := range stored {
		if !seen[c] {
			t.Errorf("category %q has phrases in stat_captions but can never be selected", c)
		}
	}
	for c := range seen {
		known := false
		for _, s := range stored {
			if s == c {
				known = true
				break
			}
		}
		if !known {
			t.Errorf("category %q is returned but has no phrases in stat_captions", c)
		}
	}
}

// A caption must never contradict the numbers next to it: congratulating someone on a clean
// board while tasks are overdue is exactly the kind of thing users notice.
func TestCaptionCategoryNeverCongratulatesWithOverdue(t *testing.T) {
	congratulatory := map[string]bool{"perfect_day": true, "success": true, "project": true}
	for done := 0; done <= 30; done++ {
		for open := 0; open <= 30; open++ {
			for overdue := 1; overdue <= 30; overdue++ {
				got := captionCategory(done, overdue, open)
				if got == "perfect_day" {
					t.Fatalf("perfect_day with %d overdue (done=%d, open=%d)", overdue, done, open)
				}
				if congratulatory[got] && overdue > done {
					t.Fatalf("category %q with more overdue (%d) than done (%d)", got, overdue, done)
				}
			}
		}
	}
}

// inactive means "nothing happened", so it must never appear once work was completed.
func TestCaptionCategoryInactiveOnlyWhenNothingDone(t *testing.T) {
	for done := 1; done <= 30; done++ {
		for overdue := 0; overdue <= 5; overdue++ {
			for open := 0; open <= 30; open++ {
				if got := captionCategory(done, overdue, open); got == "inactive" {
					t.Fatalf("inactive with done=%d (overdue=%d, open=%d)", done, overdue, open)
				}
			}
		}
	}
}
