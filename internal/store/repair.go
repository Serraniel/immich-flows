package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"modernc.org/sqlite"
)

// Marker files next to the database.
const (
	corruptSuffix      = ".corrupt"       // corruption seen at runtime: run a full check at next start
	repairFailedSuffix = ".repair-failed" // automatic repair failed: do not retry
	keepBackups        = 3
)

// IsCorrupt reports whether err is SQLite database corruption.
func IsCorrupt(err error) bool {
	var e *sqlite.Error
	if !errors.As(err, &e) {
		return false
	}
	switch e.Code() & 0xff {
	case 11, 26: // SQLITE_CORRUPT, SQLITE_NOTADB
		return true
	}
	return false
}

// Monitor watches database errors at runtime. On corruption it writes a marker so that the next
// start runs a full check, and closes Corrupt.
type Monitor struct {
	path    string
	log     *slog.Logger
	once    sync.Once
	corrupt chan struct{}
}

// NewMonitor returns a Monitor for the database at path.
func NewMonitor(path string, log *slog.Logger) *Monitor {
	return &Monitor{path: path, log: log, corrupt: make(chan struct{})}
}

// Observe inspects err. Safe for concurrent use and nil errors.
func (m *Monitor) Observe(err error) {
	if !IsCorrupt(err) {
		return
	}
	m.once.Do(func() {
		m.log.Error("database corruption detected; restart to repair", "err", err)
		if werr := os.WriteFile(m.path+corruptSuffix, []byte(err.Error()+"\n"), 0o600); werr != nil {
			m.log.Error("write corruption marker", "err", werr)
		}
		close(m.corrupt)
	})
}

// Corrupt is closed once corruption was observed.
func (m *Monitor) Corrupt() <-chan struct{} { return m.corrupt }

// Check verifies the database at path before it is opened and repairs it if needed.
// Before any change, the database files are copied to backups/<timestamp>/ next to it.
// A failed repair is not retried until the marker file <path>.repair-failed is removed.
func Check(ctx context.Context, path string, log *slog.Logger) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	full := exists(path + corruptSuffix)
	problems, err := check(ctx, path, full)
	if err != nil {
		return fmt.Errorf("check database: %w", err)
	}
	if len(problems) == 0 {
		return clearMarkers(path)
	}
	log.Error("database is corrupt", "problems", problems)

	if exists(path + repairFailedSuffix) {
		return fmt.Errorf("database is corrupt and automatic repair failed before; see %s and docs/install.md#database-corruption", path+repairFailedSuffix)
	}
	err = repair(ctx, path, log)
	if err == nil {
		return clearMarkers(path)
	}
	if ctx.Err() == nil {
		_ = os.WriteFile(path+repairFailedSuffix, []byte(err.Error()+"\n"), 0o600)
	}
	return fmt.Errorf("repair database: %w", err)
}

func repair(ctx context.Context, path string, log *slog.Logger) (err error) {
	dir, err := backup(path)
	if err != nil {
		return fmt.Errorf("backup (nothing changed): %w", err)
	}
	log.Info("database backed up", "dir", dir)
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w (backup: %s)", err, dir)
		}
	}()

	err = reindex(ctx, path)
	if err == nil {
		log.Info("database repaired by rebuilding indexes")
		return nil
	}
	log.Warn("rebuilding indexes did not help, rebuilding database", "err", err)

	tmp := path + ".repair"
	removeDB(tmp)
	stats, err := rebuild(ctx, path, tmp)
	if err != nil {
		removeDB(tmp)
		return err
	}
	p, err := check(ctx, tmp, true)
	if err == nil && len(p) > 0 {
		err = fmt.Errorf("still corrupt: %v", p)
	}
	if err != nil {
		removeDB(tmp)
		return fmt.Errorf("check rebuilt database: %w", err)
	}
	// The old WAL must not be applied to the new file. Both are in the backup.
	for _, s := range []string{"-wal", "-shm"} {
		if err := os.Remove(path + s); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	for _, s := range stats {
		lvl := slog.LevelInfo
		if s.failed > 0 {
			lvl = slog.LevelWarn
		}
		log.Log(ctx, lvl, "database table rebuilt", "table", s.table, "rows", s.rows, "unreadable_ranges", s.failed, "orphans_removed", s.orphans)
	}
	log.Info("database repaired", "backup", dir)
	return nil
}

func dsn(path string) string {
	return "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(30000)"
}

