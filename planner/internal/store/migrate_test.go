package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// TestSchemaSnapshot 은 마이그레이션이 만드는 스키마를 고정한다.
// 바꿨으면: UPDATE_SNAPSHOT=1 go test ./internal/store -run TestSchemaSnapshot
func TestSchemaSnapshot(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	got, err := st.SchemaDump(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "schema.sql")
	if os.Getenv("UPDATE_SNAPSHOT") != "" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot: %v (run with UPDATE_SNAPSHOT=1 to create it)", err)
	}
	if string(want) != got {
		t.Fatalf("schema drifted from testdata/schema.sql — if intentional, UPDATE_SNAPSHOT=1 go test ./internal/store -run TestSchemaSnapshot\n--- got ---\n%s", got)
	}
}

// TestMigrationsAreDenseAndRecorded: every embedded file is applied exactly
// once, versions are 1..N with no gaps, and Status reports nothing pending.
func TestMigrationsAreDenseAndRecorded(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()

	applied, pending, err := Status(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending after Open: %v", pending)
	}
	migs, _ := embeddedMigrations()
	if len(applied) != len(migs) {
		t.Fatalf("applied %d, embedded %d", len(applied), len(migs))
	}
	for i, a := range applied {
		if a.Version != i+1 {
			t.Fatalf("versions must be dense from 1: got %d at index %d", a.Version, i)
		}
		if a.Missing || a.Modified {
			t.Fatalf("migration %d flagged: %+v", a.Version, a)
		}
	}
}

// TestModifiedMigrationIsRefused: tampering with a shipped migration must
// stop the server, not silently diverge.
func TestModifiedMigrationIsRefused(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Forge a different checksum for version 1 as if the file had changed.
	if _, err := st.db.Exec(`UPDATE schema_migrations SET checksum='deadbeef' WHERE version=1`); err != nil {
		t.Fatal(err)
	}
	st.Close()

	if _, err := Open(dir); err == nil {
		t.Fatal("Open should refuse a modified migration")
	}
	applied, _, err := Status(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !applied[0].Modified {
		t.Fatalf("Status should flag version 1 as modified: %+v", applied[0])
	}
}

// user_version 시절 DB 는 이력만 기록하고 재적용하지 않는다(재적용하면 CREATE TABLE 에서 깨진다).
func TestLegacyUserVersionIsAdopted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "planner.db")

	migs, err := embeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) < 2 {
		t.Skip("needs at least two migrations to be meaningful")
	}

	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(migs[0].body); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	st, err := Open(dir)
	if err != nil {
		t.Fatalf("adopting legacy db: %v", err)
	}
	st.Close()

	applied, pending, err := Status(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending after adoption: %v", pending)
	}
	if len(applied) != len(migs) {
		t.Fatalf("applied %d, want %d — adoption records 1, migrate applies the rest", len(applied), len(migs))
	}
	for _, a := range applied {
		if a.Modified || a.Missing {
			t.Fatalf("migration %d flagged after adoption: %+v", a.Version, a)
		}
	}
}
