package steps

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"slices"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/immich"
	"github.com/serraniel/immich-flows/internal/queue"
)

func init() { register(hashStep()) }

var algorithms = map[string]func() hash.Hash{
	"md5":    md5.New,
	"sha1":   sha1.New,
	"sha256": sha256.New,
	"sha512": sha512.New,
}

const (
	sourceOriginal = "original" // bytes the client uploaded, hashed by the proxy
	sourceStored   = "stored"   // file as stored by Immich, e.g. after conversion
)

func hashStep() engine.StepType {
	return engine.StepType{
		Name:        "hash",
		Description: "File hashes (hex). original: as uploaded by the client (asset.uploaded only). stored: as stored in Immich.",
		Schema: schema(map[string]any{
			"source": map[string]any{"type": "string", "enum": []any{sourceOriginal, sourceStored}, "default": sourceStored},
			"algorithms": map[string]any{
				"type":    "array",
				"items":   map[string]any{"type": "string", "enum": []any{"md5", "sha1", "sha256", "sha512"}},
				"default": []any{"md5", "sha1"},
			},
		}),
		Validate: func(with map[string]any) error {
			if s := str(with, "source", sourceStored); s != sourceOriginal && s != sourceStored && !isTemplate(s) {
				return fmt.Errorf("source must be %s or %s", sourceOriginal, sourceStored)
			}
			_, err := hashAlgorithms(with)
			return err
		},
		Run: func(ctx context.Context, sc *engine.StepContext, with map[string]any) (map[string]any, error) {
			algs, err := hashAlgorithms(with)
			if err != nil {
				return nil, queue.Permanent(err)
			}
			switch src := str(with, "source", sourceStored); src {
			case sourceOriginal:
				return originalHashes(sc, algs)
			case sourceStored:
				return storedHashes(ctx, sc, algs)
			default:
				return nil, queue.Permanent(fmt.Errorf("unknown source %q", src))
			}
		},
	}
}

func hashAlgorithms(with map[string]any) ([]string, error) {
	algs, err := strs(with, "algorithms")
	if err != nil {
		return nil, err
	}
	if len(algs) == 0 {
		return []string{"md5", "sha1"}, nil
	}
	for _, a := range algs {
		if _, ok := algorithms[a]; !ok && !isTemplate(a) {
			return nil, fmt.Errorf("unknown algorithm %q (md5, sha1, sha256, sha512)", a)
		}
	}
	return algs, nil
}

func originalHashes(sc *engine.StepContext, algs []string) (map[string]any, error) {
	trigger, _ := sc.Env["trigger"].(map[string]any)
	data, _ := trigger["data"].(map[string]any)
	orig := toMap(data["original"])
	if orig == nil {
		return nil, queue.Permanent(errors.New("original hashes are only available for asset.uploaded through immich-flows"))
	}
	out := map[string]any{"source": sourceOriginal, "size": orig["size"]}
	for _, a := range algs {
		out[a] = orig[a]
	}
	return out, nil
}

// toMap returns v as a map. Events from the queue hold maps; events built in-process may hold structs.
func toMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok || v == nil {
		return m
	}
	var m map[string]any
	if b, err := json.Marshal(v); err == nil && json.Unmarshal(b, &m) == nil {
		return m
	}
	return nil
}

func storedHashes(ctx context.Context, sc *engine.StepContext, algs []string) (map[string]any, error) {
	asset, _ := sc.Env["asset"].(map[string]any)
	out := map[string]any{"source": sourceStored}

	// Immich's checksum is the SHA-1 of the stored file: no download needed.
	if checksum, ok := asset["checksum"].(string); ok && slices.Equal(algs, []string{"sha1"}) {
		raw, err := base64.StdEncoding.DecodeString(checksum)
		if err == nil && len(raw) == sha1.Size {
			out["sha1"] = hex.EncodeToString(raw)
			return out, nil
		}
	}

	ids, err := assetIDs(sc, map[string]any{})
	if err != nil {
		return nil, err
	}
	c, err := client(ctx, sc)
	if err != nil {
		return nil, err
	}
	edited := false
	res, err := c.DownloadAsset(ctx, ids[0], &immich.DownloadAssetParams{Edited: &edited})
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return nil, permanentIfClientError(&immich.Error{Status: res.StatusCode, Body: string(body)})
	}

	hs := make([]hash.Hash, len(algs))
	ws := make([]io.Writer, len(algs))
	for i, a := range algs {
		hs[i] = algorithms[a]()
		ws[i] = hs[i]
	}
	n, err := io.Copy(io.MultiWriter(ws...), res.Body)
	if err != nil {
		return nil, fmt.Errorf("download original: %w", err)
	}
	out["size"] = n
	for i, a := range algs {
		out[a] = hex.EncodeToString(hs[i].Sum(nil))
	}
	return out, nil
}
