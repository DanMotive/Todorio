package auth

import (
	"strings"
	"testing"
	"time"
)

func newSecret(t *testing.T) string {
	t.Helper()
	s, err := NewTOTPSecret()
	if err != nil {
		t.Fatalf("NewTOTPSecret: %v", err)
	}
	return s
}

func codeFor(t *testing.T, secret string, counter int64) string {
	t.Helper()
	c, err := totpCode(secret, uint64(counter))
	if err != nil {
		t.Fatalf("totpCode(%d): %v", counter, err)
	}
	return c
}

// awaitStableStep waits until the current 30-second step has enough time left to run a test
// against its neighbouring windows. Without this, a test that starts at second 29 computes a code
// for "the previous window" and then verifies it a moment later, when the clock has moved on and
// that window has become two steps old — correctly rejected, and a flake rather than a bug.
func awaitStableStep(t *testing.T) {
	t.Helper()
	if secs := time.Now().Unix() % totpStep; secs > totpStep-5 {
		time.Sleep(time.Duration(totpStep-secs+1) * time.Second)
	}
}

func TestVerifyTOTPAtAcceptsCurrentCode(t *testing.T) {
	secret := newSecret(t)
	counter := time.Now().Unix() / totpStep
	got, ok := VerifyTOTPAt(secret, codeFor(t, secret, counter), 0)
	if !ok {
		t.Fatal("the code for the current window was rejected")
	}
	if got != counter {
		t.Errorf("accepted counter = %d, want %d", got, counter)
	}
}

// The reason VerifyTOTPAt exists: a code stays valid for up to 90 seconds, which is long enough
// for someone who reads it over a shoulder, out of a screen share, or off a phishing page to type
// it in themselves. Recording the counter that was accepted and refusing anything at or below it
// makes each code single-use.
func TestVerifyTOTPAtRejectsReplay(t *testing.T) {
	secret := newSecret(t)
	code := codeFor(t, secret, time.Now().Unix()/totpStep)

	accepted, ok := VerifyTOTPAt(secret, code, 0)
	if !ok {
		t.Fatal("first use of the code was rejected")
	}
	if _, ok := VerifyTOTPAt(secret, code, accepted); ok {
		t.Error("the same code was accepted twice")
	}
}

func TestVerifyTOTPAtRejectsEarlierWindowThanAccepted(t *testing.T) {
	awaitStableStep(t)
	secret := newSecret(t)
	now := time.Now().Unix() / totpStep

	// A code from the previous window is normally valid, but not once a later one has been used:
	// otherwise an attacker holding an older code could still spend it after the victim logged in.
	if _, ok := VerifyTOTPAt(secret, codeFor(t, secret, now-1), now); ok {
		t.Error("a code older than the last accepted counter was allowed")
	}
}

func TestVerifyTOTPAtWindow(t *testing.T) {
	awaitStableStep(t)
	secret := newSecret(t)
	now := time.Now().Unix() / totpStep

	// One step either side is accepted, to tolerate clock drift between the server and the phone.
	for _, offset := range []int64{-1, 0, 1} {
		if _, ok := VerifyTOTPAt(secret, codeFor(t, secret, now+offset), 0); !ok {
			t.Errorf("code for window %+d was rejected", offset)
		}
	}
	// Two steps out is not.
	for _, offset := range []int64{-2, 2} {
		if _, ok := VerifyTOTPAt(secret, codeFor(t, secret, now+offset), 0); ok {
			t.Errorf("code for window %+d was accepted", offset)
		}
	}
}

func TestVerifyTOTPAtRejectsMalformedInput(t *testing.T) {
	secret := newSecret(t)
	valid := codeFor(t, secret, time.Now().Unix()/totpStep)

	cases := []struct {
		name   string
		secret string
		code   string
	}{
		{"empty code", secret, ""},
		{"too short", secret, valid[:5]},
		{"too long", secret, valid + "0"},
		{"empty secret", "", valid},
		{"not base32", "not a secret!", valid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := VerifyTOTPAt(tc.secret, tc.code, 0); ok {
				t.Error("accepted")
			}
		})
	}
}

func TestVerifyTOTPAtTrimsWhitespace(t *testing.T) {
	// Authenticator apps display codes as "123 456" and users paste them with the space or with a
	// trailing newline. Leading and trailing space is tolerated; the digits themselves are not
	// reformatted.
	secret := newSecret(t)
	code := codeFor(t, secret, time.Now().Unix()/totpStep)
	if _, ok := VerifyTOTPAt(secret, "  "+code+"\n", 0); !ok {
		t.Error("a padded code was rejected")
	}
}

func TestVerifyTOTPRejectsCodeFromAnotherSecret(t *testing.T) {
	counter := time.Now().Unix() / totpStep
	mine, theirs := newSecret(t), newSecret(t)
	if VerifyTOTP(mine, codeFor(t, theirs, counter)) {
		t.Error("a code generated from a different secret was accepted")
	}
}

func TestTOTPURL(t *testing.T) {
	uri := TOTPURL("JBSWY3DPEHPK3PXP", "vlad@example.com", "Todorio")
	for _, want := range []string{
		"otpauth://totp/Todorio:vlad@example.com",
		"secret=JBSWY3DPEHPK3PXP",
		"issuer=Todorio",
		"digits=6",
		"period=30",
	} {
		if !strings.Contains(uri, want) {
			t.Errorf("otpauth URL is missing %q:\n%s", want, uri)
		}
	}
}
