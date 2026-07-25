package telegram

// Covers the DB-free parts: parsing, and the HTTP client against a fake Telegram server. The
// polling loop itself (pollOnce/Run) also touches Postgres and is exercised instead by
// `todorio testsql` (the project's convention — see internal/ops/testsql.go — is that `go test`
// never requires a live database; DB-touching SQL is verified there, not here) plus a manual
// end-to-end run against the embedded instance during the verification pass before delivery.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseStartCode(t *testing.T) {
	cases := []struct {
		text     string
		wantCode string
		wantOK   bool
	}{
		{"/start abc123", "abc123", true},
		{"  /start   abc123  ", "abc123", true}, // stray whitespace, e.g. a mobile client's autocorrect
		{"/start", "", false},                   // bot opened with no deep-link payload
		{"/start ", "", false},
		{"hello", "", false},
		{"", "", false},
		{"/startsomething", "something", true}, // Telegram sends "/start CODE", never "/startCODE" in
		// practice, but the prefix-trim is deliberately permissive rather than requiring a space —
		// harmless either way since a real deep-link code never collides with plain text.
	}
	for _, c := range cases {
		code, ok := parseStartCode(c.text)
		if ok != c.wantOK || code != c.wantCode {
			t.Errorf("parseStartCode(%q) = (%q, %v), want (%q, %v)", c.text, code, ok, c.wantCode, c.wantOK)
		}
	}
}

// fakeTelegram builds a test server that speaks just enough of the Bot API shape for these
// tests, and points the package's apiBase at it for the duration of the test.
func fakeTelegram(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	orig := apiBase
	apiBase = srv.URL + "/bot"
	t.Cleanup(func() { apiBase = orig })
}

func TestGetMeSuccess(t *testing.T) {
	fakeTelegram(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/getMe") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("getMe should be GET, got %s", r.Method)
		}
		w.Write([]byte(`{"ok":true,"result":{"username":"my_todorio_bot"}}`))
	})
	username, err := GetMe(context.Background(), "123:FAKE")
	if err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if username != "my_todorio_bot" {
		t.Errorf("username = %q, want my_todorio_bot", username)
	}
}

func TestGetMeRejectsInvalidToken(t *testing.T) {
	fakeTelegram(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"ok":false,"description":"Unauthorized"}`))
	})
	_, err := GetMe(context.Background(), "bad-token")
	if err == nil {
		t.Fatal("expected an error for a token Telegram rejects, got nil")
	}
	if !strings.Contains(err.Error(), "Unauthorized") {
		t.Errorf("error should surface Telegram's own description, got: %v", err)
	}
}

func TestSendMessageUsesPostFormNotQueryString(t *testing.T) {
	// A long announcement body must not silently get truncated or rejected by a proxy's URL
	// length limit — this is exactly why SendMessage uses postCall, not getCall.
	longText := "New comment · «Task» · by @someone\n" + strings.Repeat("This is a long announcement body. ", 200)

	var gotMethod, gotContentType, gotChatID, gotText string
	fakeTelegram(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		gotChatID = r.FormValue("chat_id")
		gotText = r.FormValue("text")
		// The whole point: none of the payload should be riding on the URL.
		if r.URL.RawQuery != "" {
			t.Errorf("sendMessage put data on the query string (%q); want a form body", r.URL.RawQuery)
		}
		w.Write([]byte(`{"ok":true,"result":{}}`))
	})

	if err := SendMessage(context.Background(), "123:FAKE", 555, longText); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Errorf("content-type = %s", gotContentType)
	}
	if gotChatID != "555" {
		t.Errorf("chat_id = %s, want 555", gotChatID)
	}
	if gotText != longText {
		t.Errorf("text was mangled in transit (got %d chars, want %d)", len(gotText), len(longText))
	}
}

func TestSendMessageSurfacesAPIError(t *testing.T) {
	fakeTelegram(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":false,"description":"Forbidden: bot was blocked by the user"}`))
	})
	err := SendMessage(context.Background(), "123:FAKE", 1, "hi")
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Errorf("expected the block reason to surface, got: %v", err)
	}
}

func TestNewLinkCodeIsURLSafeAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		code, err := NewLinkCode()
		if err != nil {
			t.Fatalf("NewLinkCode: %v", err)
		}
		if len(code) != 24 {
			t.Errorf("code length = %d, want 24 (12 random bytes, hex-encoded)", len(code))
		}
		for _, r := range code {
			if !strings.ContainsRune("0123456789abcdef", r) {
				t.Errorf("code %q contains a character outside Telegram's start-param charset", code)
				break
			}
		}
		if seen[code] {
			t.Errorf("NewLinkCode produced a duplicate: %q", code)
		}
		seen[code] = true
	}
}
