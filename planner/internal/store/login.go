package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// 계정 잠금 단계. 연속 실패가 accountThreshold를 넘을 때마다 다음 단계로 간다.
// 영구 잠금은 두지 않는다 — 가족이 스스로 걸릴 텐데 내가 풀어줘야 하면 곤란하다.
var lockSteps = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}

const (
	accountThreshold = 5 // 연속 실패 5회부터 잠금

	// IP 버킷: 창 안에서 시도 자체를 제한한다. 계정 잠금과 달리 성공해도 줄지
	// 않는다 — 목적이 '누가 맞혔나'가 아니라 'bcrypt를 몇 번 돌렸나'이기 때문.
	ipWindow = 15 * time.Minute
	ipMax    = 20

	// 없는 이름으로 시도하는 건 가족일 수 없다 — 자기 이름은 틀리지 않는다.
	// 스캐너 신호로 보고 IP를 세게 막는다. 오탐이 거의 없는 대신, 셀룰러
	// CGNAT를 공유하는 가족이 걸릴 수 있어 영구 차단은 하지 않는다.
	unknownThreshold = 3
	unknownLock      = time.Hour
)

// LoginGate는 시도를 허용할지와, 막는다면 얼마나 기다려야 하는지다.
type LoginGate struct {
	Allowed bool
	Retry   time.Duration // Retry-After 헤더용
}

// CheckLogin은 계정 키와 IP 키를 모두 보고 시도를 허용할지 정한다.
// 호출자는 막히더라도 비밀번호 비교를 건너뛰면 안 된다 — 응답이 즉시 돌아오면
// 잠금 상태가 타이밍으로 드러난다.
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

// RecordAttempt는 시도 결과를 남긴다. 성공이면 계정 키를 지우고, 실패면 계정
// 연속 실패를 올려 필요하면 잠근다. IP 창은 성공·실패 무관하게 올라가고,
// 없는 이름 시도는 IP 키의 fails를 올려 임계값을 넘으면 그 IP를 잠근다.
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
