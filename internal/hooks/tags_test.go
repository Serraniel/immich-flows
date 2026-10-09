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

func TestTagEvents(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/tags/assets" {
			_, _ = w.Write([]byte(`{"count":2}`))
			return
		}
		_, _ = fmt.Fprintf(w, `[{"id":%q,"success":true},{"id":%q,"success":false,"error":"duplicate"}]`, a1, a2)
	}))
	defer up.Close()
	upURL, _ := url.Parse(up.URL)
	f := immichtest.New()
	f.Tag(a1, "people/anna")
	f.Tag(a1, "places/home")
	anna, home := f.ByValue("people/anna").ID, f.ByValue("places/home").ID
	fsrv := httptest.NewServer(f)
	defer fsrv.Close()
	fURL, _ := url.Parse(fsrv.URL)

	rec := newRecorder()
	d := Deps{Users: fakeUsers{}, Publish: rec.publish, Log: slog.New(slog.DiscardHandler), Immich: fURL,
		Wants: func(string) bool { return true }}
	srv := httptest.NewServer(proxy.New(upURL, slog.New(slog.DiscardHandler), Tags(d)...))
	defer srv.Close()

	ids := fmt.Sprintf(`{"ids":[%q,%q]}`, a1, a2)
	do(t, http.MethodPut, srv.URL+"/api/tags/"+anna+"/assets", ids)
	ev := rec.wait(t)
	tag := ev.Data["tag"].(map[string]any)
	if ev.Type != events.TagAssigned || fmt.Sprint(ev.AssetIDs) != "["+a1+"]" || tag["value"] != "people/anna" || tag["name"] != "anna" {
		t.Errorf("assign: %+v", ev)
	}

	do(t, http.MethodDelete, srv.URL+"/api/tags/"+anna+"/assets", ids)
	if ev := rec.wait(t); ev.Type != events.TagRemoved {
		t.Errorf("remove: %+v", ev)
	}

	do(t, http.MethodPut, srv.URL+"/api/tags/assets", fmt.Sprintf(`{"tagIds":[%q,%q],"assetIds":[%q,%q]}`, anna, home, a1, a2))
	first, second := rec.wait(t), rec.wait(t)
	if first.Data["tag"].(map[string]any)["value"] != "people/anna" || second.Data["tag"].(map[string]any)["value"] != "places/home" ||
		len(second.AssetIDs) != 2 {
		t.Errorf("bulk: %+v %+v", first, second)
	}
	rec.none(t)
}