// check runs quick_check (or integrity_check if full) and returns the problems found.
// A file SQLite cannot read at all counts as one problem.
func check(ctx context.Context, path string, full bool) ([]string, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	pragma := "PRAGMA quick_check(20)"
	if full {
		pragma = "PRAGMA integrity_check(20)"
	}
	rows, err := db.QueryContext(ctx, pragma)
	if IsCorrupt(err) {
		return []string{err.Error()}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		if s != "ok" {
			out = append(out, s)
		}
	}
	if err := rows.Err(); IsCorrupt(err) {
		return append(out, err.Error()), nil
	} else if err != nil {
		return nil, err
	}
	return out, nil
}

func reindex(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "REINDEX"); err != nil {
		_ = db.Close()
		return err
	}
	if err := db.Close(); err != nil {
		return err
	}
	p, err := check(ctx, path, true)
	if err != nil {
		return err
	}
	if len(p) > 0 {
		return fmt.Errorf("still corrupt: %v", p)
	}
	return nil
}

type tableStats struct {
	table   string
	rows    int
	failed  int
	orphans int
}

// rebuild creates dst with the schema of src and copies every readable row.
func rebuild(ctx context.Context, src, dst string) ([]tableStats, error) {
	db, err := sql.Open("sqlite", dsn(dst))
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	conn, err := db.Conn(ctx) // ATTACH and foreign_keys are per connection
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	// dst is a scratch file until it passed integrity_check: no journal, no fsync per row.
	for _, q := range []string{"PRAGMA foreign_keys = OFF", "PRAGMA journal_mode = OFF", "PRAGMA synchronous = OFF"} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			return nil, err
		}
	}
	if _, err := conn.ExecContext(ctx, "ATTACH DATABASE ? AS old", src); err != nil {
		return nil, err
	}

	type obj struct{ typ, name, sql string }
	var schema []obj
	rows, err := conn.QueryContext(ctx, `SELECT type, name, sql FROM old.sqlite_schema
		WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%'
		ORDER BY CASE type WHEN 'table' THEN 0 ELSE 1 END`)
	if err != nil {
		return nil, fmt.Errorf("read schema: %w", err)
	}
	for rows.Next() {
		var o obj
		if err := rows.Scan(&o.typ, &o.name, &o.sql); err != nil {
			_ = rows.Close()
			return nil, err
		}
		schema = append(schema, o)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read schema: %w", err)
	}
	if len(schema) == 0 {
		return nil, errors.New("no schema found")
	}

	create := func(later bool) error {
		for _, o := range schema {
			if (o.typ == "trigger" || o.typ == "view") != later {
				continue
			}
			if _, err := conn.ExecContext(ctx, o.sql); err != nil {
				return fmt.Errorf("create %s %s: %w", o.typ, o.name, err)
			}
		}
		return nil
	}
	// Indexes exist before the copy so that INSERT OR IGNORE drops duplicates from corrupt pages;
	// triggers are created after it so they do not fire.
	if err := create(false); err != nil {
		return nil, err
	}
	var stats []tableStats
	for _, o := range schema {
		if o.typ != "table" {
			continue
		}
		s, err := copyTable(ctx, conn, o.name)
		if err != nil {
			return nil, fmt.Errorf("copy %s: %w", o.name, err)
		}
		stats = append(stats, s)
	}
	if err := create(true); err != nil {
		return nil, err
	}
	if err := removeOrphans(ctx, conn, stats); err != nil {
		return nil, err
	}
	for _, q := range []string{"DETACH DATABASE old", "PRAGMA journal_mode = WAL"} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			return nil, err
		}
	}
	return stats, nil
}

const copyBatch = 500

// copyTable copies the rows of old.<table> into main.<table> in rowid order. Unreadable parts are
// skipped: after an error the scan resumes at doubling distances from the last good rowid.
func copyTable(ctx context.Context, conn *sql.Conn, table string) (tableStats, error) {
	st := tableStats{table: table}
	cols, err := columns(ctx, conn, table)
	if err != nil {
		return st, err
	}
	qt, list := quote(table), strings.Join(cols, ", ")
	insert := fmt.Sprintf("INSERT OR IGNORE INTO main.%s (rowid, %s) VALUES (?%s)", qt, list, strings.Repeat(", ?", len(cols)))
	sel := fmt.Sprintf("SELECT rowid, %s FROM old.%s WHERE rowid > ? ORDER BY rowid LIMIT %d", list, qt, copyBatch)

	const maxRowid = 1<<63 - 1
	cursor := int64(-1 << 63)
	var skip int64 // distance of the next jump; 0 while reading succeeds
	for {
		if err := ctx.Err(); err != nil {
			return st, err
		}
		b, err := copyBatchRows(ctx, conn, sel, insert, cursor, len(cols))
		st.rows += b.copied
		cursor = max(cursor, b.last)
		switch {
		case err != nil && !IsCorrupt(err):
			return st, err
		case err == nil && b.read == 0:
			return st, nil // end of table
		case err == nil && b.copied > 0:
			skip = 0
			continue
		}
		// Corrupt page, or only out-of-order rows: jump ahead.
		if skip == 0 {
			st.failed++
			skip = 1
		} else {
			skip *= 2
		}
		if skip > 1<<62 || cursor > maxRowid-skip {
			return st, nil
		}
		cursor += skip
	}
}

