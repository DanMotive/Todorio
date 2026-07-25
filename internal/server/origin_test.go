package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHostMatches(t *testing.T) {
	cases := []struct {
		rawURL string
		host   string
		want   bool
	}{
		{"https://todorio.example.com", "todorio.example.com", true},
		{"https://todorio.example.com/some/path", "todorio.example.com", true},
		{"http://todorio.example.com:8080", "todorio.example.com:8080", true},
		{"https://TODORIO.example.com", "todorio.example.com", true}, // host names are case insensitive
		{"https://evil.example.com", "todorio.example.com", false},
		// A different port is a different origin, and so is a look-alike prefix or suffix.
		{"http://todorio.example.com:8080", "todorio.example.com", false},
		{"https://todorio.example.com.evil.net", "todorio.example.com", false},
		{"https://nottodorio.example.com", "todorio.example.com", false},
		{"", "todorio.example.com", false},
		{"not a url", "todorio.example.com", false},
		{"/relative/path", "todorio.example.com", false}, // no host to compare
		{"null", "todorio.example.com", false},           // sandboxed iframes send Origin: null
	}
	for _, tc := range cases {
		if got := hostMatches(tc.rawURL, tc.host); got != tc.want {
			t.Errorf("hostMatches(%q, %q) = %v, want %v", tc.rawURL, tc.host, got, tc.want)
		}
	}
}

func TestSameOrigin(t *testing.T) {
	const host = "todorio.example.com"
	cases := []struct {
		name    string
		headers map[string]string
		want    bool
	}{
		// Sec-Fetch-Site is set by the browser and cannot be forged by a page, so it wins where
		// it is present.
		{"fetch metadata, same origin", map[string]string{"Sec-Fetch-Site": "same-origin"}, true},
		{"fetch metadata, typed or bookmarked", map[string]string{"Sec-Fetch-Site": "none"}, true},
		{"fetch metadata, cross site", map[string]string{"Sec-Fetch-Site": "cross-site"}, false},
		{"fetch metadata, same site subdomain", map[string]string{"Sec-Fetch-Site": "same-site"}, false},
		{
			// A cross-site request that also carries a matching Origin should still be refused:
			// the trustworthy header is the one the browser controls.
			name: "fetch metadata overrides a matching origin",
			headers: map[string]string{
				"Sec-Fetch-Site": "cross-site",
				"Origin":         "https://" + host,
			},
			want: false,
		},
		{"origin matches", map[string]string{"Origin": "https://" + host}, true},
		{"origin from another site", map[string]string{"Origin": "https://evil.example.com"}, false},
		{"opaque origin", map[string]string{"Origin": "null"}, false},
		{"referer matches", map[string]string{"Referer": "https://" + host + "/app/inbox"}, true},
		{"referer from another site", map[string]string{"Referer": "https://evil.example.com/"}, false},
		{
			name: "origin is preferred over referer",
			headers: map[string]string{
				"Origin":  "https://evil.example.com",
				"Referer": "https://" + host + "/app",
			},
			want: false,
		},
		// An old browser or a curl script sends nothing. Refusing here is what makes the check
		// worth having, since a forged cross-site form post looks exactly like this.
		{"no headers at all", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "https://"+host+"/api/tasks", nil)
			r.Host = host
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			if got := sameOrigin(r); got != tc.want {
				t.Errorf("sameOrigin = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRequireSameOriginAllowsReads(t *testing.T) {
	// Session cookies are SameSite=Lax, so a cross-site GET arrives without credentials anyway.
	// Blocking reads here would break following a shared link from Telegram or an email.
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			rec := serveThroughOriginCheck(t, method, nil)
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
		})
	}
}

func TestRequireSameOriginBlocksCrossSiteWrites(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			rec := serveThroughOriginCheck(t, method, map[string]string{"Origin": "https://evil.example.com"})
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				// The frontend parses every error body as JSON; an HTML page here surfaces as an
				// unhelpful parse error instead of the actual reason.
				t.Errorf("Content-Type = %q, want JSON", ct)
			}
			if !strings.Contains(rec.Body.String(), "cross-site request blocked") {
				t.Errorf("unexpected body: %s", rec.Body.String())
			}
		})
	}
}

func TestRequireSameOriginAllowsOwnFrontend(t *testing.T) {
	rec := serveThroughOriginCheck(t, http.MethodPost, map[string]string{"Sec-Fetch-Site": "same-origin"})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 — the app's own requests must get through", rec.Code)
	}
}

func TestRequireSameOriginBlocksWritesWithNoOriginHeaders(t *testing.T) {
	// This is the shape of a classic forged form post from another page.
	if rec := serveThroughOriginCheck(t, http.MethodPost, nil); rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func serveThroughOriginCheck(t *testing.T, method string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	const host = "todorio.example.com"

	reached := false
	handler := requireSameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(method, "https://"+host+"/api/tasks", nil)
	r.Host = host
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	if rec.Code == http.StatusForbidden && reached {
		t.Error("the request was rejected but the handler behind the check still ran")
	}
	return rec
}
