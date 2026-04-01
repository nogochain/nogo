package explorer

import (
	"net/http"
	"strings"
)

type explorerFileServer struct {
	http.Handler
}

func NewExplorerFileServer() http.Handler {
	return &explorerFileServer{
		Handler: http.FileServer(http.FS(ExplorerFiles)),
	}
}

func ServeExplorer(w http.ResponseWriter, r *http.Request) {
	acceptHTML := false
	for _, v := range r.Header["Accept"] {
		if strings.Contains(v, "text/html") {
			acceptHTML = true
			break
		}
	}

	if !acceptHTML && r.Header.Get("X-Requested-With") == "" {
		http.Error(w, "406 Not Acceptable", http.StatusNotAcceptable)
		return
	}

	index, err := ExplorerFiles.ReadFile("explorer/index.html")
	if err != nil {
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(index)
}
