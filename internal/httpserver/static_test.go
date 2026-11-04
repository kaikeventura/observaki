package httpserver

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeWebBuild creates a minimal SvelteKit static build in a temporary dir.
func writeWebBuild(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"index.html":                        "<!doctype html><title>Observaki</title>",
		"favicon.svg":                       "<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>",
		"_app/immutable/assets/app.abc.css": "body{color:red}",
	}
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func newWebTestServer(t *testing.T) *Server {
	t.Helper()
	return New(":0", "v0.1.0-test", slog.New(slog.DiscardHandler), WithWebDir(writeWebBuild(t)))
}

func TestUIHidesHealthz(t *testing.T) {
	srv := newWebTestServer(t)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("body = %q, want the JSON health payload", rec.Body.String())
	}
}

func TestUIServesIndex(t *testing.T) {
	srv := newWebTestServer(t)

	for _, target := range []string{"/", "/index.html"} {
		t.Run(target, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if !strings.Contains(rec.Body.String(), "Observaki") {
				t.Fatalf("body = %q, want index.html", rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
				t.Fatalf("Content-Type = %q, want text/html", got)
			}
			if got := rec.Header().Get("Cache-Control"); got != revalidateCache {
				t.Fatalf("Cache-Control = %q, want %q", got, revalidateCache)
			}
		})
	}
}

func TestUIServesSPAFallback(t *testing.T) {
	srv := newWebTestServer(t)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/projects/project-dev/explore", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "Observaki") {
		t.Fatalf("body = %q, want the SPA shell", rec.Body.String())
	}
}

func TestUIServesImmutableAssetsWithLongCache(t *testing.T) {
	srv := newWebTestServer(t)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_app/immutable/assets/app.abc.css", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Cache-Control"); got != immutableCache {
		t.Fatalf("Cache-Control = %q, want %q", got, immutableCache)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
		t.Fatalf("Content-Type = %q, want text/css", got)
	}
}

func TestUIMissingAssetIsNotFound(t *testing.T) {
	srv := newWebTestServer(t)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_app/immutable/assets/missing.js", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUIRejectsTraversal(t *testing.T) {
	srv := newWebTestServer(t)

	for _, target := range []string{"/../../etc/passwd", "/%2e%2e/%2e%2e/etc/passwd"} {
		t.Run(target, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))

			// The mux redirects unclean paths and the static handler rejects the
			// rest; neither may return a file outside the web build.
			if rec.Code == http.StatusOK {
				t.Fatalf("status = %d, want a redirect or an error", rec.Code)
			}
			if strings.Contains(rec.Body.String(), "root:") {
				t.Fatal("traversal leaked /etc/passwd")
			}
		})
	}
}

func TestUIRejectsUnsupportedMethod(t *testing.T) {
	srv := newWebTestServer(t)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if got := rec.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("Allow = %q, want %q", got, "GET, HEAD")
	}
}

func TestAPINotFoundStaysJSON(t *testing.T) {
	srv := newWebTestServer(t)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	var body errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (raw: %q)", err, rec.Body.String())
	}
	if body.Code != "not_found" {
		t.Fatalf("code = %q, want %q", body.Code, "not_found")
	}
}

func TestUIMissingWebBuildIsReported(t *testing.T) {
	srv := New(":0", "v0.1.0-test", slog.New(slog.DiscardHandler), WithWebDir(filepath.Join(t.TempDir(), "absent")))

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	var body errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (raw: %q)", err, rec.Body.String())
	}
	if body.Code != "not_found" {
		t.Fatalf("code = %q, want %q", body.Code, "not_found")
	}
}

func TestNoWebDirKeepsAPIAvailable(t *testing.T) {
	srv := newTestServer(t, "v0.1.0-test")

	api := httptest.NewRecorder()
	srv.Handler().ServeHTTP(api, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if api.Code != http.StatusOK {
		t.Fatalf("api status = %d, want %d", api.Code, http.StatusOK)
	}

	ui := httptest.NewRecorder()
	srv.Handler().ServeHTTP(ui, httptest.NewRequest(http.MethodGet, "/", nil))
	if ui.Code != http.StatusNotFound {
		t.Fatalf("ui status = %d, want %d", ui.Code, http.StatusNotFound)
	}
	var body errorResponse
	if err := json.Unmarshal(ui.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (raw: %q)", err, ui.Body.String())
	}
	if body.Code != "ui_not_installed" {
		t.Fatalf("code = %q, want %q", body.Code, "ui_not_installed")
	}
}
