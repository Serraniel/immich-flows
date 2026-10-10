package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/serraniel/immich-flows/internal/auth"
	"github.com/serraniel/immich-flows/internal/connect"
	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/immich"
	"github.com/serraniel/immich-flows/internal/runs"
)

const maxManualAssets = 500000

// Runner starts flow runs.
type Runner interface {
	RunFlow(ctx context.Context, q engine.Enqueuer, flowID, userID string, assetIDs []string) (int, error)
}

type runRequest struct {
	AssetIDs []string `json:"assetIds"`
	All      bool     `json:"all"` // all of the caller's assets
}

// runFlow queues a flow for the caller's assets.
func (s *Server) runFlow(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	var req runRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.All == (len(req.AssetIDs) > 0) {
		writeError(w, http.StatusBadRequest, "set either assetIds or all")
		return
	}
	if st, err := s.conn.Status(r.Context(), u.ID); err != nil || !st.Connected {
		writeError(w, http.StatusConflict, connect.ErrNotConnected.Error())
		return
	}
	for _, id := range req.AssetIDs {
		var u openapi_types.UUID
		if u.UnmarshalText([]byte(id)) != nil {
			writeError(w, http.StatusBadRequest, "invalid asset id: "+id)
			return
		}
	}
	ids := req.AssetIDs
	if req.All {
		var err error
		if ids, err = s.allAssets(r); err != nil {
			s.upstream(w, err)
			return
		}
	}
	n, err := s.runner.RunFlow(r.Context(), s.jobs, r.PathValue("id"), u.ID, ids)
	switch {
	case errors.Is(err, engine.ErrUnknownFlow):
		writeError(w, http.StatusNotFound, err.Error())
	case err != nil:
		s.internal(w, err)
	default:
		writeJSON(w, http.StatusAccepted, map[string]int{"queued": n})
	}
}

// allAssets lists the caller's asset IDs (not trashed).
func (s *Server) allAssets(r *http.Request) ([]string, error) {
	c, err := immich.New(s.immichURL, immich.Forward(r), nil)
	if err != nil {
		return nil, err
	}
	var ids []string
	size := 1000
	for page := 1; len(ids) < maxManualAssets; page++ {
		p := page
		res, err := c.SearchAssetsWithResponse(r.Context(), nil, immich.MetadataSearchDto{Page: &p, Size: &size})
		if err != nil {
			return nil, err
		}
		if err := immich.Check(res, res.Body); err != nil {
			return nil, err
		}
		for _, a := range res.JSON200.Assets.Items {
			ids = append(ids, a.Id.String())
		}
		if res.JSON200.Assets.NextPage == nil {
			break
		}
	}
	return ids, nil
}

// RunLog reads recorded runs.
type RunLog interface {
	List(ctx context.Context, f runs.Filter) ([]runs.Run, error)
	Get(ctx context.Context, id int64) (*runs.Run, error)
	Request(ctx context.Context, id int64, v any) error
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	q := r.URL.Query()
	f := runs.Filter{FlowID: q.Get("flow"), Status: q.Get("status")}
	f.Before, _ = strconv.ParseInt(q.Get("before"), 10, 64)
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	if !u.IsAdmin || q.Get("mine") == "true" {
		f.UserID = u.ID
	}
	list, err := s.runLog.List(r.Context(), f)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// ownRun loads a run the caller may see (own, or any for admins).
func (s *Server) ownRun(w http.ResponseWriter, r *http.Request) (*runs.Run, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return nil, false
	}
	run, err := s.runLog.Get(r.Context(), id)
	u := auth.FromContext(r.Context())
	switch {
	case errors.Is(err, runs.ErrNotFound) || (err == nil && !u.IsAdmin && run.UserID != u.ID):
		writeError(w, http.StatusNotFound, "run not found")
		return nil, false
	case err != nil:
		s.internal(w, err)
		return nil, false
	}
	return run, true
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	if run, ok := s.ownRun(w, r); ok {
		writeJSON(w, http.StatusOK, run)
	}
}

// retryRun queues the run's original request again.
func (s *Server) retryRun(w http.ResponseWriter, r *http.Request) {
	run, ok := s.ownRun(w, r)
	if !ok {
		return
	}
	var req engine.RunRequest
	if err := s.runLog.Request(r.Context(), run.ID, &req); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if _, err := s.jobs.Enqueue(r.Context(), engine.JobRun, req); err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"queued": 1})
}
