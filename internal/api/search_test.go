package api

import (
	"strings"
	"testing"
)

// searchTSQuery builds a tsquery out of untrusted input. Two things must hold: the result is
// always safe to hand to to_tsquery (a malformed expression makes Postgres raise an error, so a
// leaked operator is a broken endpoint, not just a wrong result), and ordinary phrases keep
// finding what they used to before the ILIKE scan was replaced.

func TestSearchTSQueryBuildsPrefixTerms(t *testing.T) {
	got := searchTSQuery("deploy script")
	want := "deploy:* & script:*"
	if got != want {
		t.Fatalf("searchTSQuery(%q) = %q, want %q", "deploy script", got, want)
	}
}

// The prefix match is what stands in for stemming: the index is built with the 'simple'
// configuration, which does not reduce words to a stem, so "задач" has to match "задачи"
// through the trailing :* instead.
func TestSearchTSQueryLowercasesUnicode(t *testing.T) {
	got := searchTSQuery("Задача Отчёт")
	want := "задача:* & отчёт:*"
	if got != want {
		t.Fatalf("searchTSQuery = %q, want %q", got, want)
	}
}

// Every tsquery operator has to come out as a separator, never as syntax. If any of these
// leaked through, to_tsquery would either error or evaluate an expression the user never wrote.
func TestSearchTSQueryStripsOperators(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"and", "a & b", "a:* & b:*"},
		{"or", "a | b", "a:* & b:*"},
		{"not", "!secret", "secret:*"},
		{"followed by", "a <-> b", "a:* & b:*"},
		{"parens", "(a b)", "a:* & b:*"},
		{"quotes", `"a b"`, "a:* & b:*"},
		{"colon star in input", "a:* b", "a:* & b:*"},
		{"backslash", `a\ b`, "a:* & b:*"},
		{"mixed punctuation", "user@example.com", "user:* & example:* & com:*"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := searchTSQuery(tc.in)
			if got != tc.want {
				t.Fatalf("searchTSQuery(%q) = %q, want %q", tc.in, got, tc.want)
			}
			for _, op := range []string{"|", "!", "<", ">", "(", ")", "'", `"`, `\`} {
				if strings.Contains(got, op) {
					t.Fatalf("operator %q survived sanitisation in %q", op, got)
				}
			}
		})
	}
}

// An input with nothing indexable must return "", which is the caller's signal to skip the
// full-text pass entirely and go straight to the ILIKE fallback. Returning a malformed query
// here would turn a search for "???" into a 500.
func TestSearchTSQueryEmptyWhenNoTerms(t *testing.T) {
	for _, in := range []string{"", "   ", "!!!", "&|<>", "\t\n", "---"} {
		if got := searchTSQuery(in); got != "" {
			t.Fatalf("searchTSQuery(%q) = %q, want empty", in, got)
		}
	}
}

func TestSearchTSQueryKeepsDigits(t *testing.T) {
	if got := searchTSQuery("release 2026"); got != "release:* & 2026:*" {
		t.Fatalf("searchTSQuery = %q", got)
	}
}

// A pathological query must not turn into a pathological tsquery.
func TestSearchTSQueryCapsTermCount(t *testing.T) {
	in := strings.Repeat("word ", 50)
	got := searchTSQuery(in)
	if n := strings.Count(got, ":*"); n != searchMaxTerms {
		t.Fatalf("got %d terms, want the cap of %d", n, searchMaxTerms)
	}
	if strings.HasSuffix(got, "&") || strings.HasPrefix(got, "&") {
		t.Fatalf("query has a dangling operator: %q", got)
	}
}

// Truncation must not leave a trailing "&" behind, which would be a syntax error rather than a
// shorter search — worth its own check because the cap is applied while terms are collected.
func TestSearchTSQueryNeverDanglingOperator(t *testing.T) {
	for _, in := range []string{"a", "a b", "a b c d e f g h i j k l", "a!", "!a", "a & "} {
		got := searchTSQuery(in)
		if got == "" {
			continue
		}
		if strings.HasPrefix(got, " ") || strings.HasSuffix(got, " ") ||
			strings.HasSuffix(got, "&") || strings.Contains(got, "& &") {
			t.Fatalf("malformed tsquery %q from input %q", got, in)
		}
	}
}
