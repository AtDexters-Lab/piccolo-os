package server

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// This file would contain all the HTTP handler functions for our API.
// For brevity in this skeleton, we'll keep them minimal.

func (s *Server) handleRoot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<h1>Welcome to Piccolo OS</h1><p>The piccolod API is running.</p>")
	}
}

func (s *Server) handleContainers() http.HandlerFunc {
	// This handler would route to different methods based on r.Method (GET, POST)
	return func(w http.ResponseWriter, r *http.Request) {
		s.respondJSON(w, http.StatusNotImplemented, map[string]string{"message": "Not implemented yet"})
	}
}

func (s *Server) handleSingleContainer() http.HandlerFunc {
	// This handler would parse the container ID from the URL and route based on method (GET, DELETE)
	return func(w http.ResponseWriter, r *http.Request) {
		s.respondJSON(w, http.StatusNotImplemented, map[string]string{"message": "Not implemented yet"})
	}
}

// respondJSON is a helper to write JSON responses.
func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}
