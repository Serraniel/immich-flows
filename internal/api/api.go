// Package api serves the immich-flows JSON API under <prefix>/api.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/serraniel/immich-flows/internal/auth"
	"github.com/serraniel/immich-flows/internal/connect"
	"github.com/serraniel/immich-flows/internal/immich"
)

// Server holds API dependencies.
type Server struct {
	auth      *auth.Authenticator
	conn      *connect.Manager
	immichURL *url.URL
	log       *slog.Logger
}

// New returns the API handler mounted at prefix (e.g. "/_flows/api").
func New(prefix string, immichURL *url.URL, a *auth.Authenticator, conn *connect.Manager, log *slog.Logger) http.Handler {
	s := &Server{auth: a, conn: conn, immichURL: immichURL, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+prefix+"/me", s.me)
	mux.HandleFunc("GET "+prefix+"/connection", s.connectionStatus)
	mux.HandleFunc("POST "+prefix+"/connection", s.connect)
	mux.HandleFunc("DELETE "+prefix+"/connection", s.disconnect)
	mux.HandleFunc("POST "+prefix+"/tags/move", s.moveTag)
	mux.HandleFunc(prefix+"/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	return a.Middleware(mux)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, auth.FromContext(r.Context()))
}

func (s *Server) connectionStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.conn.Status(r.Context(), auth.FromContext(r.Context()).ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) connect(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	if err := s.conn.Connect(r.Context(), u.ID, immich.Forward(r)); err != nil {
		s.upstream(w, err)
		return
	}
	s.connectionStatus(w, r)
}

func (s *Server) disconnect(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	if err := s.conn.Disconnect(r.Context(), u.ID, immich.Forward(r)); err != nil {
		s.upstream(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// upstream maps Immich errors to the response; other errors are internal.
func (s *Server) upstream(w http.ResponseWriter, err error) {
	var ie *immich.Error
	if errors.As(err, &ie) {
		writeError(w, ie.Status, ie.Body)
		return
	}
	s.internal(w, err)
}

func (s *Server) internal(w http.ResponseWriter, err error) {
	s.log.Error("api", "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"message": msg})
}