type batch struct {
	read, copied int
	last         int64 // last rowid copied
}

// copyBatchRows copies up to copyBatch rows after cursor. Rows are read first and then written in
// one transaction: a corruption error during a write transaction would roll it back.
func copyBatchRows(ctx context.Context, conn *sql.Conn, sel, insert string, cursor int64, ncols int) (batch, error) {
	b := batch{last: cursor}
	var vals [][]any
	readErr := func() error {
		rows, err := conn.QueryContext(ctx, sel, cursor)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			b.read++
			v := make([]any, ncols+1)
			ptrs := make([]any, ncols+1)
			for i := range v {
				ptrs[i] = &v[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				return err
			}
			if id, ok := v[0].(int64); ok && id > b.last {
				vals = append(vals, v)
				b.last = id
			} // else out of order: corrupt page
		}
		return rows.Err()
	}()
	if len(vals) == 0 {
		b.last = cursor
		return b, readErr
	}
	if _, err := conn.ExecContext(ctx, "BEGIN"); err != nil {
		return b, err
	}
	for _, v := range vals {
		if _, err := conn.ExecContext(ctx, insert, v...); err != nil {
			_, _ = conn.ExecContext(ctx, "ROLLBACK")
			return batch{last: cursor}, err
		}
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return batch{last: cursor}, err
	}
	b.copied = len(vals)
	return b, readErr
}

func columns(ctx context.Context, conn *sql.Conn, table string) ([]string, error) {
	rows, err := conn.QueryContext(ctx, "SELECT name FROM pragma_table_info(?, 'main')", table)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cols = append(cols, quote(c))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, errors.New("no columns")
	}
	return cols, nil
}

// removeOrphans deletes rows whose foreign key parent was lost.
func removeOrphans(ctx context.Context, conn *sql.Conn, stats []tableStats) error {
	for {
		rows, err := conn.QueryContext(ctx, "PRAGMA main.foreign_key_check")
		if err != nil {
			return err
		}
		type orphan struct {
			table string
			rowid int64
		}
		var found []orphan
		for rows.Next() {
			var (
				o      orphan
				rowid  sql.NullInt64
				parent string
				fkid   int
			)
			if err := rows.Scan(&o.table, &rowid, &parent, &fkid); err != nil {
				_ = rows.Close()
				return err
			}
			if rowid.Valid {
				o.rowid = rowid.Int64
				found = append(found, o)
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(found) == 0 {
			return nil
		}
		for _, o := range found {
			if _, err := conn.ExecContext(ctx, "DELETE FROM main."+quote(o.table)+" WHERE rowid = ?", o.rowid); err != nil { // #nosec G202 -- quoted name from sqlite_schema
				return err
			}
			if i := slices.IndexFunc(stats, func(s tableStats) bool { return s.table == o.table }); i >= 0 {
				stats[i].orphans++
				stats[i].rows--
			}
		}
	}
}

// backup copies the database files to backups/<timestamp>/ and keeps the newest keepBackups.
func backup(path string) (string, error) {
	root := filepath.Join(filepath.Dir(path), "backups")
	dir := filepath.Join(root, time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	for _, s := range []string{"", "-wal", "-shm"} {
		err := copyFile(path+s, filepath.Join(dir, filepath.Base(path)+s))
		if errors.Is(err, os.ErrNotExist) && s != "" {
			continue
		}
		if err != nil {
			_ = os.RemoveAll(dir)
			return "", err
		}
	}
	entries, err := os.ReadDir(root)
	if err == nil && len(entries) > keepBackups {
		for _, e := range entries[:len(entries)-keepBackups] { // sorted by name = time
			_ = os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
	return dir, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) // #nosec G304 -- database files in the data directory
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func clearMarkers(path string) error {
	for _, s := range []string{corruptSuffix, repairFailedSuffix} {
		if err := os.Remove(path + s); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func removeDB(path string) {
	for _, s := range []string{"", "-wal", "-shm", "-journal"} {
		_ = os.Remove(path + s)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
