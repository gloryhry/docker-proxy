package main

import (
	"docker-proxy-edgeone/internal/proxy"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Fatal(http.ListenAndServe(":"+port, proxy.NewHandler()))
}
