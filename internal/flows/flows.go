// Package flows loads flow files from a directory and keeps them current.
package flows

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/serraniel/immich-flows/internal/flow"
)

// Validator checks a parsed flow against the engine (known steps, expressions).
type Validator interface {
	Validate(f *flow.Flow) error
}

// Entry is one flow file and its state.
type Entry struct {
	Source string     `json:"source"` // file name relative to the directory
	Flow   *flow.Flow `json:"flow,omitempty"`
	Error  string     `json:"error,omitempty"` // set if the file is invalid; the flow is not run
}

// Dir is a FlowSource backed by *.yaml / *.yml files in a directory.
type Dir struct {
	dir string
	v   Validator
	log *slog.Logger

	wmu     sync.Mutex // serializes writes
	mu      sync.RWMutex
	entries []Entry
	byID    map[string]*flow.Flow
	sum     [32]byte
}

// NewDir returns a Dir. Call Load once, then Watch.
func NewDir(dir string, v Validator, log *slog.Logger) *Dir {
	return &Dir{dir: dir, v: v, log: log, byID: map[string]*flow.Flow{}}
}

// Flows returns all valid flows.
func (d *Dir) Flows() []*flow.Flow {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]*flow.Flow, 0, len(d.byID))
	for _, e := range d.entries {
		if e.Flow != nil && e.Error == "" {
			out = append(out, e.Flow)
		}
	}
	return out
}

// Flow returns a valid flow by ID.
func (d *Dir) Flow(id string) (*flow.Flow, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	f, ok := d.byID[id]
	return f, ok
}

// Entries returns all files including invalid ones, sorted by source.
func (d *Dir) Entries() []Entry {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]Entry(nil), d.entries...)
}

type file struct {
	name string
	data []byte
}

// Load reads the directory. A missing directory means no flows.
func (d *Dir) Load() error {
	files, sum, err := d.read()
	if err != nil {
		return err
	}
	d.mu.RLock()
	same := sum == d.sum && d.entries != nil
	d.mu.RUnlock()
	if same {
		return nil
	}

	var entries []Entry
	byID := map[string]*flow.Flow{}
	sources := map[string]string{}
	for _, f := range files {
		e := Entry{Source: f.name}
		fl, err := flow.Parse(f.data)
		if err == nil && d.v != nil {
			if verr := d.v.Validate(fl); verr != nil {
				e.Flow, err = fl, verr
			}
		}
		switch {
		case err != nil:
			e.Error = err.Error()
		case sources[fl.ID] != "":
			e.Flow, e.Error = fl, fmt.Sprintf("id %q already defined in %s", fl.ID, sources[fl.ID])
		default:
			e.Flow = fl
			byID[fl.ID] = fl
			sources[fl.ID] = f.name
		}
		if e.Error != "" {
			d.log.Warn("invalid flow file, not running it", "file", f.name, "err", e.Error)
		}
		entries = append(entries, e)
	}

	d.mu.Lock()
	d.entries, d.byID, d.sum = entries, byID, sum
	d.mu.Unlock()
	d.log.Info("flows loaded", "dir", d.dir, "valid", len(byID), "invalid", len(entries)-len(byID))
	return nil
}

func (d *Dir) read() ([]file, [32]byte, error) {
	root, err := os.OpenRoot(d.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, sha256.Sum256(nil), nil
	}
	if err != nil {
		return nil, [32]byte{}, err
	}
	defer func() { _ = root.Close() }()
	fsys := root.FS() // confined to the directory, symlinks cannot escape it

	var files []file
	err = fs.WalkDir(fsys, ".", func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if de.IsDir() {
			if path != "." && strings.HasPrefix(de.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(de.Name()))
		if (ext != ".yaml" && ext != ".yml") || strings.HasPrefix(de.Name(), ".") {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		files = append(files, file{name: path, data: data})
		return nil
	})
	if err != nil {
		return nil, [32]byte{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	h := sha256.New()
	for _, f := range files {
		h.Write([]byte(f.name + "\x00"))
		h.Write(f.data)
		h.Write([]byte{0})
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return files, sum, nil
}

// Watch reloads the directory every interval until ctx is cancelled. Polling works on bind
// mounts and network filesystems where change notifications are unreliable.
func (d *Dir) Watch(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := d.Load(); err != nil {
				d.log.Error("reload flows", "dir", d.dir, "err", err)
			}
		}
	}
}
