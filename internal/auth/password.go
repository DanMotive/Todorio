package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64 MB
	argonThreads = 2
	argonKeyLen  = 32
	saltLen      = 16
)

// Accepted ranges for the parameters read back out of a stored hash.
//
// The values in an encoded hash are data, not code: they come from whatever is in the users
// table, which may have been written by an older build, truncated by a botched restore, or
// edited by hand in psql. argon2.IDKey treats them as trusted and reacts badly to degenerate
// ones — it panics on time < 1 and threads < 1 by contract, panics on a zero key length by
// accident (see VerifyPassword), and will happily try to allocate whatever memory it is told to.
// Since m is in KiB, a single hand-edited m=8388608 means an 8 GiB allocation per login attempt.
//
// So every field gets a range check before it reaches the library. The bounds are deliberately
// wider than what HashPassword produces, so that hashes from an older, cheaper configuration
// still verify and existing users can still log in; they only exclude values that are either
// impossible or hostile.
const (
	minStoredMemory  = 8       // argon2 itself raises anything lower to 8*threads
	maxStoredMemory  = 1 << 20 // 1 GiB expressed in KiB
	maxStoredTime    = 16
	maxStoredThreads = 16
	minStoredSaltLen = 8
	minStoredKeyLen  = 16
	maxStoredKeyLen  = 64 // blake2b's maximum digest size
)

// maxConcurrentHashes bounds how many argon2id derivations may run at the same time.
//
// Each one reserves argonMemory (64 MB) for its whole duration, so an unbounded number of
// concurrent logins is a memory-exhaustion vector: a hundred parallel requests to /api/login
// would ask for ~6.4 GB on a box that typically has one or two. The rate limiter does not help
// here, because it only counts *failed* attempts and only after the hash has been computed.
//
// Queueing costs a little latency under a burst and nothing at all otherwise, and it caps the
// memory a login flood can pin at maxConcurrentHashes * argonMemory.
const maxConcurrentHashes = 4

var hashSem = make(chan struct{}, maxConcurrentHashes)

func acquireHash() func() {
	hashSem <- struct{}{}
	return func() { <-hashSem }
}

// HashPassword — argon2id, format $argon2id$v=19$m=...,t=...,p=...$salt$hash.
func HashPassword(password string) (string, error) {
	defer acquireHash()()
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches the encoded argon2id hash.
//
// Anything it cannot make sense of is a false, never a panic: this runs on the login path with
// a string straight out of the database, and a crash there would be a denial of service against
// the whole server triggered by one bad row.
func VerifyPassword(password, encoded string) bool {
	// "$argon2id$v=19$m=..,t=..,p=..$salt$hash" splits into a leading empty field plus five.
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false
	}
	// IDKey implements version 0x13 only, so a hash claiming any other version cannot be
	// reproduced here and must not be silently derived with the wrong algorithm.
	if parts[2] != "v=19" {
		return false
	}
	var m, t uint32
	var p uint8
	if n, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || n != 3 {
		return false
	}
	if m < minStoredMemory || m > maxStoredMemory ||
		t < 1 || t > maxStoredTime ||
		p < 1 || p > maxStoredThreads {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < minStoredSaltLen {
		return false
	}
	// The stored hash decides how many bytes to derive, so an empty or truncated hash field
	// would ask argon2 for a zero-length key. blake2b.New rejects that size and returns a nil
	// digest with an error that argon2's blake2bHash discards, and the following Write then
	// dereferences nil. Length is checked here, before any derivation, rather than trusting the
	// library to reject it.
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) < minStoredKeyLen || len(want) > maxStoredKeyLen {
		return false
	}
	defer acquireHash()()
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

var (
	dummyOnce sync.Once
	dummyHash string
)

// BurnPasswordTime runs one full argon2id verification against a throwaway hash and discards
// the result.
//
// Call it on the "no such user" branch of a login. Without it the two branches are trivially
// distinguishable by timing: a missing account is rejected in microseconds, because
// VerifyPassword bails on the malformed empty hash before doing any work, while a real account
// costs a full 64 MB derivation — tens of milliseconds, comfortably measurable over a network.
// That difference turns the login endpoint into a username oracle regardless of how carefully
// the error message is worded. Burning the same work makes both paths cost the same.
func BurnPasswordTime(password string) {
	dummyOnce.Do(func() {
		// Generated lazily so the cost lands on the first login rather than on every start,
		// including short-lived CLI invocations that never verify a password at all.
		if h, err := HashPassword("todorio/timing-equaliser"); err == nil {
			dummyHash = h
		}
	})
	if dummyHash == "" {
		return
	}
	_ = VerifyPassword(password, dummyHash)
}
