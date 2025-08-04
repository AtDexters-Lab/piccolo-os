package server

import (
	"encoding/json"
	"net/http"
)

// Version represents the application version information.
type Version struct {
	Version string `json:"version"`
}

// handleVersion returns a handler that writes the application's version as JSON.
func (s *Server) handleVersion() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v := Version{
			Version: s.version,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(v); err != nil {
			http.Error(w, "Failed to encode version", http.StatusInternalServerError)
		}
	}
}
