package handler

import "net/http"

var catchAllHandler = NewHandler()

// CatchAll 处理除根路径外的所有请求。
func CatchAll(w http.ResponseWriter, r *http.Request) {
	catchAllHandler.ServeHTTP(w, r)
}
