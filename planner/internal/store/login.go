package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// 계정 잠금 단계. 영구 잠금은 없다(가족이 스스로 걸린다).
var lockSteps = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}

const (
	accountThreshold = 5 // 연속 실패 5회부터 잠금

	// IP 버킷은 창 안의 시도 수를 센다. 성공해도 줄지 않는다(목적은 bcrypt 횟수 제한).
	ipWindow = 15 * time.Minute
	ipMax    = 20

	// 없는 이름 시도는 스캐너 신호라 IP 를 세게 막는다. CGNAT 오탐 때문에 영구 차단은 없다.
	unknownThreshold = 3
	unknownLock      = time.Hour
)

// LoginGate는 시도를 허용할지와, 막는다면 얼마나 기다려야 하는지다.
type LoginGate struct {
	Allowed bool
	Retry   time.Duration // Retry-After 헤더용
}

// CheckLogin 은 계정·IP 키로 시도 허용 여부를 정한다.
// 호출자는 막혀도 비밀번호 비교를 건너뛰면 안 된다(타이밍으로 잠금이 드러난다).
func (s *Store) CheckLogin(ctx context.Context, account, ip string, now time.Time) (LoginGate, error) {
	for _, key := range keysFor(account, ip) {
		var lockedUntil, windowStart int64
		var windowCount int
		err := s.db.QueryRowContext(ctx,
			`SELECT locked_until, window_start, window_count FROM login_attempts WHERE key=?`, key).
			Scan(&lockedUntil, &windowStart, &windowCount)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return LoginGate{}, err
		}
		if lockedUntil > now.Unix() {
			return LoginGate{Allowed: false, Retry: time.Duration(lockedUntil-now.Unix()) * time.Second}, nil
		}
		// IP 버킷은 창이 살아 있을 때만 센다.
		if isIPKey(key) && now.Unix()-windowStart < int64(ipWindow.Seconds()) && windowCount >= ipMax {
			retry := int64(ipWindow.Seconds()) - (now.Unix() - windowStart)
			return LoginGate{Allowed: false, Retry: time.Duration(retry) * time.Second}, nil
		}
	}
	return LoginGate{Allowed: true}, nil
}

// AttemptOutcome은 한 번의 로그인 시도 결과다.
type AttemptOutcome int

const (
	AttemptSuccess     AttemptOutcome = iota
	AttemptWrongPass                  // 이름은 있는데 비밀번호가 틀림 — 가족일 수 있다
	AttemptUnknownUser                // 이름 자체가 없음 — 스캐너 신호
)

// RecordAttempt 는 결과를 남긴다. 성공이면 계정 키를 지우고 실패면 연속 실패를 올린다.
// IP 창은 성공·실패 무관하게 올라간다.
func (s *Store) RecordAttempt(ctx context.Context, account, ip string, outcome AttemptOutcome, now time.Time) error {
	success := outcome == AttemptSuccess
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if ip != "" {
		if err := bumpWindow(ctx, tx, "ip:"+ip, now); err != nil {
			return err
		}
		if outcome == AttemptUnknownUser {
			if err := bumpUnknown(ctx, tx, "ip:"+ip, now); err != nil {
				return err
			}
		}
	}
	if account != "" {
		key := "user:" + account
		if success {
			if _, err := tx.ExecContext(ctx, `DELETE FROM login_attempts WHERE key=?`, key); err != nil {
				return err
			}
		} else {
			var fails int
			err := tx.QueryRowContext(ctx, `SELECT fails FROM login_attempts WHERE key=?`, key).Scan(&fails)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			fails++
			var lockedUntil int64
			if fails >= accountThreshold {
				step := fails - accountThreshold
				if step >= len(lockSteps) {
					step = len(lockSteps) - 1
				}
				lockedUntil = now.Add(lockSteps[step]).Unix()
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO login_attempts (key, fails, window_start, window_count, locked_until)
				VALUES (?, ?, ?, 0, ?)
				ON CONFLICT(key) DO UPDATE SET fails=excluded.fails, locked_until=excluded.locked_until`,
				key, fails, now.Unix(), lockedUntil); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// bumpWindow는 IP 창을 1 올린다. 창이 만료됐으면 새 창을 연다.
func bumpWindow(ctx context.Context, tx *sql.Tx, key string, now time.Time) error {
	var start int64
	var count int
	err := tx.QueryRowContext(ctx, `SELECT window_start, window_count FROM login_attempts WHERE key=?`, key).Scan(&start, &count)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		start, count = now.Unix(), 0
	case err != nil:
		return err
	case now.Unix()-start >= int64(ipWindow.Seconds()):
		start, count = now.Unix(), 0
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO login_attempts (key, fails, window_start, window_count, locked_until)
		VALUES (?, 0, ?, ?, 0)
		ON CONFLICT(key) DO UPDATE SET window_start=excluded.window_start, window_count=excluded.window_count`,
		key, start, count+1)
	return err
}

// bumpUnknown은 없는 이름 시도를 IP 키에 누적하고, 임계값을 넘으면 잠근다.
func bumpUnknown(ctx context.Context, tx *sql.Tx, key string, now time.Time) error {
	var fails int
	err := tx.QueryRowContext(ctx, `SELECT fails FROM login_attempts WHERE key=?`, key).Scan(&fails)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	fails++
	var lockedUntil int64
	if fails >= unknownThreshold {
		lockedUntil = now.Add(unknownLock).Unix()
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE login_attempts SET fails=?, locked_until=? WHERE key=?`, fails, lockedUntil, key)
	return err
}

// PurgeLoginAttempts는 잠기지 않았고 창도 지난 행을 지운다. 주기 작업용.
func (s *Store) PurgeLoginAttempts(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM login_attempts
		WHERE locked_until <= ? AND window_start < ?`, now.Unix(), now.Add(-ipWindow).Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func keysFor(account, ip string) []string {
	var out []string
	if account != "" {
		out = append(out, "user:"+account)
	}
	if ip != "" {
		out = append(out, "ip:"+ip)
	}
	return out
}

func isIPKey(k string) bool { return len(k) > 3 && k[:3] == "ip:" }
