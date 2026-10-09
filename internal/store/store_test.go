package store

import (
	"context"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestOpenMigratesOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db.sqlite")

	for range 2 {
		db, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("migrations recorded: %d", n)
		}
		var mode string
		_ = db.QueryRow(`PRAGMA journal_mode`).Scan(&mode)
		if mode != "wal" {
			t.Errorf("journal_mode %q", mode)
		}
		_ = db.Close()
	}
}

func TestMigrateOrderAndRollback(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	fsys := fstest.MapFS{
		"migrations/2_b.sql": {Data: []byte(`INSERT INTO t VALUES (2);`)},
		"migrations/1_a.sql": {Data: []byte(`CREATE TABLE t (v INTEGER);`)},
		"migrations/3_c.sql": {Data: []byte(`INSERT INTO t VALUES (3); BROKEN;`)},
	}
	if err := migrate(ctx, db, fsys); err == nil {
		t.Fatal("expected error from broken migration")
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM t`).Scan(&n)
	if n != 1 {
		t.Errorf("rows after failed migration: %d, want 1", n)
	}
	_ = db.QueryRow(`SELECT count(*) FROM schema_migrations WHERE name = '3_c'`).Scan(&n)
	if n != 0 {
		t.Error("failed migration recorded")
	}
}
