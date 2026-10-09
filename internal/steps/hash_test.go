package steps

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/flow"
	"github.com/serraniel/immich-flows/internal/queue"
)

func hexOf(sum []byte) string { return hex.EncodeToString(sum) }

func TestHashOriginalAndStored(t *testing.T) {
	deps, f := fakeDeps(t)
	stored := []byte("converted jxl bytes")
	f.Originals[runAsset] = stored
	md, sh1, sh256 := md5.Sum(stored), sha1.Sum(stored), sha256.Sum256(stored)

	src := `id: f
trigger: {on: asset.uploaded}
steps:
  - {id: orig, use: hash, with: {source: original, algorithms: [md5, sha1]}}
  - {id: stored, use: hash, with: {source: stored, algorithms: [md5, sha1, sha256]}}
  - use: tag.assign
    with:
      tags:
        - 'hashes/MD5/ORIGINAL/{{ steps.orig.md5 }}'
        - 'hashes/MD5/STORED/{{ steps.stored.md5 }}'
`
	data := map[string]any{"original": map[string]any{"md5": "aaa", "sha1": "bbb", "size": 3}}
	r := runFlowEvent(t, src, map[string]any{"id": runAsset}, data, deps)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if o := r.outputs["orig"]; o["md5"] != "aaa" || o["sha1"] != "bbb" || o["sha256"] != nil {
		t.Errorf("orig: %v", o)
	}
	if s := r.outputs["stored"]; s["md5"] != hexOf(md[:]) || s["sha1"] != hexOf(sh1[:]) || s["sha256"] != hexOf(sh256[:]) || s["size"] != float64(len(stored)) {
		t.Errorf("stored: %v", s)
	}
	assets, _ := f.State()
	if assets != runAsset+": hashes/MD5/ORIGINAL/aaa,hashes/MD5/STORED/"+hexOf(md[:]) {
		t.Errorf("tags: %s", assets)
	}
}

func TestHashStoredSHA1FromChecksum(t *testing.T) {
	deps, f := fakeDeps(t)
	sum := sha1.Sum([]byte("x"))
	asset := map[string]any{"id": runAsset, "checksum": base64.StdEncoding.EncodeToString(sum[:])}
	r := runFlow(t, "id: f\ntrigger: {on: manual}\nsteps: [{id: h, use: hash, with: {algorithms: [sha1]}}]\n", asset, deps)
	if r.err != nil || r.outputs["h"]["sha1"] != hexOf(sum[:]) {
		t.Fatalf("%v %v", r.err, r.outputs)
	}
	if n := f.Requests["GET /assets/"+runAsset+"/original"]; n != 0 {
		t.Errorf("downloaded %d times, checksum should suffice", n)
	}
}

func TestHashErrors(t *testing.T) {
	deps, _ := fakeDeps(t)
	r := runFlow(t, "id: f\ntrigger: {on: manual}\nsteps: [{use: hash, with: {source: original}}]\n", nil, deps)
	if !queue.IsPermanent(r.err) || !strings.Contains(r.err.Error(), "only available for asset.uploaded") {
		t.Errorf("original without upload data: %v", r.err)
	}
	r = runFlow(t, "id: f\ntrigger: {on: manual}\nsteps: [{use: hash}]\n", map[string]any{"id": runAsset}, deps)
	if !queue.IsPermanent(r.err) || !strings.Contains(r.err.Error(), "404") {
		t.Errorf("missing original: %v", r.err)
	}

	reg := engine.NewRegistry()
	Register(reg)
	for with, want := range map[string]string{"{algorithms: [crc32]}": "unknown algorithm", "{source: disk}": "source must be"} {
		fl, _ := flow.Parse([]byte(fmt.Sprintf("id: f\ntrigger: {on: manual}\nsteps: [{use: hash, with: %s}]\n", with)))
		if err := reg.Validate(fl); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", with, err)
		}
	}
}
