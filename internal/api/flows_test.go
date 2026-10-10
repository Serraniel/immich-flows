package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/serraniel/immich-flows/internal/auth"
	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/flows"
)

func TestFlowFiles(t *testing.T) {
	immich := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admin := r.Header.Get("x-api-key") == "admin"
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"0b6f2a1e-4c3a-4d8e-9a4b-1f2e3d4c5b6a","email":"a@b.c","name":"A","isAdmin":`+map[bool]string{true: "true", false: "false"}[admin]+`}`)
	}))
	defer immich.Close()
	u, _ := url.Parse(immich.URL)
	log := slog.New(slog.DiscardHandler)
	reg := engine.NewRegistry()
	dir := flows.NewDir(t.TempDir(), reg, log)
	h := New("/_flows/api", Deps{Immich: u, Auth: auth.New(u, nil), Flows: dir, Schema: reg, Log: log})

	do := func(key, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/_flows/api/"+path, strings.NewReader(body))
		req.Header.Set("x-api-key", key)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	const good = "id: a\ntrigger: {on: manual}\nsteps: [{use: log, with: {message: hi}}]\n"

	if rec := do("user", "PUT", "flow-files/a.yaml", good); rec.Code != http.StatusForbidden {
		t.Errorf("non-admin write: %d", rec.Code)
	}
	if rec := do("admin", "PUT", "flow-files/sub/a.yaml", good); rec.Code != http.StatusNoContent {
		t.Fatalf("write: %d %s", rec.Code, rec.Body)
	}
	if rec := do("admin", "GET", "flow-files/sub/a.yaml", ""); rec.Code != http.StatusOK || rec.Body.String() != good {
		t.Errorf("read: %d %s", rec.Code, rec.Body)
	}
	if rec := do("admin", "GET", "flow-files/nope.yaml", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing: %d", rec.Code)
	}
	if rec := do("admin", "PUT", "flow-files/x.txt", good); rec.Code != http.StatusBadRequest {
		t.Errorf("bad source: %d", rec.Code)
	}

	rec := do("admin", "PUT", "flow-files/b.yaml", "id: b\ntrigger: {on: manual}\nsteps: [{use: nope}]\n")
	var res struct {
		Problems []problem `json:"problems"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || rec.Code != http.StatusUnprocessableEntity || len(res.Problems) != 1 || res.Problems[0].Line != 3 {
		t.Errorf("invalid write: %d %s", rec.Code, rec.Body)
	}

	rec = do("admin", "PUT", "flow-files/b.yaml?dryRun=true", "id: [")
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || rec.Code != http.StatusOK || len(res.Problems) == 0 {
		t.Errorf("check: %d %s", rec.Code, rec.Body)
	}
	rec = do("admin", "PUT", "flow-files/sub/a.yaml?dryRun=true", good)
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || len(res.Problems) != 0 {
		t.Errorf("check valid: %d %s", rec.Code, rec.Body)
	}

	if rec := do("admin", "PATCH", "flow-files/sub/a.yaml", `{"enabled":false}`); rec.Code != http.StatusNoContent {
		t.Errorf("disable: %d %s", rec.Code, rec.Body)
	}
	var entries []flows.Entry
	rec = do("admin", "GET", "flows", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil || len(entries) != 1 || entries[0].Flow.IsEnabled() {
		t.Errorf("list: %s", rec.Body)
	}

	if rec := do("admin", "GET", "flows/schema", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"const":"log"`) {
		t.Errorf("schema: %d", rec.Code)
	}

	if rec := do("admin", "DELETE", "flow-files/sub/a.yaml", ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete: %d", rec.Code)
	}
	if rec := do("admin", "DELETE", "flow-files/sub/a.yaml", ""); rec.Code != http.StatusNotFound {
		t.Errorf("delete again: %d", rec.Code)
	}
}
