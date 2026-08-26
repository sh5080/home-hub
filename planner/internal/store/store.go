// Package store is the SQLite persistence layer for the planner.
//
// One rule governs every method here: the pool holds exactly ONE connection
// (SetMaxOpenConns(1)), so a method that already holds a *sql.Tx must never
// call db.Query/db.Exec — it would wait forever for a second connection.
// Methods therefore take either the *Store (auto-commit) or an explicit *sql.Tx,
// never both.
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver: keeps CGO_ENABLED=0 cross-compile working
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// ValidationError marks bad caller input (HTTP 400), as opposed to a driver
// failure (HTTP 500). Handlers distinguish the two with errors.As.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(msg string) error { return &ValidationError{Msg: msg} }

// Store wraps the single-connection SQLite handle.
type Store struct {
	db    *sql.DB
	path  string
	quota int64 // 0이면 DefaultQuota
}

// Open opens (creating if needed) the database at dir/planner.db and applies
// pending migrations. WAL + NORMAL sync is the right trade for an SD card:
// durable against an app crash, may lose the last transaction on power loss.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	path := filepath.Join(dir, "planner.db")
	dsn := "file:" + path +
		"?_pragma=busy_timeout(5000)" + // the CLI (user add / backup) is a second process
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.backfillContent(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.backfillEvents(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.backfillPriority(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.seed(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the connection.
func (s *Store) Close() error { return s.db.Close() }

// Path is the on-disk database file (used by backup).
func (s *Store) Path() string { return s.path }

// Migration is one embedded migrations/NNNN_name.sql file.
type Migration struct {
	Version  int
	Name     string
	Checksum string // sha256 of the file body
	body     string
}

// AppliedMigration is a row of schema_migrations.
type AppliedMigration struct {
	Version   int
	Name      string
	Checksum  string
	AppliedAt int64
	// Missing is true when the DB has this version but this binary doesn't
	// carry the file — i.e. the binary is older than the database.
	Missing bool
	// Modified is true when the embedded file no longer matches what was
	// applied — someone edited a shipped migration. Open refuses to start.
	Modified bool
}

// embeddedMigrations lists migrations/*.sql sorted by version.
func embeddedMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	var migs []Migration
	seen := map[int]string{}
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".sql") {
			continue
		}
		v, err := strconv.Atoi(strings.SplitN(n, "_", 2)[0])
		if err != nil {
			return nil, fmt.Errorf("migration %q: name must start with NNNN_", n)
		}
		if prev, dup := seen[v]; dup {
			return nil, fmt.Errorf("migrations %q and %q share version %d", prev, n, v)
		}
		seen[v] = n
		body, err := migrationFS.ReadFile("migrations/" + n)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(body)
		migs = append(migs, Migration{Version: v, Name: n, Checksum: hex.EncodeToString(sum[:]), body: string(body)})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].Version < migs[j].Version })
	return migs, nil
}

const schemaMigrationsDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version    INTEGER PRIMARY KEY,
  name       TEXT    NOT NULL,
  checksum   TEXT    NOT NULL,
  applied_at INTEGER NOT NULL
)`

// readApplied returns schema_migrations keyed by version, creating the table
// on first use. A database created by the earlier PRAGMA user_version scheme
// is adopted: its applied versions are recorded with the current checksums.
func readApplied(db *sql.DB, migs []Migration) (map[int]AppliedMigration, error) {
	if _, err := db.Exec(schemaMigrationsDDL); err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil {
		return nil, err
	}
	if count == 0 {
		var legacy int
		if err := db.QueryRow(`PRAGMA user_version`).Scan(&legacy); err != nil {
			return nil, err
		}
		if legacy > 0 {
			now := time.Now().Unix()
			for _, m := range migs {
				if m.Version <= legacy {
					if _, err := db.Exec(`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`,
						m.Version, m.Name, m.Checksum, now); err != nil {
						return nil, fmt.Errorf("adopt legacy version %d: %w", m.Version, err)
					}
				}
			}
		}
	}
	rows, err := db.Query(`SELECT version, name, checksum, applied_at FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]AppliedMigration{}
	for rows.Next() {
		var a AppliedMigration
		if err := rows.Scan(&a.Version, &a.Name, &a.Checksum, &a.AppliedAt); err != nil {
			return nil, err
		}
		out[a.Version] = a
	}
	return out, rows.Err()
}

