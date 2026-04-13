package handler

import (
	dockerproxy "docker-proxy"
	"net/http"
)

var sharedHandler = dockerproxy.NewHandler()

// Handler 作为 Vercel 的单函数入口，通过 rewrite 后恢复原始路径。
func Handler(w http.ResponseWriter, r *http.Request) {
	sharedHandler.ServeHTTP(w, restoreOriginalPath(r))
}

func restoreOriginalPath(r *http.Request) *http.Request {
	cloned := r.Clone(r.Context())
	values := cloned.URL.Query()
	originalPath := values.Get("__path")
	values.Del("__path")
	cloned.URL.RawQuery = values.Encode()
	if originalPath == "" {
		originalPath = "/"
	}
	cloned.URL.Path = originalPath
	return cloned
}
