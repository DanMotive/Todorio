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

func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
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
