package dashboard

import (
	"embed"
	"net/http"
)

//go:embed index.html
var assets embed.FS

// Handler serves the embedded dashboard from the root path.
func Handler() http.Handler {
	// Read once at startup: a corrupt embed fails fast instead of panicking
	// on first traffic.
	data, err := assets.ReadFile("index.html")
	if err != nil {
		panic(err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
}
