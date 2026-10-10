package hooks

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"sync"
	"time"

	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/proxy"
)

// Upload form field names used by Immich.
const (
	fieldAsset    = "assetData"
	fieldFilename = "filename"
)

// Original describes the uploaded file as sent by the client, before any downstream conversion.
type Original struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	MD5      string `json:"md5"`
	SHA1     string `json:"sha1"`
	SHA256   string `json:"sha256"`
	SHA512   string `json:"sha512"`
}

type uploadKey struct{}

type uploadResult struct {
	done chan struct{}
	orig Original
	err  error
}

// Upload hashes the original file of POST /api/assets while it streams to the upstream and
// publishes asset.uploaded once Immich returns the asset ID. If hashing fails, the event is
// published without the original hashes.
func Upload(d Deps) proxy.Hook {
	return proxy.Hook{
		Route: "POST /api/assets",
		Pre: func(_ context.Context, ex *proxy.Exchange) error {
			mt, params, err := mime.ParseMediaType(ex.Request.Header.Get("Content-Type"))
			if err != nil || mt != "multipart/form-data" || params["boundary"] == "" {
				return nil
			}
			pr, pw := io.Pipe()
			res := &uploadResult{done: make(chan struct{})}
			go func() {
				defer close(res.done)
				res.orig, res.err = hashMultipart(pr, params["boundary"])
				// Keep the pipe drained so the proxy never blocks on a parse error.
				_, _ = io.Copy(io.Discard, pr)
			}()
			ex.Request.Body = &teeBody{src: ex.Request.Body, pw: pw}
			ex.Set(uploadKey{}, res)
			return nil
		},
		Post: func(ex *proxy.Exchange) {
			if ex.Status != http.StatusCreated && ex.Status != http.StatusOK {
				return
			}
			var body struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal(ex.Response, &body); err != nil || body.ID == "" {
				d.Log.Warn("upload response without asset id", "status", ex.Status)
				return
			}
			// Flows run even if the original could not be hashed; they just lack trigger.data.original.
			data := map[string]any{"status": body.Status}
			res, _ := ex.Get(uploadKey{}).(*uploadResult)
			if res == nil {
				d.Log.Warn("upload not hashed: not a multipart request", "asset", body.ID)
			} else {
				select {
				case <-res.done:
					if res.err != nil {
						d.Log.Warn("upload not hashed", "asset", body.ID, "err", res.err)
					} else {
						data["original"] = res.orig
					}
				case <-time.After(30 * time.Second):
					d.Log.Error("upload hashing did not finish", "asset", body.ID)
				}
			}
			d.publish(ex.Request, events.Event{
				Type:     events.AssetUploaded,
				Time:     time.Now().UTC(),
				AssetIDs: []string{body.ID},
				Data:     data,
			})
		},
	}
}

// teeBody copies everything read from src into pw and closes pw at EOF or Close.
// The transport may call Close concurrently with Read.
type teeBody struct {
	src io.ReadCloser
	mu  sync.Mutex
	pw  *io.PipeWriter
}

func (t *teeBody) Read(p []byte) (int, error) {
	n, err := t.src.Read(p)
	t.mu.Lock()
	defer t.mu.Unlock()
	if n > 0 && t.pw != nil {
		if _, werr := t.pw.Write(p[:n]); werr != nil {
			// Parser gone: stop teeing, keep proxying.
			t.pw = nil
		}
	}
	if err != nil && t.pw != nil {
		if errors.Is(err, io.EOF) {
			_ = t.pw.Close()
		} else {
			_ = t.pw.CloseWithError(err)
		}
		t.pw = nil
	}
	return n, err
}

func (t *teeBody) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pw != nil {
		_ = t.pw.CloseWithError(io.ErrUnexpectedEOF)
		t.pw = nil
	}
	return t.src.Close()
}

func hashMultipart(r io.Reader, boundary string) (Original, error) {
	var o Original
	found := false
	mr := multipart.NewReader(r, boundary)
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return o, err
		}
		switch part.FormName() {
		case fieldAsset:
			hs := map[string]hash.Hash{"md5": md5.New(), "sha1": sha1.New(), "sha256": sha256.New(), "sha512": sha512.New()}
			w := io.MultiWriter(hs["md5"], hs["sha1"], hs["sha256"], hs["sha512"])
			n, err := io.Copy(w, part)
			if err != nil {
				return o, err
			}
			found = true
			o.Size = n
			if o.Filename == "" {
				o.Filename = part.FileName()
			}
			o.MD5 = hex.EncodeToString(hs["md5"].Sum(nil))
			o.SHA1 = hex.EncodeToString(hs["sha1"].Sum(nil))
			o.SHA256 = hex.EncodeToString(hs["sha256"].Sum(nil))
			o.SHA512 = hex.EncodeToString(hs["sha512"].Sum(nil))
		case fieldFilename:
			b, err := io.ReadAll(io.LimitReader(part, 4096))
			if err != nil {
				return o, err
			}
			o.Filename = string(b)
		}
		_ = part.Close()
	}
	if !found {
		return o, errors.New("no assetData part")
	}
	return o, nil
}
