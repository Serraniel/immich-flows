package hooks

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/immichtest"
	"github.com/serraniel/immich-flows/internal/proxy"
)

func TestDuplicatesResolved(t *testing.T) {
	const a3 = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"g1","success":true},{"id":"g2","success":false,"error":"not_found"}]`))
	}))
	defer up.Close()
	upURL, _ := url.Parse(up.URL)
	f := immichtest.New()
	f.Tag(a1, "hashes/MD5/x")
	f.Tag(a2, "hashes/MD5/y")
	fsrv := httptest.NewServer(f)
	defer fsrv.Close()
	fURL, _ := url.Parse(fsrv.URL)

	rec := newRecorder()
	d := Deps{Users: fakeUsers{}, Publish: rec.publish, Log: slog.New(slog.DiscardHandler), Immich: fURL,
		Wants: func(string) bool { return true }}
	srv := httptest.NewServer(proxy.New(upURL, slog.New(slog.DiscardHandler), Duplicates(d)))
	defer srv.Close()

	do(t, http.MethodPost, srv.URL+"/api/duplicates/resolve", fmt.Sprintf(
		`{"groups":[{"duplicateId":"g1","keepAssetIds":[%q],"trashAssetIds":[%q]},{"duplicateId":"g2","keepAssetIds":[%q],"trashAssetIds":[]}]}`, a1, a2, a3))

	ev := rec.wait(t)
	if ev.Type != events.DuplicatesResolved || len(ev.AssetIDs) != 1 || ev.AssetIDs[0] != a1 || ev.Data["duplicateId"] != "g1" {
		t.Fatalf("resolved: %+v", ev)
	}
	tags := ev.Data["tags"].(map[string]any)
	if fmt.Sprint(tags[a1]) != "[hashes/MD5/x]" || fmt.Sprint(tags[a2]) != "[hashes/MD5/y]" {
		t.Errorf("tags: %v", tags)
	}
	if ev := rec.wait(t); ev.Type != events.AssetTrashed || fmt.Sprint(ev.AssetIDs) != "["+a2+"]" {
		t.Errorf("trashed: %+v", ev)
	}
	rec.none(t)
}
