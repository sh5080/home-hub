package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/sh5080/home-hub/planner/internal/auth"
)

// MinPasswordLen 은 funnel(공개) 노출 기준의 최소 길이.
const MinPasswordLen = 8

// ErrNotFound is returned when a row doesn't exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned on a unique-constraint violation (e.g. duplicate name).
var ErrConflict = errors.New("already exists")

// dummyHash: 없는 사용자도 비밀번호 비교만큼 시간을 쓰게 한다(이름 추측 방지).
var dummyHash = func() string {
	h, _ := auth.HashPassword("planner-dummy")
	return h
}()

// User is a family member.
type User struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// BirthDate 는 'YYYY-MM-DD'. 어른도 포함. 아이는 이유식 D+n 기준.
	BirthDate *string `json:"birth_date"`
	CreatedAt int64   `json:"created_at"`
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

// DeleteUser 는 사용자를 지운다. created_by 가 NOT NULL 인 행이 있으면 실패한다.
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
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, birth_date, created_at FROM users ORDER BY birth_date IS NULL, birth_date, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.BirthDate, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	if out == nil {
		out = []User{}
	}
	return out, rows.Err()
}

// UserExists 는 가족의 오타와 이름을 지어내는 스캐너를 구분하는 데 쓴다.
func (s *Store) UserExists(ctx context.Context, name string) bool {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM users WHERE name=?`, strings.TrimSpace(name)).Scan(&n); err != nil {
		return false // 조회 실패 시 '없는 이름'으로 단정하지 않는다
	}
	return n > 0
}

// SetPasswordByID 는 비밀번호를 바꾸고 모든 세션을 끊는다.
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
	// 재설정은 로그인 잠금도 푼다.
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

// isUnique 는 SQLite UNIQUE 위반인지 본다(modernc 는 문자열로만 준다).
func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// SetBirthDate 는 생년월일을 넣거나 지운다(""). 이유식 D+n 기준이라 형식을 여기서 막는다.
func (s *Store) SetBirthDate(ctx context.Context, id int64, date string) (User, error) {
	date = strings.TrimSpace(date)
	var v *string
	if date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return User{}, invalid("생년월일은 YYYY-MM-DD 형식이어야 해요")
		}
		v = &date
	}
	res, err := s.db.ExecContext(ctx, `UPDATE users SET birth_date=? WHERE id=?`, v, id)
	if err != nil {
		return User{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return User{}, ErrNotFound
	}
	var u User
	err = s.db.QueryRowContext(ctx,
		`SELECT id, name, birth_date, created_at FROM users WHERE id=?`, id).
		Scan(&u.ID, &u.Name, &u.BirthDate, &u.CreatedAt)
	return u, err
}
