package main

import (
	"log"
	"piccolod/internal/server" // Fictional import path for structure
)

func main() {
	// The main function is the entry point. Its only job is to
	// initialize and start the server.
	srv, err := server.New()
	if err != nil {
		log.Fatalf("FATAL: Failed to initialize server: %v", err)
	}

	if err := srv.Start(); err != nil {
		log.Fatalf("FATAL: Server failed to start: %v", err)
	}
}
