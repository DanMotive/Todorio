package api

// Рейт-лимит логина: in-memory, окно 10 минут, ключ = IP + логин.
// Максимум попыток настраивается рутом: limits.login.max_attempts (дефолт 10).

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

// allow проверяет, не исчерпан ли лимит (не увеличивает счётчик).
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

// fail регистрирует неудачную попытку.
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

// reset сбрасывает счётчик после успешного входа.
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

// maxLoginAttempts читает лимит из system_settings.
func (a *API) maxLoginAttempts(r *http.Request) int {
	if n, err := strconv.Atoi(a.DB.Setting(r.Context(), "limits.login.max_attempts", "10")); err == nil && n > 0 {
		return n
	}
	return 10
}
