package main

import (
	dockerproxy "docker-proxy"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}

	log.Fatal(http.ListenAndServe(":"+port, dockerproxy.NewHandler()))
}
