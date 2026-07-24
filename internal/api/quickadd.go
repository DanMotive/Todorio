package api

// Smart quick-add parsing (spec section 5: "быстрый ввод с распознаванием:
// `Купить домен #infra !high @Vlad tomorrow`").
//
// The parser is deliberately conservative: a token is only consumed when it resolves to
// something real. `@someone` who isn't an active member of the list's space stays in the title
// as literal text rather than silently vanishing, and `!urgent` is only stripped when it names
// an actual priority. Quietly eating part of what someone typed is worse than not parsing it —
// they'd never know the title they saved isn't the title they wrote.
//
// Recognised tokens, anywhere in the text:
//
//	#tag        -> appended to the "labels" multiselect custom field
//	!priority   -> low | normal | high | urgent (also localised aliases)
//	@username   -> assignee, if that user can actually see the list
//	a date word -> today | tomorrow | monday..sunday | dd.mm | dd.mm.yyyy | yyyy-mm-dd
//
// Everything not consumed stays in the title, with whitespace collapsed.

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	quickTagRe      = regexp.MustCompile(`(^|\s)#([\p{L}\p{N}_-]{1,32})`)
	quickPriorityRe = regexp.MustCompile(`(^|\s)!([\p{L}]{1,12})`)
	quickUserRe     = regexp.MustCompile(`(^|\s)@([a-zA-Z0-9_]{3,32})`)
	quickISODateRe  = regexp.MustCompile(`(^|\s)(\d{4}-\d{2}-\d{2})(\s|$)`)
	quickDotDateRe  = regexp.MustCompile(`(^|\s)(\d{1,2})\.(\d{1,2})(?:\.(\d{2,4}))?(\s|$)`)
)

// priorityAliases maps what a user might type to the four stored priorities. Russian and
// English are both accepted because the product ships both as first-class interface languages
// and people type quick-add in whatever language they're thinking in.
var priorityAliases = map[string]string{
	"low": "low", "normal": "normal", "high": "high", "urgent": "urgent",
	"med": "normal", "medium": "normal",
	"низкий": "low", "низкая": "low", "низ": "low",
	"обычный": "normal", "обычная": "normal", "норм": "normal", "средний": "normal",
	"высокий": "high", "высокая": "high", "выс": "high",
	"срочно": "urgent", "срочный": "urgent", "срочная": "urgent",
}

// weekdayAliases maps day names (English and Russian, full and short) to time.Weekday.
var weekdayAliases = map[string]time.Weekday{
	"monday": time.Monday, "mon": time.Monday, "понедельник": time.Monday, "пн": time.Monday,
	"tuesday": time.Tuesday, "tue": time.Tuesday, "вторник": time.Tuesday, "вт": time.Tuesday,
	"wednesday": time.Wednesday, "wed": time.Wednesday, "среда": time.Wednesday, "ср": time.Wednesday,
	"thursday": time.Thursday, "thu": time.Thursday, "четверг": time.Thursday, "чт": time.Thursday,
	"friday": time.Friday, "fri": time.Friday, "пятница": time.Friday, "пт": time.Friday,
	"saturday": time.Saturday, "sat": time.Saturday, "суббота": time.Saturday, "сб": time.Saturday,
	"sunday": time.Sunday, "sun": time.Sunday, "воскресенье": time.Sunday, "вс": time.Sunday,
}

var todayWords = map[string]int{
	"today": 0, "сегодня": 0,
	"tomorrow": 1, "завтра": 1,
	"overmorrow": 2, "послезавтра": 2,
}

// quickParsed is the outcome of parsing one quick-add line.
type quickParsed struct {
	Title      string     `json:"title"`
	Tags       []string   `json:"tags"`
	Priority   string     `json:"priority"`
	AssigneeID *int64     `json:"assignee_id"`
	Assignee   string     `json:"assignee"`
	DueAt      *time.Time `json:"due_at"`
	// Unresolved reports tokens that looked like a mention but matched no visible user, so the
	// UI can explain why they were left in the title instead of leaving the user guessing.
	Unresolved []string `json:"unresolved,omitempty"`
}

