package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAHandlerServesDeepLinks(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":        &fstest.MapFile{Data: []byte("spa-index")},
		"assets/app.js":     &fstest.MapFile{Data: []byte("app-js")},
	}
	handler := spaHandler(fs.FS(dist))

	for _, path := range []string{
		"/app/my",
		"/app/spaces/12",
		"/app/spaces/12/lists/34",
		"/app/tasks/56",
		"/app/notes/78",
		"/s/sharetoken",
	} {
		t.Run(path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}
			if strings.TrimSpace(w.Body.String()) != "spa-index" {
				t.Fatalf("body = %q, want SPA index", w.Body.String())
			}
		})
	}
}

func TestSPAHandlerKeepsAssetsAndAPINotFound(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("spa-index")},
		"assets/app.js": &fstest.MapFile{Data: []byte("app-js")},
	}
	handler := spaHandler(fs.FS(dist))

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if asset.Code != http.StatusOK || strings.TrimSpace(asset.Body.String()) != "app-js" {
		t.Fatalf("asset response = %d %q", asset.Code, asset.Body.String())
	}

	api := httptest.NewRecorder()
	handler.ServeHTTP(api, httptest.NewRequest(http.MethodGet, "/api/missing", nil))
	if api.Code != http.StatusNotFound {
		t.Fatalf("API status = %d, want %d", api.Code, http.StatusNotFound)
	}
}
