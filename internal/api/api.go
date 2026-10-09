// Package api serves the immich-flows JSON API under <prefix>/api.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/serraniel/immich-flows/internal/auth"
)

// Server holds API dependencies.
type Server struct {
	auth *auth.Authenticator
}

// New returns the API handler mounted at prefix (e.g. "/_flows/api").
func New(prefix string, a *auth.Authenticator) http.Handler {
	s := &Server{auth: a}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+prefix+"/me", s.me)
	mux.HandleFunc(prefix+"/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	return a.Middleware(mux)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, auth.FromContext(r.Context()))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"message": msg})
}
