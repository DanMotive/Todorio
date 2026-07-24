package api

// Login rate limiting: in-memory, 15-minute window, keyed by IP + username.
// Max attempts is configurable by root: limits.login.max_attempts (default 10).

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const loginWindow = 15 * time.Minute

type rlEntry struct {
	count       int
	windowStart time.Time
}

type rateLimiter struct {
	mu sync.Mutex
	m  map[string]*rlEntry
}

var loginLimiter = &rateLimiter{m: map[string]*rlEntry{}}

// allow checks whether the limit is already exhausted (does not increment the counter).
// max <= 0 means "no limit" (spec section 10: 0 = unlimited) — never block.
func (rl *rateLimiter) allow(key string, max int) bool {
	if max <= 0 {
		return true
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	e, ok := rl.m[key]
	if !ok || time.Since(e.windowStart) > loginWindow {
		delete(rl.m, key)
		return true
	}
	return e.count < max
}

// fail records a failed attempt.
func (rl *rateLimiter) fail(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	e, ok := rl.m[key]
	if !ok || time.Since(e.windowStart) > loginWindow {
		rl.m[key] = &rlEntry{count: 1, windowStart: time.Now()}
		return
	}
	e.count++
}

// reset clears the counter after a successful login.
func (rl *rateLimiter) reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.m, key)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// maxLoginAttempts reads the limit from system_settings. 0 is a deliberate "no limit" (see
// allow() below), not a signal to fall back to the default.
func (a *API) maxLoginAttempts(r *http.Request) int {
	return a.intSetting(r.Context(), "limits.login.max_attempts", 10)
}
