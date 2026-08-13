package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/sh5080/home-hub/planner/internal/auth"
)

// MinPasswordLen은 공개(tailscale funnel) 노출을 전제로 한 최소 길이다.
// 4자는 랜 전용일 때의 값이었고, 인터넷에서 닿는 순간 너무 약하다.
const MinPasswordLen = 8

// ErrNotFound is returned when a row doesn't exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned on a unique-constraint violation (e.g. duplicate name).
var ErrConflict = errors.New("already exists")

// dummyHash is a real bcrypt hash compared against when the user doesn't
// exist, so the "no such user" path costs as much as a wrong password.
var dummyHash = func() string {
	h, _ := auth.HashPassword("planner-dummy")
	return h
}()

// User is a family member.
type User struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
}

// CreateUser inserts a user with a bcrypt-hashed password.
func (s *Store) CreateUser(ctx context.Context, name, password string) (User, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return User{}, invalid("name is empty")
	}
	if len(password) < MinPasswordLen {
		return User{}, invalid("비밀번호는 8자 이상이어야 해요")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return User{}, err
	}
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (name, password_hash, created_at) VALUES (?, ?, ?)`, name, hash, now)
	if err != nil {
		if isUnique(err) {
			return User{}, ErrConflict
		}
		return User{}, err
	}
	id, _ := res.LastInsertId()
	return User{ID: id, Name: name, CreatedAt: now}, nil
}

// SetPassword replaces the hash for name.
func (s *Store) SetPassword(ctx context.Context, name, password string) error {
	if len(password) < MinPasswordLen {
		return invalid("비밀번호는 8자 이상이어야 해요")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash=? WHERE name=?`, hash, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteUser removes a user. Assignments are nulled by the schema; content the
// user *created* keeps a NOT NULL created_by, so deleting such a user fails
// with a clear error rather than orphaning rows.
func (s *Store) DeleteUser(ctx context.Context, name string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE name=?`, name)
	if err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY") {
			return invalid("user has created cards/events; reassign or delete those first")
		}
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListUsers returns everyone, oldest first.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	if out == nil {
		out = []User{}
	}
	return out, rows.Err()
}

// UserExists reports whether name is a real account. Used to tell a family
// member's typo apart from a scanner probing made-up names.
func (s *Store) UserExists(ctx context.Context, name string) bool {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM users WHERE name=?`, strings.TrimSpace(name)).Scan(&n); err != nil {
		return false // 조회 실패 시 '없는 이름'으로 단정하지 않는다
	}
	return n > 0
}

// SetPasswordByID replaces the hash for a user id and invalidates every
// session they had — a reset must revoke whoever held the old credential.
func (s *Store) SetPasswordByID(ctx context.Context, id int64, password string) (User, error) {
	if len(password) < MinPasswordLen {
		return User{}, invalid("비밀번호는 8자 이상이어야 해요")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return User{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	var u User
	err = tx.QueryRowContext(ctx, `SELECT id, name, created_at FROM users WHERE id=?`, id).
		Scan(&u.ID, &u.Name, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash=? WHERE id=?`, hash, id); err != nil {
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, id); err != nil {
		return User{}, err
	}
	// 재설정은 잠금 해제이기도 하다 — 가족이 잊어서 잠긴 상태를 그대로 두면
	// 비밀번호를 바꾸고도 못 들어간다.
	if _, err := tx.ExecContext(ctx, `DELETE FROM login_attempts WHERE key=?`, "user:"+u.Name); err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return u, nil
}

// Authenticate checks name/password and returns the user on success.
// A wrong name and a wrong password are indistinguishable to the caller.
func (s *Store) Authenticate(ctx context.Context, name, password string) (User, bool, error) {
	var u User
	var hash string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, password_hash, created_at FROM users WHERE name=?`, strings.TrimSpace(name)).
		Scan(&u.ID, &u.Name, &hash, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// Burn roughly the same time as a real compare so name enumeration
		// by timing isn't trivial.
		auth.CheckPassword(dummyHash, password)
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	if !auth.CheckPassword(hash, password) {
		return User{}, false, nil
	}
	return u, true, nil
}

// --- sessions ---

// CreateSession issues a token for userID.
func (s *Store) CreateSession(ctx context.Context, userID int64) (string, error) {
	token, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	exp := time.Now().Add(auth.SessionTTL).Unix()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`, token, userID, exp); err != nil {
		return "", err
	}
	return token, nil
}

// DeleteSession logs a token out. Deleting a missing token is not an error.
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token=?`, token)
	return err
}

// UserBySession implements auth.Resolver.
func (s *Store) UserBySession(ctx context.Context, token string, now time.Time) (auth.User, bool, error) {
	var u auth.User
	err := s.db.QueryRowContext(ctx,
		`SELECT u.id, u.name FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token=? AND s.expires_at > ?`, token, now.Unix()).
		Scan(&u.ID, &u.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.User{}, false, nil
	}
	if err != nil {
		return auth.User{}, false, err
	}
	return u, true, nil
}

// PurgeExpiredSessions removes dead sessions; run periodically.
func (s *Store) PurgeExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// isUnique reports whether err is a SQLite UNIQUE violation.
// modernc surfaces it as a string; matching the text is the portable check.
func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
