package api

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"syscall"

	"github.com/serraniel/immich-flows/internal/flow"
	"github.com/serraniel/immich-flows/internal/flows"
)

// FlowStore lists and edits flow files.
type FlowStore interface {
	Entries() []flows.Entry
	Read(source string) ([]byte, error)
	Check(source string, data []byte) (*flow.Flow, error)
	Write(source string, data []byte) error
	SetEnabled(source string, enabled bool) error
	Delete(source string) error
}

// Schemer returns the flow JSON Schema including the registered steps.
type Schemer interface {
	FlowSchema() (map[string]any, error)
}

type problem struct {
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	Message string `json:"message"`
}

func (s *Server) listFlows(w http.ResponseWriter, _ *http.Request) {
	entries := s.flows.Entries()
	if entries == nil {
		entries = []flows.Entry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) flowSchema(w http.ResponseWriter, _ *http.Request) {
	sch, err := s.schema.FlowSchema()
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sch)
}

func (s *Server) readFlow(w http.ResponseWriter, r *http.Request) {
	data, err := s.flows.Read(r.PathValue("source"))
	if err != nil {
		s.flowError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data) // #nosec G705 -- served as YAML with nosniff, never rendered as HTML
}

// writeFlow writes a flow file. With ?dryRun=true it only validates and returns the
// problems with status 200.
func (s *Server) writeFlow(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, flows.MaxSize))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "flow file too large")
		return
	}
	source := r.PathValue("source")
	if r.URL.Query().Get("dryRun") != "true" {
		if err := s.flows.Write(source, data); err != nil {
			s.flowError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_, err = s.flows.Check(source, data)
	var errs flow.Errors
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"problems": []problem{}})
	case errors.As(err, &errs):
		writeJSON(w, http.StatusOK, map[string]any{"problems": problems(errs)})
	default:
		s.flowError(w, err)
	}
}

func (s *Server) enableFlow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.flows.SetEnabled(r.PathValue("source"), req.Enabled); err != nil {
		s.flowError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteFlow(w http.ResponseWriter, r *http.Request) {
	if err := s.flows.Delete(r.PathValue("source")); err != nil {
		s.flowError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) flowError(w http.ResponseWriter, err error) {
	var errs flow.Errors
	var e flow.Error
	switch {
	case errors.Is(err, flows.ErrSource):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.As(err, &errs):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"message": "invalid flow", "problems": problems(errs)})
	case errors.As(err, &e):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"message": "invalid flow", "problems": problems(flow.Errors{e})})
	case errors.Is(err, fs.ErrNotExist):
		writeError(w, http.StatusNotFound, "flow file not found")
	case errors.Is(err, fs.ErrPermission), errors.Is(err, syscall.EROFS):
		writeError(w, http.StatusConflict, "flows directory is not writable")
	default:
		s.internal(w, err)
	}
}

func problems(errs flow.Errors) []problem {
	out := make([]problem, len(errs))
	for i, e := range errs {
		out[i] = problem{Line: e.Line, Col: e.Col, Message: e.Msg}
	}
	return out
}
