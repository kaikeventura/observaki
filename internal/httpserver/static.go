package httpserver

import (
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/observaki/observaki/internal/api"
)

const (
	indexFile          = "index.html"
	immutableAssetPath = "/_app/immutable/"
	immutableCache     = "public, max-age=31536000, immutable"
	revalidateCache    = "no-cache"
)

// staticHandler serves a SvelteKit static build and falls back to index.html
// for client-side routes. Assets under /_app/immutable/ are content-hashed and
// cache forever; everything else revalidates so an upgrade is picked up
// immediately.
type staticHandler struct {
	dir    string
	logger *slog.Logger
}

func newStaticHandler(dir string, logger *slog.Logger) *staticHandler {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		absolute = dir
	}
	handler := &staticHandler{dir: absolute, logger: logger}
	if info, err := os.Stat(absolute); err != nil {
		logger.Warn("web build directory is not readable; the UI is disabled", "web_dir", absolute, "error", err)
	} else if !info.IsDir() {
		logger.Warn("web build path is not a directory; the UI is disabled", "web_dir", absolute)
	}
	return handler
}

func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		api.WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed",
			r.Method+" is not supported by the web UI", map[string]any{"allow": []string{"GET", "HEAD"}})
		return
	}

	requestedPath := strings.TrimSpace(r.URL.Path)
	if requestedPath == "" || requestedPath == "/" {
		requestedPath = "/" + indexFile
	}
	if strings.Contains(requestedPath, "..") {
		h.notFound(w, r)
		return
	}

	filePath := filepath.Join(h.dir, filepath.FromSlash(path.Clean("/"+requestedPath)))
	if !strings.HasPrefix(filePath, h.dir+string(filepath.Separator)) {
		h.notFound(w, r)
		return
	}

	// Prerendered routes are emitted as "<route>.html" (SvelteKit's default
	// build format) or "<route>/index.html" (directory format). Exact files
	// keep priority, then the two prerender shapes, then the SPA fallback.
	for _, candidate := range []string{filePath, filePath + ".html", filepath.Join(filePath, indexFile)} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			h.serveFile(w, r, candidate, requestedPath)
			return
		}
	}

	// A path with an extension is a missing asset, not a client-side route.
	if filepath.Ext(requestedPath) != "" {
		h.notFound(w, r)
		return
	}

	indexPath := filepath.Join(h.dir, indexFile)
	if _, err := os.Stat(indexPath); err != nil {
		h.notFound(w, r)
		return
	}
	h.serveFile(w, r, indexPath, "/"+indexFile)
}

func (h *staticHandler) serveFile(w http.ResponseWriter, r *http.Request, filePath, servedPath string) {
	file, err := os.Open(filePath)
	if err != nil {
		h.notFound(w, r)
		return
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		h.notFound(w, r)
		return
	}

	if contentType := mime.TypeByExtension(filepath.Ext(filePath)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if strings.HasPrefix(servedPath, immutableAssetPath) {
		w.Header().Set("Cache-Control", immutableCache)
	} else {
		w.Header().Set("Cache-Control", revalidateCache)
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}

func (h *staticHandler) notFound(w http.ResponseWriter, r *http.Request) {
	api.WriteError(w, r, http.StatusNotFound, "not_found",
		"no file matches "+r.URL.Path, map[string]any{"path": r.URL.Path})
}
