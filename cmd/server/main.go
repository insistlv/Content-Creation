package main

import (
	"log"
	"net/http"
	"os"

	"image-agent/internal/app"
)

func main() {
	cfg, err := app.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	application, err := app.New(cfg)
	if err != nil {
		log.Fatalf("boot app: %v", err)
	}

	server := &http.Server{
		Addr:    listenAddr(),
		Handler: application.Router(),
	}
	log.Printf("listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

func listenAddr() string {
	addr := os.Getenv("LISTEN_ADDR")
	if addr != "" {
		return addr
	}
	return ":" + listenPort()
}

func listenPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		return "8888"
	}
	return port
}
