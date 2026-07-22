package api

// Login rate limiting: in-memory, 10-minute window, keyed by IP + username.
// Max attempts is configurable by root: limits.login.max_attempts (default 10).

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const loginWindow = 10 * time.Minute

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
func (rl *rateLimiter) allow(key string, max int) bool {
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

// maxLoginAttempts reads the limit from system_settings.
func (a *API) maxLoginAttempts(r *http.Request) int {
	if n, err := strconv.Atoi(a.DB.Setting(r.Context(), "limits.login.max_attempts", "10")); err == nil && n > 0 {
		return n
	}
	return 10
}
