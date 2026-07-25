package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestRateLimiterBlocksAtTheLimit(t *testing.T) {
	rl := newRateLimiter()
	const max = 3

	for i := 0; i < max; i++ {
		if !rl.begin("k", max) {
			t.Fatalf("attempt %d was blocked before the limit was reached", i+1)
		}
		rl.end("k", true)
	}
	if rl.begin("k", max) {
		t.Error("an attempt past the limit was allowed")
	}
}

func TestRateLimiterCountsInFlightAttempts(t *testing.T) {
	// The bug this replaced: the old allow()/fail() pair only incremented the counter after
	// argon2 had finished, so every request that arrived during those ~50 ms saw the same
	// pre-increment count and sailed through. Attempts still being verified now occupy a slot.
	rl := newRateLimiter()
	const max = 3

	for i := 0; i < max; i++ {
		if !rl.begin("k", max) {
			t.Fatalf("concurrent attempt %d was blocked early", i+1)
		}
		// deliberately no end(): these are still in flight
	}
	if rl.begin("k", max) {
		t.Error("a fourth attempt was allowed while three were still being verified")
	}
}

func TestRateLimiterSuccessfulAttemptsDoNotCount(t *testing.T) {
	// Only failures accumulate. Someone logging in and out repeatedly on a shared address must
	// not lock themselves out.
	rl := newRateLimiter()
	for i := 0; i < 50; i++ {
		if !rl.begin("k", 3) {
			t.Fatalf("successful attempt %d was blocked", i+1)
		}
		rl.end("k", false)
	}
}

func TestRateLimiterResetClearsTheCounter(t *testing.T) {
	rl := newRateLimiter()
	for i := 0; i < 3; i++ {
		rl.begin("k", 3)
		rl.end("k", true)
	}
	rl.reset("k")
	if !rl.begin("k", 3) {
		t.Error("the counter survived a reset after a successful login")
	}
}

func TestRateLimiterZeroMaxMeansUnlimited(t *testing.T) {
	// 0 is the documented “no limit” setting, not a limit of zero that locks everyone out.
	rl := newRateLimiter()
	for i := 0; i < 100; i++ {
		if !rl.begin("k", 0) {
			t.Fatalf("attempt %d was blocked although the limit is disabled", i+1)
		}
		rl.end("k", true)
	}
}

func TestRateLimiterKeysAreIndependent(t *testing.T) {
	rl := newRateLimiter()
	for i := 0; i < 3; i++ {
		rl.begin("1.2.3.4|alice", 3)
		rl.end("1.2.3.4|alice", true)
	}
	if !rl.begin("1.2.3.4|bob", 3) {
		t.Error("exhausting one key locked out another")
	}
}

func TestRateLimiterEndWithoutBeginIsHarmless(t *testing.T) {
	// Handlers call end() from a defer; a path that returns before begin() must not panic or
	// drive the in-flight count negative.
	rl := newRateLimiter()
	rl.end("never seen", true)
	rl.end("never seen", false)
	if !rl.begin("never seen", 1) {
		t.Error("a stray end() left the key in a blocked state")
	}
}

// Run with -race: this is the property the lock exists for.
func TestRateLimiterConcurrentUse(t *testing.T) {
	rl := newRateLimiter()
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i%4)
			for j := 0; j < 100; j++ {
				if rl.begin(key, 10) {
					rl.end(key, j%2 == 0)
				}
				if j%25 == 0 {
					rl.reset(key)
				}
				rl.sweep(time.Now())
			}
		}(i)
	}
	wg.Wait()
}

func TestRateLimiterSweepDropsStaleKeys(t *testing.T) {
	// The map is keyed partly by the submitted username, so without sweeping, posting logins for
	// randomly generated usernames grows it without bound — an unauthenticated memory leak.
	rl := newRateLimiter()
	rl.begin("stale", 10)
	rl.end("stale", true)

	if removed := rl.sweep(time.Now()); removed != 0 {
		t.Errorf("a key touched just now was swept (%d removed)", removed)
	}
	if removed := rl.sweep(time.Now().Add(rlEntryTTL + time.Minute)); removed != 1 {
		t.Errorf("stale key was not swept: %d removed", removed)
	}

	rl.mu.Lock()
	size := len(rl.m)
	rl.mu.Unlock()
	if size != 0 {
		t.Errorf("%d entries left in the map after sweeping", size)
	}
}

