package auth

// TOTP (RFC 6238) using only the standard library: HMAC-SHA1, 30s step, 6 digits, ±1 window.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// totpStep is the 30-second window from the spec; codes are valid for the current window plus
// one on either side, to allow for clock drift between the phone and the server.
const totpStep int64 = 30

func NewTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b32.EncodeToString(b), nil
}

func totpCode(secret string, counter uint64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha1.New, key)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff) % 1_000_000
	return fmt.Sprintf("%06d", code), nil
}

// VerifyTOTPAt checks a code and reports which 30-second counter it belonged to.
//
// Returning the counter is what makes replay protection possible: the caller stores the highest
// counter it has accepted for the account and passes it back as minCounter, so a code that has
// already been used is refused for the remainder of its ±1 window instead of staying valid for
// another minute and a half. That matters because the code travels in a URL-less POST body but
// is still short-lived shared knowledge — shoulder-surfed, pasted into the wrong window, or
// replayed by anything sitting between the phone and the server.
//
// Pass minCounter = 0 when there is nothing to compare against yet (initial enrolment).
//
// The whole window is scanned even after a match, and the comparison is constant-time, so the
// time taken does not reveal how close a guess was.
func VerifyTOTPAt(secret, code string, minCounter int64) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != 6 || secret == "" {
		return 0, false
	}
	now := time.Now().Unix() / totpStep
	var matched int64
	found := 0
	for w := int64(-1); w <= 1; w++ {
		counter := now + w
		if counter <= minCounter {
			continue // already spent: this window cannot be used again
		}
		want, err := totpCode(secret, uint64(counter))
		if err != nil {
			return 0, false // malformed secret; no point trying the other windows
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			matched = counter
			found = 1
		}
	}
	return matched, found == 1
}

// VerifyTOTP accepts the code from the current, previous, and next 30-second window.
//
// Kept for callers that have no counter to persist. Prefer VerifyTOTPAt, which additionally
// rejects a code that has already been presented.
func VerifyTOTP(secret, code string) bool {
	_, ok := VerifyTOTPAt(secret, code, 0)
	return ok
}

// TOTPURL — otpauth link for a QR code in any authenticator app (Google Authenticator, Aegis, etc.).
func TOTPURL(secret, account, issuer string) string {
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&digits=6&period=30",
		url.PathEscape(issuer), url.PathEscape(account), secret, url.QueryEscape(issuer))
}
