package api

// Login rate limiting: in-memory, 15-minute window, keyed by IP + username.
// Max attempts is configurable by root: limits.login.max_attempts (default 10).

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const loginWindow = 15 * time.Minute

// How often stale keys are swept, and how long an untouched key is kept.
//
// Entries used to be dropped only when the same key came back after its window had passed,
// which meant a key that was never seen again stayed in the map forever. Since the key contains
// the submitted username, anyone could grow the map without bound by posting logins for
// randomly generated usernames — a slow but entirely unauthenticated memory leak. Sweeping on a
// timer bounds the map by traffic in the last half hour instead of by uptime.
const (
	rlSweepEvery = 5 * time.Minute
	rlEntryTTL   = 2 * loginWindow
)

type rlEntry struct {
	count       int // failures recorded in this window
	inFlight    int // attempts currently being verified
	windowStart time.Time
	touched     time.Time
}

type rateLimiter struct {
	mu sync.Mutex
	m  map[string]*rlEntry
}

var loginLimiter = newRateLimiter()

func newRateLimiter() *rateLimiter {
	rl := &rateLimiter{m: map[string]*rlEntry{}}
	go rl.sweepLoop()
	return rl
}

// begin reserves one attempt and reports whether it is allowed to proceed.
//
// Checking the limit and claiming a slot happen under the same lock, which is the point. The
// previous allow()/fail() pair left a gap: allow() only read the counter and fail() incremented
// it after the password had been verified, so every request that arrived during that ~50 ms
// argon2 derivation saw the same pre-increment count. Firing a few hundred attempts in parallel
// therefore got a few hundred guesses through a limit of ten. Counting in-flight attempts
// against the limit closes that window.
//
// max <= 0 means "no limit" (spec section 10: 0 = unlimited) — never block, but still track the
// entry so the counters stay meaningful if the setting is raised later.
//
// Every begin that returns true must be paired with exactly one end.
func (rl *rateLimiter) begin(key string, max int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	e, ok := rl.m[key]
	if !ok || now.Sub(e.windowStart) > loginWindow {
		e = &rlEntry{windowStart: now}
		rl.m[key] = e
	}
	e.touched = now
	if max > 0 && e.count+e.inFlight >= max {
		return false
	}
	e.inFlight++
	return true
}

// end releases the slot taken by begin and records the outcome.
func (rl *rateLimiter) end(key string, failed bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	e, ok := rl.m[key]
	if !ok {
		return
	}
	if e.inFlight > 0 {
		e.inFlight--
	}
	if failed {
		e.count++
		e.touched = time.Now()
	}
}

// reset clears the counter after a successful login.
func (rl *rateLimiter) reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.m, key)
}

func (rl *rateLimiter) sweepLoop() {
	for range time.Tick(rlSweepEvery) {
		rl.sweep(time.Now())
	}
}

// sweep drops entries nobody has touched for a while. Keys with an attempt still in flight are
// kept regardless, so a slow verification cannot have its slot swept out from under it.
func (rl *rateLimiter) sweep(now time.Time) int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	removed := 0
	for k, e := range rl.m {
		if e.inFlight == 0 && now.Sub(e.touched) > rlEntryTTL {
			delete(rl.m, k)
			removed++
		}
	}
	return removed
}

// clientIP returns the address a request should be rate-limited by.
//
// Todorio is normally run behind nginx or Caddy on the same host, where RemoteAddr is always
// 127.0.0.1. Keying off it alone collapsed every visitor in the world into a single bucket:
// ten bad logins from anywhere locked out everyone, and an attacker could lock a victim out
// deliberately. X-Forwarded-For is therefore honoured — but only when the immediate peer is
// loopback or a private address, i.e. plausibly our own proxy. A request arriving directly from
// the internet cannot talk its way into a different bucket by sending the header itself.
//
// Within the header the list is walked from the right, because a proxy appends and a client can
// pre-seed anything it likes on the left. The first entry that is not itself a private hop is
// the earliest address we can actually vouch for.
func clientIP(r *http.Request) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	if !isTrustedProxy(peer) {
		return peer
	}
	fwd := r.Header.Get("X-Forwarded-For")
	if fwd == "" {
		if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
			return real
		}
		return peer
	}
	parts := strings.Split(fwd, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(parts[i])
		if ip == "" {
			continue
		}
		if !isTrustedProxy(ip) {
			return ip
		}
	}
	// Everything in the chain was private (e.g. an entirely internal deployment): fall back to
	// the client-most entry rather than lumping them all together as "the proxy".
	if first := strings.TrimSpace(parts[0]); first != "" {
		return first
	}
	return peer
}

// isTrustedProxy reports whether an address is one we assume to be our own infrastructure:
// loopback, link-local, or RFC 1918 / unique-local space.
func isTrustedProxy(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

// maxLoginAttempts reads the limit from system_settings. 0 is a deliberate "no limit" (see
// begin() above), not a signal to fall back to the default.
func (a *API) maxLoginAttempts(r *http.Request) int {
	return a.intSetting(r.Context(), "limits.login.max_attempts", 10)
}
