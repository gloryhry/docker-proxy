package handler

import (
	"docker-proxy-edgeone/shared"
	"net/http"
)

// Handler is the EdgeOne Pages entrypoint for "/".
func Handler(w http.ResponseWriter, r *http.Request) {
	shared.NewHandler().ServeHTTP(w, r)
}
