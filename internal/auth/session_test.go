package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionCookieAttributes(t *testing.T) {
	c := sessionCookie("session-value", 3600, true)

	if c.Name != CookieName {
		t.Errorf("cookie name = %q, want %q", c.Name, CookieName)
	}
	if c.Value != "session-value" {
		t.Errorf("cookie value = %q", c.Value)
	}
	if c.Path != "/" {
		t.Errorf("cookie path = %q, want /", c.Path)
	}
	// Without HttpOnly, any script that manages to run on the page can read the session id and
	// send it elsewhere; the CSP is a second line of defence, not a substitute for this one.
	if !c.HttpOnly {
		t.Error("the session cookie is readable from JavaScript")
	}
	if !c.Secure {
		t.Error("Secure was not set for an HTTPS request")
	}
	// Lax rather than Strict: Strict would drop the cookie when a user follows a link into
	// Todorio from Telegram or an email, landing them on a login screen while already signed in.
	// Cross-origin writes are stopped by requireSameOrigin instead.
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", c.SameSite)
	}
	if c.MaxAge != 3600 {
		t.Errorf("MaxAge = %d, want 3600", c.MaxAge)
	}
}

func TestSessionCookieSecureFollowsTheRequest(t *testing.T) {
	// Setting Secure on a plain-HTTP deployment makes the browser drop the cookie outright, so it
	// tracks the actual scheme rather than being hard-coded.
	if sessionCookie("v", 3600, false).Secure {
		t.Error("Secure was set for a plain HTTP request")
	}
}

func TestSessionCookieDeletion(t *testing.T) {
	c := sessionCookie("", -1, true)
	if c.MaxAge >= 0 {
		t.Errorf("MaxAge = %d, want a negative value to expire the cookie", c.MaxAge)
	}
	if c.Value != "" {
		t.Errorf("the deletion cookie still carries a value: %q", c.Value)
	}
	// The attributes have to match the cookie being replaced, or the browser keeps the old one
	// alongside the empty one and the user stays logged in after pressing “log out”.
	if !c.HttpOnly || c.Path != "/" {
		t.Error("the deletion cookie does not match the attributes of the cookie it replaces")
	}
}

func TestSecureRequest(t *testing.T) {
	cases := []struct {
		name      string
		cfgHTTPS  bool
		forwarded string
		want      bool
	}{
		{name: "plain http", want: false},
		{name: "https configured", cfgHTTPS: true, want: true},
		{name: "terminated by a proxy", forwarded: "https", want: true},
		{name: "proxy header in mixed case", forwarded: "HTTPS", want: true},
		{name: "padded proxy header", forwarded: "  https ", want: true},
		// A chain lists the client-facing hop first: "https, http" is a browser that reached the
		// edge over TLS even though the last internal hop was plaintext.
		{name: "chain, tls at the edge", forwarded: "https, http", want: true},
		{name: "chain, plaintext at the edge", forwarded: "http, https", want: false},
		{name: "unrelated value", forwarded: "ftp", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://todorio.local/api/me", nil)
			if tc.forwarded != "" {
				r.Header.Set("X-Forwarded-Proto", tc.forwarded)
			}
			if got := SecureRequest(r, tc.cfgHTTPS); got != tc.want {
				t.Errorf("SecureRequest = %v, want %v", got, tc.want)
			}
		})
	}
}
