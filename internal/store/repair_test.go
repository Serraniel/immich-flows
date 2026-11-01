package store

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testRows = 5000

// testDB creates a migrated database with table t (id, v), index t_v and a child table c
// referencing t. It returns the path and the page numbers of t's and t_v's leaf pages.
func testDB(t *testing.T) (path string, tableLeaves, indexLeaves []int64, pageSize int64) {
	t.Helper()
	ctx := context.Background()
	path = filepath.Join(t.TempDir(), "immich-flows.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT NOT NULL)`,
		`CREATE INDEX t_v ON t (v)`,
		`CREATE TABLE c (t_id INTEGER NOT NULL REFERENCES t (id), n INTEGER)`,
		`WITH RECURSIVE s(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM s WHERE i < 5000)
		 INSERT INTO t SELECT i, printf('value-%06d-%s', i, hex(randomblob(20))) FROM s`,
		`INSERT INTO c SELECT id, id FROM t`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	leaves := func(name string) []int64 {
		rows, err := db.QueryContext(ctx, `SELECT pageno FROM dbstat WHERE name = ? AND pagetype = 'leaf' ORDER BY pageno`, name)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		var out []int64
		for rows.Next() {
			var p int64
			_ = rows.Scan(&p)
			out = append(out, p)
		}
		return out
	}
	tableLeaves, indexLeaves = leaves("t"), leaves("t_v")
	if err := db.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&pageSize); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path, tableLeaves, indexLeaves, pageSize
}

// corrupt overwrites the cell pointers of page pageno.
func corrupt(t *testing.T, path string, pageno, pageSize int64) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	junk := []byte(strings.Repeat("\xff\x07", 64))
	if _, err := f.WriteAt(junk, (pageno-1)*pageSize+8); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, path, q string) int {
	t.Helper()
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow(q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func backups(t *testing.T, path string) []os.DirEntry {
	t.Helper()
	e, _ := os.ReadDir(filepath.Join(filepath.Dir(path), "backups"))
	return e
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestCheckHealthy(t *testing.T) {
	path, _, _, _ := testDB(t)
	if err := Check(context.Background(), path, quiet()); err != nil {
		t.Fatal(err)
	}
	if len(backups(t, path)) != 0 {
		t.Error("backup made for a healthy database")
	}
	if err := Check(context.Background(), filepath.Join(t.TempDir(), "missing.db"), quiet()); err != nil {
		t.Errorf("missing database: %v", err)
	}
}

func TestCheckRebuildsCorruptTable(t *testing.T) {
	ctx := context.Background()
	path, leaves, _, ps := testDB(t)
	corrupt(t, path, leaves[len(leaves)/2], ps)

	// Corruption seen at runtime.
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatal(err)
	}
	var sum int
	err = db.QueryRow(`SELECT sum(length(v)) FROM t`).Scan(&sum)
	_ = db.Close()
	if !IsCorrupt(err) {
		t.Fatalf("IsCorrupt(%v) = false", err)
	}
	m := NewMonitor(path, quiet())
	m.Observe(err)
	m.Observe(err)
	select {
	case <-m.Corrupt():
	default:
		t.Fatal("monitor did not report corruption")
	}
	if !exists(path + corruptSuffix) {
		t.Fatal("no marker")
	}

	if err := Check(ctx, path, quiet()); err != nil {
		t.Fatal(err)
	}
	if p, err := check(ctx, path, true); err != nil || len(p) > 0 {
		t.Fatalf("after repair: %v %v", p, err)
	}
	n := count(t, path, `SELECT count(*) FROM t`)
	if n >= testRows || n < testRows*9/10 {
		t.Errorf("rows in t: %d", n)
	}
	if c := count(t, path, `SELECT count(*) FROM c`); c != n {
		t.Errorf("rows in c: %d, want %d (orphans removed)", c, n)
	}
	if c := count(t, path, `SELECT count(*) FROM schema_migrations`); c == 0 {
		t.Error("schema_migrations lost")
	}
	if exists(path+corruptSuffix) || exists(path+repairFailedSuffix) {
		t.Error("marker left")
	}
	b := backups(t, path)
	if len(b) != 1 {
		t.Fatalf("backups: %d", len(b))
	}
	if p, err := check(ctx, filepath.Join(filepath.Dir(path), "backups", b[0].Name(), filepath.Base(path)), false); err != nil || len(p) == 0 {
		t.Errorf("backup is not the corrupt original: %v %v", p, err)
	}
}

func TestCheckReindexesCorruptIndex(t *testing.T) {
	ctx := context.Background()
	path, _, leaves, ps := testDB(t)
	corrupt(t, path, leaves[len(leaves)/2], ps)
	if p, _ := check(ctx, path, false); len(p) == 0 {
		t.Fatal("corruption not detected")
	}
	if err := Check(ctx, path, quiet()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, path, `SELECT count(*) FROM t`); n != testRows {
		t.Errorf("rows: %d, want %d", n, testRows)
	}
	if n := count(t, path, `SELECT count(*) FROM t INDEXED BY t_v WHERE v > ''`); n != testRows {
		t.Errorf("rows via index: %d, want %d", n, testRows)
	}
}

func TestCheckDoesNotRetryFailedRepair(t *testing.T) {
	path, leaves, _, ps := testDB(t)
	corrupt(t, path, leaves[1], ps)
	if err := os.WriteFile(path+repairFailedSuffix, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Check(context.Background(), path, quiet()); err == nil {
		t.Fatal("expected error")
	}
	if len(backups(t, path)) != 0 {
		t.Error("backup made although repair is disabled by marker")
	}
}

func TestBackupKeepsNewest(t *testing.T) {
	path, _, _, _ := testDB(t)
	root := filepath.Join(filepath.Dir(path), "backups")
	for _, n := range []string{"20200101T000000.000000000Z", "20200102T000000.000000000Z", "20200103T000000.000000000Z"} {
		if err := os.MkdirAll(filepath.Join(root, n), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	dir, err := backup(path)
	if err != nil {
		t.Fatal(err)
	}
	b := backups(t, path)
	if len(b) != keepBackups || b[0].Name() != "20200102T000000.000000000Z" || filepath.Join(root, b[len(b)-1].Name()) != dir {
		t.Errorf("backups: %v", b)
	}
}
