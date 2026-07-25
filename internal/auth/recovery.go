package auth

// Single-use recovery codes for two-factor authentication.
//
// Without these, losing the authenticator app means losing the account: the only way back in was
// for an admin to clear totp_enabled directly in the database, and on a single-admin instance
// there may be nobody able to do that. A batch is issued when 2FA is switched on, shown once,
// and each code works exactly once — in the login form's code field or when disabling 2FA.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"strings"
)

// RecoveryCodeCount is how many codes are issued per enrolment.
const RecoveryCodeCount = 10

// recoveryAlphabet omits i, l, o, 0 and 1 so a code can be copied off a screen or a scrap of
// paper without the usual transcription traps.
const recoveryAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// recoveryCodeLen is the number of characters per code, excluding the separating dash.
// 10 characters over a 31-symbol alphabet is a little under 50 bits — far beyond guessing, and
// still short enough to write down.
const recoveryCodeLen = 10

// NewRecoveryCodes returns n freshly generated codes in display form ("abcde-fghij").
func NewRecoveryCodes(n int) ([]string, error) {
	codes := make([]string, 0, n)
	for i := 0; i < n; i++ {
		code, err := newRecoveryCode()
		if err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, nil
}

func newRecoveryCode() (string, error) {
	var b strings.Builder
	max := big.NewInt(int64(len(recoveryAlphabet)))
	for i := 0; i < recoveryCodeLen; i++ {
		// rand.Int over the exact alphabet size: taking a byte modulo 31 would make the first few
		// symbols slightly likelier than the rest.
		k, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		if i == recoveryCodeLen/2 {
			b.WriteByte('-')
		}
		b.WriteByte(recoveryAlphabet[k.Int64()])
	}
	return b.String(), nil
}

// NormalizeRecoveryCode reduces a code to the form it is stored in: lower case, with dashes,
// spaces and anything else the user may have typed or pasted removed. "ABCDE-FGHIJ",
// "abcde fghij" and "abcdefghij" are therefore the same code.
func NormalizeRecoveryCode(code string) string {
	var b strings.Builder
	b.Grow(len(code))
	for _, r := range strings.ToLower(strings.TrimSpace(code)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// HashRecoveryCode returns the value stored in totp_recovery_codes.code_hash. The input is
// normalized first, so it is safe to pass either the display form or an already-normalized code.
//
// SHA-256 rather than argon2id, deliberately: unlike a password, a recovery code is generated
// here and carries ~50 bits of entropy, so there is nothing for an offline attacker to guess
// their way through, and a lookup on the login path must not reserve 64 MB per attempt the way
// argon2id does.
func HashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(NormalizeRecoveryCode(code)))
	return hex.EncodeToString(sum[:])
}