// migrate applies every embedded migration the database hasn't seen, in
// version order, each in its own transaction, recording its checksum.
//
// It refuses to start if an already-applied migration's file has changed:
// editing a shipped migration silently forks the schema between machines.
// Write a new NNNN file instead. A database ahead of the binary (versions
// with no file) is tolerated so an older CLI binary can still run.
func (s *Store) migrate() error {
	migs, err := embeddedMigrations()
	if err != nil {
		return err
	}
	applied, err := readApplied(s.db, migs)
	if err != nil {
		return err
	}
	maxApplied := 0
	for v := range applied {
		if v > maxApplied {
			maxApplied = v
		}
	}
	for _, m := range migs {
		if a, ok := applied[m.Version]; ok {
			if a.Checksum != m.Checksum {
				return fmt.Errorf("migration %s was modified after it was applied (db %s…, file %s…): write a new migration instead",
					m.Name, a.Checksum[:8], m.Checksum[:8])
			}
			continue
		}
		if m.Version < maxApplied {
			return fmt.Errorf("migration %s has a lower version than the already-applied %d: number new migrations after the latest", m.Name, maxApplied)
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(m.body); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply %s: %w", m.Name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`,
			m.Version, m.Name, m.Checksum, time.Now().Unix()); err != nil {
			tx.Rollback()
			return fmt.Errorf("record %s: %w", m.Name, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		maxApplied = m.Version
	}
	return nil
}

// Status reports applied and pending migrations for dir WITHOUT applying
// anything — for `planner migrate status`.
func Status(dir string) (applied []AppliedMigration, pending []Migration, err error) {
	migs, err := embeddedMigrations()
	if err != nil {
		return nil, nil, err
	}
	path := filepath.Join(dir, "planner.db")
	if _, statErr := os.Stat(path); statErr != nil {
		return nil, migs, nil // no database yet: everything is pending
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	rows, err := readApplied(db, migs)
	if err != nil {
		return nil, nil, err
	}
	byVersion := map[int]Migration{}
	for _, m := range migs {
		byVersion[m.Version] = m
	}
	for v, a := range rows {
		if m, ok := byVersion[v]; !ok {
			a.Missing = true
		} else if m.Checksum != a.Checksum {
			a.Modified = true
		}
		applied = append(applied, a)
	}
	sort.Slice(applied, func(i, j int) bool { return applied[i].Version < applied[j].Version })
	for _, m := range migs {
		if _, ok := rows[m.Version]; !ok {
			pending = append(pending, m)
		}
	}
	return applied, pending, nil
}

// SchemaDump returns the live schema (tables, indexes) as SQL, sorted, for
// snapshot tests and documentation.
func (s *Store) SchemaDump(ctx context.Context) (string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT type, name, sql FROM sqlite_master
		WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%' AND name <> 'schema_migrations'
		ORDER BY CASE type WHEN 'table' THEN 0 ELSE 1 END, name`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var typ, name, ddl string
		if err := rows.Scan(&typ, &name, &ddl); err != nil {
			return "", err
		}
		b.WriteString(ddl)
		b.WriteString(";\n\n")
	}
	return b.String(), rows.Err()
}

// seed creates the default "할 일" board on an empty database so the todo
// list exists on first login without a separate module.
func (s *Store) seed() error {
	var n int
	if err := s.db.QueryRow("SELECT count(*) FROM boards").Scan(&n); err != nil {
		return fmt.Errorf("count boards: %w", err)
	}
	if n > 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	res, err := tx.Exec(`INSERT INTO boards (name, created_by, created_at) VALUES (?, NULL, ?)`, "할 일", now)
	if err != nil {
		return fmt.Errorf("seed board: %w", err)
	}
	boardID, _ := res.LastInsertId()
	for i, name := range []string{"할 일", "진행 중", "완료"} {
		if _, err := tx.Exec(`INSERT INTO columns (board_id, name, position) VALUES (?, ?, ?)`, boardID, name, i); err != nil {
			return fmt.Errorf("seed column: %w", err)
		}
	}
	return tx.Commit()
}

// backfillContent는 0003 이전에 만들어진 카드(content IS NULL)의 평문 설명을
// 단락 문서로 옮긴다. SQL이 아니라 Go에서 하는 건 문서 JSON의 모양을 한 곳
// (PlainToContent)에만 두기 위해서다. 멱등이라 매 기동 시 돌아도 안전하다.
func (s *Store) backfillContent() error {
	rows, err := s.db.Query(`SELECT id, description FROM cards WHERE content IS NULL`)
	if err != nil {
		return fmt.Errorf("scan cards for backfill: %w", err)
	}
	type row struct {
		id   int64
		desc string
	}
	var pending []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.desc); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, r)
	}
	rows.Close()
	if len(pending) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range pending {
		if _, err := tx.Exec(`UPDATE cards SET content=? WHERE id=?`, PlainToContent(r.desc), r.id); err != nil {
			return fmt.Errorf("backfill card %d: %w", r.id, err)
		}
	}
	return tx.Commit()
}

// backfillPriority는 노션에서 가져올 때 본문 콜아웃 텍스트로만 들어갔던
// 중요도(⭐ 개수)를 priority 컬럼으로 복구한다. LIKE로 걸러서 깨끗한 DB에서는
// 비용이 없고, 이미 값이 있는 카드는 건드리지 않아 멱등이다.
// 콜아웃 자체는 지우지 않는다 — 분류와 메모가 거기 같이 들어 있다.
func (s *Store) backfillPriority() error {
	rows, err := s.db.Query(`SELECT id, content FROM cards WHERE priority = 0 AND content LIKE '%중요도:%'`)
	if err != nil {
		return fmt.Errorf("scan cards for priority backfill: %w", err)
	}
	type row struct {
		id int64
		n  int
	}
	var pending []row
	for rows.Next() {
		var id int64
		var content string
		if err := rows.Scan(&id, &content); err != nil {
			rows.Close()
			return err
		}
		if n := starsAfter(content, "중요도:"); n > 0 {
			pending = append(pending, row{id, n})
		}
	}
	rows.Close()
	if len(pending) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range pending {
		if _, err := tx.Exec(`UPDATE cards SET priority=? WHERE id=?`, r.n, r.id); err != nil {
			return fmt.Errorf("backfill priority %d: %w", r.id, err)
		}
	}
	return tx.Commit()
}

// starsAfter는 marker 바로 뒤에 이어지는 ⭐ 개수를 센다. 다음 구분자(·)나
// 줄바꿈에서 멈춰, 본문 다른 곳의 별은 세지 않는다.
func starsAfter(text, marker string) int {
	i := strings.Index(text, marker)
	if i < 0 {
		return 0
	}
	n := 0
	for _, r := range text[i+len(marker):] {
		switch {
		case r == '⭐':
			n++
		case r == ' ':
			continue
		default:
			return min(n, maxPriority)
		}
	}
	return min(n, maxPriority)
}

// Backup writes a consistent snapshot to dst using VACUUM INTO — safe to run
// while the server is live, and the output is a compacted single file.
func (s *Store) Backup(ctx context.Context, dst string) error {
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", dst); err != nil {
		return fmt.Errorf("vacuum into: %w", err)
	}
	return nil
}

// Ping is the health probe.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