func TestRateLimiterSweepKeepsInFlightKeys(t *testing.T) {
	// A verification slower than the TTL must not have its slot swept out from under it: end()
	// would then find no entry and the failure would go unrecorded.
	rl := newRateLimiter()
	rl.begin("slow", 10)
	if removed := rl.sweep(time.Now().Add(rlEntryTTL + time.Hour)); removed != 0 {
		t.Errorf("an in-flight key was swept (%d removed)", removed)
	}
}

func TestRateLimiterWindowExpires(t *testing.T) {
	// A lockout is temporary. After the window passes, the next attempt starts a fresh count.
	rl := newRateLimiter()
	for i := 0; i < 3; i++ {
		rl.begin("k", 3)
		rl.end("k", true)
	}
	if rl.begin("k", 3) {
		t.Fatal("the limit did not apply")
	}

	rl.mu.Lock()
	rl.m["k"].windowStart = time.Now().Add(-loginWindow - time.Minute)
	rl.mu.Unlock()

	if !rl.begin("k", 3) {
		t.Error("the counter did not roll over into a new window")
	}
}

func TestClientIP(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		forwarded  string
		realIP     string
		want       string
	}{
		{
			name:       "direct connection",
			remoteAddr: "203.0.113.7:54321",
			want:       "203.0.113.7",
		},
		{
			// The header is attacker-controlled unless the peer is our own proxy. Honouring it
			// here would let anyone pick a fresh bucket per request and skip the limit entirely.
			name:       "forged header from the internet",
			remoteAddr: "203.0.113.7:54321",
			forwarded:  "1.1.1.1",
			want:       "203.0.113.7",
		},
		{
			name:       "behind a local reverse proxy",
			remoteAddr: "127.0.0.1:44444",
			forwarded:  "198.51.100.9",
			want:       "198.51.100.9",
		},
		{
			// A proxy appends; the client can pre-seed anything on the left. Walking from the
			// right finds the earliest hop we can actually vouch for.
			name:       "client-supplied entries are ignored",
			remoteAddr: "127.0.0.1:44444",
			forwarded:  "9.9.9.9, 198.51.100.9",
			want:       "198.51.100.9",
		},
		{
			name:       "internal hops are skipped",
			remoteAddr: "127.0.0.1:44444",
			forwarded:  "198.51.100.9, 10.0.0.5, 172.16.0.2",
			want:       "198.51.100.9",
		},
		{
			name:       "whitespace is trimmed",
			remoteAddr: "127.0.0.1:44444",
			forwarded:  "  198.51.100.9  ",
			want:       "198.51.100.9",
		},
		{
			// An entirely internal deployment: nothing in the chain is public, so the client-most
			// entry is still better than collapsing everyone onto the proxy address.
			name:       "all hops private",
			remoteAddr: "127.0.0.1:44444",
			forwarded:  "10.0.0.5, 10.0.0.6",
			want:       "10.0.0.5",
		},
		{
			name:       "x-real-ip fallback",
			remoteAddr: "127.0.0.1:44444",
			realIP:     "198.51.100.9",
			want:       "198.51.100.9",
		},
		{
			name:       "proxy with no forwarding headers",
			remoteAddr: "127.0.0.1:44444",
			want:       "127.0.0.1",
		},
		{
			name:       "ipv6 peer",
			remoteAddr: "[2001:db8::1]:44444",
			want:       "2001:db8::1",
		},
		{
			name:       "ipv6 loopback proxy",
			remoteAddr: "[::1]:44444",
			forwarded:  "198.51.100.9",
			want:       "198.51.100.9",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
			r.RemoteAddr = tc.remoteAddr
			if tc.forwarded != "" {
				r.Header.Set("X-Forwarded-For", tc.forwarded)
			}
			if tc.realIP != "" {
				r.Header.Set("X-Real-IP", tc.realIP)
			}
			if got := clientIP(r); got != tc.want {
				t.Errorf("clientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsTrustedProxy(t *testing.T) {
	trusted := []string{"127.0.0.1", "::1", "10.0.0.5", "172.16.0.2", "192.168.1.10", "169.254.0.1", "fd00::1", "0.0.0.0"}
	for _, addr := range trusted {
		if !isTrustedProxy(addr) {
			t.Errorf("%s should be treated as our own infrastructure", addr)
		}
	}
	public := []string{"203.0.113.7", "8.8.8.8", "2001:db8::1", "", "not an ip", "198.51.100.9:443"}
	for _, addr := range public {
		if isTrustedProxy(addr) {
			t.Errorf("%q should not be trusted", addr)
		}
	}
}
