package cmd

import (
	_ "embed"
	"net/http"
	"strings"
)

//go:embed ui_static/index.html
var uiIndexHTML string

// serveUIStatic serves the embedded console page at /. The token arrives via
// ?token= in the URL and is moved to localStorage by the page itself.
func serveUIStatic(mux *http.ServeMux) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/assets/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(uiIndexHTML))
	})
}
