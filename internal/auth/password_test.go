package auth

import (
	"strings"
	"testing"
	"time"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	const pw = "correct horse battery staple"
	encoded, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$") {
		t.Errorf("unexpected encoding prefix: %q", encoded)
	}
	if strings.Contains(encoded, pw) {
		t.Fatal("the encoded hash contains the password itself")
	}
	if !VerifyPassword(pw, encoded) {
		t.Error("the password did not verify against its own hash")
	}
	if VerifyPassword(pw+" ", encoded) {
		t.Error("a password with a trailing space verified")
	}
	if VerifyPassword("Correct horse battery staple", encoded) {
		t.Error("verification is not case sensitive")
	}
	if VerifyPassword("", encoded) {
		t.Error("an empty password verified")
	}
}

func TestHashPasswordIsSalted(t *testing.T) {
	// Equal passwords must not produce equal hashes, otherwise a glance at the users table shows
	// which accounts share a password.
	a, err := HashPassword("same password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	b, err := HashPassword("same password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if a == b {
		t.Error("two hashes of the same password are identical")
	}
	if !VerifyPassword("same password", a) || !VerifyPassword("same password", b) {
		t.Error("a salted hash failed to verify")
	}
}

func TestHashPasswordAcceptsUnicodeAndLongInput(t *testing.T) {
	for _, pw := range []string{
		"пароль с пробелами",
		"🔐🔐🔐",
		strings.Repeat("x", 4096),
	} {
		encoded, err := HashPassword(pw)
		if err != nil {
			t.Fatalf("HashPassword(%.16q): %v", pw, err)
		}
		if !VerifyPassword(pw, encoded) {
			t.Errorf("round trip failed for %.16q", pw)
		}
	}
}

// A malformed hash must be a plain "no", never a panic. These strings stand in for rows written
// by an older build, truncated by a botched restore, or edited by hand in psql.
func TestVerifyPasswordRejectsMalformedEncodings(t *testing.T) {
	cases := map[string]string{
		"empty":              "",
		"not a hash":         "hunter2",
		"wrong algorithm":    "$argon2i$v=19$m=65536,t=3,p=2$c2FsdHNhbHQ$aGFzaGhhc2g",
		"bcrypt":             "$2y$10$abcdefghijklmnopqrstuv",
		"too few fields":     "$argon2id$v=19$m=65536,t=3,p=2",
		"bad parameters":     "$argon2id$v=19$m=abc,t=xyz,p=?$c2FsdHNhbHQ$aGFzaGhhc2g",
		"bad base64 salt":    "$argon2id$v=19$m=65536,t=3,p=2$!!!!$aGFzaGhhc2g",
		"bad base64 hash":    "$argon2id$v=19$m=65536,t=3,p=2$c2FsdHNhbHQ$!!!!",
		"truncated mid-hash": "$argon2id$v=19$m=65536,t=3,p=2$c2FsdHNhbHQ$",
		"only separators":    "$$$$$",
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			if VerifyPassword("anything", encoded) {
				t.Error("a malformed hash verified")
			}
		})
	}
}

func TestBurnPasswordTimeDoesNotPanic(t *testing.T) {
	// Called on the login path for usernames that do not exist, so that a missing account and a
	// wrong password take about the same time to answer.
	BurnPasswordTime("")
	BurnPasswordTime("whatever the attacker typed")
}

// argon2id is configured for 64 MB per derivation. Without a cap on how many run at once, a few
// dozen simultaneous logins would ask the machine for several gigabytes and the kernel would
// start killing things. acquireHash is what enforces that cap.
func TestAcquireHashLimitsConcurrency(t *testing.T) {
	releases := make([]func(), 0, maxConcurrentHashes)
	for i := 0; i < maxConcurrentHashes; i++ {
		releases = append(releases, acquireHash())
	}

	gotSlot := make(chan struct{})
	go func() {
		release := acquireHash()
		close(gotSlot)
		release()
	}()

	select {
	case <-gotSlot:
		t.Fatalf("acquireHash handed out more than %d slots at once", maxConcurrentHashes)
	case <-time.After(100 * time.Millisecond):
	}

	releases[0]() // free one slot; the waiting goroutine should now get through
	select {
	case <-gotSlot:
	case <-time.After(5 * time.Second):
		t.Fatal("a released slot was never handed to the waiting caller")
	}

	for _, release := range releases[1:] {
		release()
	}
}