// parseQuickAdd extracts structured fields from a quick-add line.
//
// resolveUser looks a username up and reports whether it may be assigned here; it's injected
// so the pure parsing logic stays testable without a database.
func parseQuickAdd(text string, resolveUser func(string) (int64, bool)) quickParsed {
	out := quickParsed{Title: text}
	now := time.Now()

	// --- #tags ---
	out.Title = quickTagRe.ReplaceAllStringFunc(out.Title, func(m string) string {
		sub := quickTagRe.FindStringSubmatch(m)
		out.Tags = append(out.Tags, sub[2])
		return sub[1]
	})

	// --- !priority (only when the word is a real priority) ---
	out.Title = quickPriorityRe.ReplaceAllStringFunc(out.Title, func(m string) string {
		sub := quickPriorityRe.FindStringSubmatch(m)
		if p, ok := priorityAliases[strings.ToLower(sub[2])]; ok {
			out.Priority = p
			return sub[1]
		}
		return m // not a priority — leave the text exactly as typed
	})

	// --- @assignee (only when the user exists and can see the list) ---
	out.Title = quickUserRe.ReplaceAllStringFunc(out.Title, func(m string) string {
		sub := quickUserRe.FindStringSubmatch(m)
		if out.AssigneeID != nil {
			return m // first @mention wins; a second one is just text
		}
		if resolveUser != nil {
			if id, ok := resolveUser(sub[2]); ok {
				out.AssigneeID = &id
				out.Assignee = sub[2]
				return sub[1]
			}
		}
		out.Unresolved = append(out.Unresolved, sub[2])
		return m
	})

	// --- dates: explicit formats first, then words ---
	out.Title = quickISODateRe.ReplaceAllStringFunc(out.Title, func(m string) string {
		sub := quickISODateRe.FindStringSubmatch(m)
		if out.DueAt != nil {
			return m
		}
		if d, err := time.ParseInLocation("2006-01-02", sub[2], now.Location()); err == nil {
			d = endOfDay(d)
			out.DueAt = &d
			return sub[1] + sub[3]
		}
		return m
	})

	out.Title = quickDotDateRe.ReplaceAllStringFunc(out.Title, func(m string) string {
		sub := quickDotDateRe.FindStringSubmatch(m)
		if out.DueAt != nil {
			return m
		}
		day, _ := strconv.Atoi(sub[2])
		mon, _ := strconv.Atoi(sub[3])
		if day < 1 || day > 31 || mon < 1 || mon > 12 {
			return m
		}
		year := now.Year()
		if sub[4] != "" {
			y, err := strconv.Atoi(sub[4])
			if err != nil {
				return m
			}
			if y < 100 {
				y += 2000
			}
			year = y
		}
		d := time.Date(year, time.Month(mon), day, 0, 0, 0, 0, now.Location())
		// Reject a rolled-over date (31.02 becomes 03.03) rather than silently accepting it.
		if d.Day() != day || d.Month() != time.Month(mon) {
			return m
		}
		// A bare dd.mm already past this year means next year — "01.02" typed in December is
		// almost certainly the coming February, not one ten months gone.
		if sub[4] == "" && d.Before(startOfToday(now)) {
			d = d.AddDate(1, 0, 0)
		}
		d = endOfDay(d)
		out.DueAt = &d
		return sub[1] + sub[5]
	})

	// Word dates are matched per whitespace-separated token so "today" inside a longer word
	// (e.g. "todays-plan") is never consumed.
	if out.DueAt == nil {
		fields := strings.Fields(out.Title)
		for i, f := range fields {
			key := strings.ToLower(strings.Trim(f, ".,;:!?()[]«»\"'"))
			if off, ok := todayWords[key]; ok {
				d := endOfDay(startOfToday(now).AddDate(0, 0, off))
				out.DueAt = &d
				fields = append(fields[:i], fields[i+1:]...)
				out.Title = strings.Join(fields, " ")
				break
			}
			if wd, ok := weekdayAliases[key]; ok {
				d := endOfDay(nextWeekday(startOfToday(now), wd))
				out.DueAt = &d
				fields = append(fields[:i], fields[i+1:]...)
				out.Title = strings.Join(fields, " ")
				break
			}
		}
	}

	out.Title = strings.Join(strings.Fields(out.Title), " ")
	return out
}

func startOfToday(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// endOfDay puts a due date at 23:59 local time. A deadline of "tomorrow" means the end of
// tomorrow, not midnight at its start — otherwise a task would read as overdue all day.
func endOfDay(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 0, 0, d.Location())
}

// nextWeekday returns the next occurrence of wd strictly after `from` (naming a weekday means
// the upcoming one; "monday" typed on a Monday means the next Monday, not today).
func nextWeekday(from time.Time, wd time.Weekday) time.Time {
	delta := (int(wd) - int(from.Weekday()) + 7) % 7
	if delta == 0 {
		delta = 7
	}
	return from.AddDate(0, 0, delta)
}

// resolveUserForList returns a lookup that only accepts users who can actually see the list —
// an assignee who can't open the task would be a confusing dead end.
func (a *API) resolveUserForList(ctx context.Context, listID int64) func(string) (int64, bool) {
	return func(username string) (int64, bool) {
		var id int64
		err := a.DB.Pool.QueryRow(ctx, `
			SELECT u.id FROM users u
			WHERE u.username = $1 AND u.status = 'active' AND u.archived_at IS NULL
			  AND (
			    u.role IN ('root','admin')
			    OR EXISTS (SELECT 1 FROM list_members lm WHERE lm.list_id = $2 AND lm.user_id = u.id)
			    OR EXISTS (
			      SELECT 1 FROM lists l
			      JOIN space_members sm ON sm.space_id = l.space_id AND sm.user_id = u.id
			      WHERE l.id = $2 AND l.is_private = false
			    )
			  )`, username, listID).Scan(&id)
		return id, err == nil
	}
}
