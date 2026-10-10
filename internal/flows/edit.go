package flows

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/serraniel/immich-flows/internal/flow"
)

// ErrSource reports an invalid file name.
var ErrSource = errors.New("invalid source: use a relative .yaml or .yml path without hidden parts")

// MaxSize is the largest flow file accepted for writing.
const MaxSize = 1 << 20

// checkSource validates a file name relative to the directory, as listed in Entry.Source.
func checkSource(source string) error {
	ext := strings.ToLower(path.Ext(source))
	if source == "" || !filepath.IsLocal(source) || path.Clean(source) != source || (ext != ".yaml" && ext != ".yml") {
		return ErrSource
	}
	for p := range strings.SplitSeq(source, "/") {
		if strings.HasPrefix(p, ".") {
			return ErrSource
		}
	}
	return nil
}

// Read returns the content of a flow file.
func (d *Dir) Read(source string) ([]byte, error) {
	if err := checkSource(source); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(d.dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return root.ReadFile(source)
}

// Check parses and validates data as the content of source. IDs used by other files are
// rejected. Validation problems are returned as flow.Errors.
func (d *Dir) Check(source string, data []byte) (*flow.Flow, error) {
	if err := checkSource(source); err != nil {
		return nil, err
	}
	f, err := flow.Parse(data)
	if err != nil {
		return nil, err
	}
	if d.v != nil {
		if err := d.v.Validate(f); err != nil {
			return f, err
		}
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, e := range d.entries {
		if e.Source != source && e.Flow != nil && e.Flow.ID == f.ID {
			return f, flow.Errors{{Line: 1, Col: 1, Msg: fmt.Sprintf("id %q already defined in %s", f.ID, e.Source)}}
		}
	}
	return f, nil
}

// Write validates data and writes it to source atomically, then reloads.
// Invalid content is not written.
func (d *Dir) Write(source string, data []byte) error {
	d.wmu.Lock()
	defer d.wmu.Unlock()
	if _, err := d.Check(source, data); err != nil {
		return err
	}
	if err := os.MkdirAll(d.dir, 0o750); err != nil {
		return err
	}
	root, err := os.OpenRoot(d.dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if dir := path.Dir(source); dir != "." {
		if err := root.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}
	tmp := path.Join(path.Dir(source), ".tmp-"+path.Base(source))
	if err := root.WriteFile(tmp, data, 0o640); err != nil {
		return err
	}
	if err := root.Rename(tmp, source); err != nil {
		_ = root.Remove(tmp)
		return err
	}
	return d.Load()
}

// SetEnabled rewrites the enabled field of source.
func (d *Dir) SetEnabled(source string, enabled bool) error {
	data, err := d.Read(source)
	if err != nil {
		return err
	}
	out, err := flow.SetEnabled(data, enabled)
	if err != nil {
		return err
	}
	return d.Write(source, out)
}

// Delete removes source, then reloads.
func (d *Dir) Delete(source string) error {
	if err := checkSource(source); err != nil {
		return err
	}
	d.wmu.Lock()
	defer d.wmu.Unlock()
	root, err := os.OpenRoot(d.dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if err := root.Remove(source); err != nil {
		return err
	}
	return d.Load()
}
