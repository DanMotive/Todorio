package api

import (
	"reflect"
	"regexp"
	"testing"
)

func TestParseMentions(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"plain", "@anna take a look", []string{"anna"}},
		{"mid sentence", "cc @anna and @bob please", []string{"anna", "bob"}},
		{"repeated once", "@anna @anna @anna", []string{"anna"}},
		{"punctuation around", "(@anna), «@bob»!", []string{"anna", "bob"}},
		{"newline", "first line\n@anna", []string{"anna"}},
		{"underscore is part of the name", "ping @an_na", []string{"an_na"}},

		// The whole reason this file exists.
		{"email address", "write to john@example.com", nil},
		{"email address, cyrillic local part", "почта@example", nil},
		{"inside a word", "foo@bar", nil},
		{"double at", "@@anna", nil},
		{"too short", "@ab", nil},
		{"nothing at all", "no mentions here", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseMentions(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("parseMentions(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// The inbox asks "does this comment mention me?" in SQL, so the same rule has to survive as a
// pattern string. Go's regexp and Postgres's `~` agree on everything used here (POSIX classes,
// alternation, anchors), so compiling it locally is a fair test of what the database will do.
//
// Each case below is something the previous `body ILIKE '%@' || username || '%'` got wrong.
func TestMentionSQLPattern(t *testing.T) {
	cases := []struct {
		username string
		body     string
		want     bool
	}{
		{"bob", "@bob please look", true},
		{"bob", "ping @bob", true},
		{"bob", "(@bob)", true},
		{"bob", "@bobby is someone else", false},
		{"bob", "mail@bob.example", false},

		// `_` is a single-character LIKE wildcard, and usernames may contain it.
		{"an_na", "@an_na hello", true},
		{"an_na", "@anXna hello", false},

		// A dot is a regexp wildcard; QuoteMeta has to neutralise it.
		{"a.bc", "@a.bc hello", true},
		{"a.bc", "@axbc hello", false},
	}
	for _, c := range cases {
		re, err := regexp.Compile(mentionSQLPattern(c.username))
		if err != nil {
			t.Fatalf("pattern for %q does not compile: %v", c.username, err)
		}
		if got := re.MatchString(c.body); got != c.want {
			t.Errorf("username %q vs body %q: matched=%v, want %v", c.username, c.body, got, c.want)
		}
	}
}
