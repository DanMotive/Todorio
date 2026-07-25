package auth

import (
	"regexp"
	"strings"
	"testing"
)

var recoveryDisplayForm = regexp.MustCompile(`^[abcdefghjkmnpqrstuvwxyz23456789]{5}-[abcdefghjkmnpqrstuvwxyz23456789]{5}$`)

func TestNewRecoveryCodesShape(t *testing.T) {
	codes, err := NewRecoveryCodes(RecoveryCodeCount)
	if err != nil {
		t.Fatalf("NewRecoveryCodes: %v", err)
	}
	if len(codes) != RecoveryCodeCount {
		t.Fatalf("got %d codes, want %d", len(codes), RecoveryCodeCount)
	}
	for _, c := range codes {
		if !recoveryDisplayForm.MatchString(c) {
			t.Errorf("code %q is not in the xxxxx-xxxxx display form", c)
		}
		// The alphabet deliberately omits i, l, o, 0 and 1. Someone reading a code off a printout
		// or a screenshot at the exact moment they have lost their phone should not have to guess
		// whether a character is a one or an ell.
		if strings.ContainsAny(c, "ilo01") {
			t.Errorf("code %q contains an ambiguous character", c)
		}
	}
}

func TestNewRecoveryCodesAreDistinct(t *testing.T) {
	// Two identical codes in one batch would silently cost the user a spare, and a generator
	// repeating itself is the first symptom of a broken entropy source.
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		codes, err := NewRecoveryCodes(RecoveryCodeCount)
		if err != nil {
			t.Fatalf("NewRecoveryCodes: %v", err)
		}
		for _, c := range codes {
			if seen[c] {
				t.Fatalf("duplicate code generated: %q", c)
			}
			seen[c] = true
		}
	}
}

func TestNormalizeRecoveryCode(t *testing.T) {
	cases := map[string]string{
		"abcde-fghij":   "abcdefghij",
		"ABCDE-FGHIJ":   "abcdefghij",
		"abcde fghij":   "abcdefghij",
		"  abcdefghij ": "abcdefghij",
		"a-b c.d\te":    "abcde",
		"23456-78923":   "2345678923",
		"":              "",
	}
	for in, want := range cases {
		if got := NormalizeRecoveryCode(in); got != want {
			t.Errorf("NormalizeRecoveryCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHashRecoveryCodeIgnoresFormatting(t *testing.T) {
	// The user types the code back in however they remember it. All of these have to match the
	// single hash stored at enrolment, or the spare code is useless exactly when it is needed.
	want := HashRecoveryCode("abcde-fghij")
	for _, variant := range []string{"ABCDE-FGHIJ", "abcde fghij", "abcdefghij", " Abcde-Fghij "} {
		if got := HashRecoveryCode(variant); got != want {
			t.Errorf("HashRecoveryCode(%q) does not match the canonical hash", variant)
		}
	}
}

func TestHashRecoveryCodeDistinguishesCodes(t *testing.T) {
	if HashRecoveryCode("abcde-fghij") == HashRecoveryCode("abcde-fghik") {
		t.Error("two different codes hash to the same value")
	}
}

func TestHashRecoveryCodeIsHex(t *testing.T) {
	// Stored in totp_recovery_codes.code_hash; a change in length or encoding here would
	// invalidate every code already issued.
	h := HashRecoveryCode("abcde-fghij")
	if len(h) != 64 {
		t.Fatalf("hash length = %d, want 64 hex characters", len(h))
	}
	if strings.Trim(h, "0123456789abcdef") != "" {
		t.Errorf("hash is not lower-case hex: %q", h)
	}
}

func TestHashRecoveryCodeDoesNotStoreThePlaintext(t *testing.T) {
	const code = "abcde-fghij"
	if strings.Contains(HashRecoveryCode(code), NormalizeRecoveryCode(code)) {
		t.Error("the hash contains the code itself")
	}
}
