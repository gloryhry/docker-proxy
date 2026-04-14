package handler

import "net/http"

var rootHandler = NewHandler()

// Index 处理根路径请求。
func Index(w http.ResponseWriter, r *http.Request) {
	rootHandler.ServeHTTP(w, r)
}
