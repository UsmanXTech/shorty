package dashboard

import (
	"embed"
	"net/http"
)

//go:embed index.html
var assets embed.FS

// Handler serves the embedded dashboard from the root path.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(mustReadIndex())
	})
}

func mustReadIndex() []byte {
	data, err := assets.ReadFile("index.html")
	if err != nil {
		panic(err)
	}
	return data
}
