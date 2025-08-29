package main

import (
	"log"
	"piccolod/internal/server"
)

var version = "dev"

func main() {
	// The main function is the entry point. Its only job is to
	// initialize and start the Gin-based server.
	srv, err := server.NewGinServer(server.WithGinVersion(version))
	if err != nil {
		log.Fatalf("FATAL: Failed to initialize server: %v", err)
	}

	if err := srv.Start(); err != nil {
		log.Fatalf("FATAL: Server failed to start: %v", err)
	}
}
