package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestSchemaSnapshot pins the schema that all migrations produce from scratch.
// When you add a migration, the diff here is the review artifact — run
//
//	UPDATE_SNAPSHOT=1 go test ./internal/store -run TestSchemaSnapshot
//
// to accept it and commit testdata/schema.sql alongside the migration.
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

// TestLegacyUserVersionIsAdopted: a database from the PRAGMA user_version
// era gets its history recorded instead of being re-migrated (which would
// fail on CREATE TABLE).
func TestLegacyUserVersionIsAdopted(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the old scheme: drop the tracking table, set user_version.
	if _, err := st.db.Exec(`DROP TABLE schema_migrations`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	st.Close()

	st, err = Open(dir)
	if err != nil {
		t.Fatalf("adopting legacy db: %v", err)
	}
	st.Close()
	applied, pending, err := Status(dir)
	if err != nil || len(pending) != 0 || len(applied) != 1 {
		t.Fatalf("applied=%v pending=%v err=%v", applied, pending, err)
	}
}
