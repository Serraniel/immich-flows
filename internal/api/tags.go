package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/serraniel/immich-flows/internal/immich"
	"github.com/serraniel/immich-flows/internal/tags"
)

type moveRequest struct {
	tags.Move
	DryRun bool `json:"dryRun"`
}

// moveTag renames, moves or merges a tag of the caller, acting with the caller's credentials.
func (s *Server) moveTag(w http.ResponseWriter, r *http.Request) {
	var req moveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	c, err := immich.New(s.immichURL, immich.Forward(r), nil)
	if err != nil {
		s.internal(w, err)
		return
	}
	svc := tags.New(c)
	op := svc.Apply
	if req.DryRun {
		op = svc.Plan
	}
	plan, err := op(r.Context(), req.Move)
	switch {
	case errors.Is(err, tags.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		s.upstream(w, err)
	default:
		writeJSON(w, http.StatusOK, plan)
	}
}

type bulkRequest struct {
	tags.Bulk
	DryRun bool `json:"dryRun"`
}

// bulkTags moves all of the caller's tags matching a pattern.
func (s *Server) bulkTags(w http.ResponseWriter, r *http.Request) {
	var req bulkRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	c, err := immich.New(s.immichURL, immich.Forward(r), nil)
	if err != nil {
		s.internal(w, err)
		return
	}
	svc := tags.New(c)
	op := svc.ApplyBulk
	if req.DryRun {
		op = svc.PlanBulk
	}
	plan, err := op(r.Context(), req.Bulk)
	switch {
	case errors.Is(err, tags.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		s.upstream(w, err)
	default:
		writeJSON(w, http.StatusOK, plan)
	}
}
