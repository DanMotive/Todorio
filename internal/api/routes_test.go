package api

import (
	"net/http"
	"testing"
)

// TestRoutesRegisterWithoutConflict guards against a real risk with Go's net/http
// ServeMux pattern syntax ("/api/tasks/{id}" vs "/api/tasks/{id}/restore", etc.):
// two patterns that could match the same request the same way panic at
// registration time, not at request time. A plain `go build` can't catch that —
// only actually calling Routes() can. This is intentionally not a full handler
// test suite (there's no test DB wiring here yet); it's a cheap tripwire for the
// one mistake that's easy to make when adding new nested routes.
func TestRoutesRegisterWithoutConflict(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Routes() panicked (likely a conflicting mux pattern): %v", r)
		}
	}()
	a := &API{}
	mux := http.NewServeMux()
	a.Routes(mux)
}
