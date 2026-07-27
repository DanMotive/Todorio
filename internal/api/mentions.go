package api

// One definition of what "@someone" means in free-form text.
//
// There were three, and they disagreed with each other. handleCreateComment matched
// `@([a-zA-Z0-9_]{3,32})` anywhere in the body, so the address in "ask john@example.com"
// notified a user called `example`, and it resolved the name against the whole users table —
// mentioning someone who cannot open the list still sent them a notification carrying the
// task's title. Quick-add (quickadd.go) already did it properly: a boundary before the `@`, and
// a lookup restricted to people who can actually see the list. The inbox did it a third way, in
// SQL, with `body ILIKE '%@' || username || '%'` — no boundary at either end, so `bob` collected
// every task where `@bobby` was mentioned, and an underscore in a username (the mention charset
// allows them) is a LIKE wildcard, so `an_na` matched `@anXna`.
//
// The rule, in one place, is: an `@` that does not continue a word, followed by 3-32 characters
// of [a-zA-Z0-9_], naming a user who can see the list in question.

import (
	"context"
	"regexp"
)

// mentionRe matches a mention and captures the username in group 2.
//
// The leading group is what keeps email addresses and paths out: the `@` must be at the start
// of the text or follow something that is not a letter, digit, underscore or another `@`.
// Letters and digits are matched by Unicode class, not [a-zA-Z0-9], so that "почта@ann" is as
// much "not a mention" as "mail@ann" is. The username itself stays ASCII — that is the charset
// quick-add has always used, and the one usernames are issued in.
var mentionRe = regexp.MustCompile(`(^|[^\p{L}\p{N}_@])@([a-zA-Z0-9_]{3,32})`)

// parseMentions returns the usernames mentioned in text, in order of first appearance and
// without duplicates — mentioning someone three times in one comment is one notification.
func parseMentions(text string) []string {
	matches := mentionRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		name := m[2]
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

// mentionedUserIDs resolves the mentions in text to user ids, keeping only people who can see
// listID. It reuses resolveUserForList, the same lookup quick-add assigns through, so "who may
// be mentioned here" and "who may be assigned here" cannot drift apart.
//
// A name that resolves to nobody visible is simply not in the result: the mention stays in the
// text the author wrote, and no one outside the list learns that the task exists.
func (a *API) mentionedUserIDs(ctx context.Context, listID int64, text string) []int64 {
	names := parseMentions(text)
	if len(names) == 0 {
		return nil
	}
	resolve := a.resolveUserForList(ctx, listID)
	ids := make([]int64, 0, len(names))
	for _, name := range names {
		if id, ok := resolve(name); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// mentionSQLPattern builds a POSIX pattern for Postgres's `~` operator that matches the same
// thing mentionRe does, for the one query that has to ask "does this row mention me?" in SQL
// rather than in Go (the inbox, which cannot pull every comment body into the process).
//
// The username goes through QuoteMeta, so nothing inside it — a wildcard, a bracket, a
// backslash — is read as pattern syntax. Matching is case-sensitive on purpose: notifications
// resolve names with `username = $1`, and a case-insensitive inbox would show mentions that
// never produced a notification.
func mentionSQLPattern(username string) string {
	return `(^|[^[:alnum:]_@])@` + regexp.QuoteMeta(username) + `([^[:alnum:]_]|$)`
}
