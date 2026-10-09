package hooks

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"testing"

	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/immichtest"
	"github.com/serraniel/immich-flows/internal/proxy"
)

const (
	a1 = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	a2 = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

// trashSetup returns a proxy with the trash hooks in front of an upstream that echoes success,
// and a fake Immich (for snapshots and trash listings).
func trashSetup(t *testing.T, wants map[string]bool) (*httptest.Server, *recorder, *immichtest.Fake, *[]string) {
	t.Helper()
	var bodies []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, r.Method+" "+r.URL.Path+" "+string(b))
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":2}`))
	}))
	t.Cleanup(up.Close)
	upURL, _ := url.Parse(up.URL)

	f := immichtest.New()
	fsrv := httptest.NewServer(f)
	t.Cleanup(fsrv.Close)
	fURL, _ := url.Parse(fsrv.URL)

	rec := newRecorder()
	d := Deps{Users: fakeUsers{}, Publish: rec.publish, Log: slog.New(slog.DiscardHandler), Immich: fURL,
		Wants: func(typ string) bool { return wants[typ] }}
	srv := httptest.NewServer(proxy.New(upURL, slog.New(slog.DiscardHandler), Trash(d)...))
	t.Cleanup(srv.Close)
	return srv, rec, f, &bodies
}

func do(t *testing.T, method, url, body string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
}

func TestTrashAndDelete(t *testing.T) {
	all := map[string]bool{events.AssetTrashed: true, events.AssetDeleted: true, events.AssetRestored: true}
	srv, rec, f, bodies := trashSetup(t, all)
	f.Info[a1] = map[string]any{"originalFileName": "x.jpg"}
	f.Tag(a1, "keep")

	body := `{"ids":["` + a1 + `","` + a2 + `"]}`
	do(t, http.MethodDelete, srv.URL+"/api/assets", body)
	if ev := rec.wait(t); ev.Type != events.AssetTrashed || len(ev.AssetIDs) != 2 || ev.Data != nil {
		t.Errorf("trash: %+v", ev)
	}
	if (*bodies)[0] != "DELETE /api/assets "+body {
		t.Errorf("upstream body changed: %q", (*bodies)[0])
	}

	do(t, http.MethodDelete, srv.URL+"/api/assets", `{"ids":["`+a1+`"],"force":true}`)
	ev := rec.wait(t)
	snap, _ := ev.Data["assets"].(map[string]any)[a1].(map[string]any)
	if ev.Type != events.AssetDeleted || snap["originalFileName"] != "x.jpg" {
		t.Errorf("delete: %+v", ev)
	}
}

func TestTrashEmptyAndRestore(t *testing.T) {
	all := map[string]bool{events.AssetDeleted: true, events.AssetRestored: true}
	srv, rec, f, _ := trashSetup(t, all)
	f.Trashed[a1], f.Trashed[a2] = true, true

	do(t, http.MethodPost, srv.URL+"/api/trash/empty", "")
	ev := rec.wait(t)
	sort.Strings(ev.AssetIDs)
	if ev.Type != events.AssetDeleted || len(ev.AssetIDs) != 2 || ev.AssetIDs[0] != a1 {
		t.Errorf("empty: %+v", ev)
	}

	do(t, http.MethodPost, srv.URL+"/api/trash/restore", "")
	if ev := rec.wait(t); ev.Type != events.AssetRestored || len(ev.AssetIDs) != 2 {
		t.Errorf("restore all: %+v", ev)
	}

	do(t, http.MethodPost, srv.URL+"/api/trash/restore/assets", `{"ids":["`+a2+`"]}`)
	if ev := rec.wait(t); ev.Type != events.AssetRestored || len(ev.AssetIDs) != 1 || ev.AssetIDs[0] != a2 {
		t.Errorf("restore assets: %+v", ev)
	}
}

func TestTrashNoFlowsNoWork(t *testing.T) {
	srv, rec, f, _ := trashSetup(t, nil)
	f.Trashed[a1] = true
	do(t, http.MethodPost, srv.URL+"/api/trash/empty", "")
	do(t, http.MethodDelete, srv.URL+"/api/assets", `{"ids":["`+a1+`"],"force":true}`)
	rec.none(t)
	if n := f.Pages + f.Requests["GET /assets/"+a1]; n != 0 {
		t.Errorf("immich queried %d times without listening flows", n)
	}
}
