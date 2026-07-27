package api

import (
	"reflect"
	"strings"
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

// A username is user-controlled text that ends up inside a pattern, which is exactly how the
// old ILIKE version went wrong. Nothing in it may survive as syntax.
func TestMentionSQLPatternQuotesTheUsername(t *testing.T) {
	for _, username := range []string{"an_na", "100%", "a.b", `back\slash`, "br[ackets]"} {
		pat := mentionSQLPattern(username)
		if !strings.Contains(pat, "@") {
			t.Fatalf("pattern for %q lost the @: %q", username, pat)
		}
		if strings.Contains(pat, "@"+username) && username != "an_na" {
			t.Fatalf("pattern for %q embedded the username unescaped: %q", username, pat)
		}
	}
}
