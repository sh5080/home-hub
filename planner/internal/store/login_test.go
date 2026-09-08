package store

import (
	"context"
	"testing"
	"time"
)

func TestAccountLocksAfterThreshold(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	now := time.Now()

	for i := 0; i < accountThreshold; i++ {
		g, err := st.CheckLogin(ctx, "테스트1", "1.2.3.4", now)
		if err != nil {
			t.Fatal(err)
		}
		if !g.Allowed {
			t.Fatalf("blocked too early at attempt %d", i)
		}
		if err := st.RecordAttempt(ctx, "테스트1", "1.2.3.4", AttemptWrongPass, now); err != nil {
			t.Fatal(err)
		}
	}
	g, err := st.CheckLogin(ctx, "테스트1", "1.2.3.4", now)
	if err != nil {
		t.Fatal(err)
	}
	if g.Allowed {
		t.Fatal("should be locked after threshold")
	}
	if g.Retry <= 0 || g.Retry > time.Minute {
		t.Fatalf("first lock should be ~1분, got %v", g.Retry)
	}
	// 잠금이 풀리면 다시 허용된다.
	if g, _ := st.CheckLogin(ctx, "테스트1", "1.2.3.4", now.Add(2*time.Minute)); !g.Allowed {
		t.Fatal("should unlock after the window")
	}
	// 다른 계정은 영향 없다 (IP 상한에는 아직 안 걸림).
	if g, _ := st.CheckLogin(ctx, "테스트2", "1.2.3.4", now); !g.Allowed {
		t.Fatal("other account should not be locked")
	}
}

func TestLockEscalates(t *testing.T) {
	st, _ := Open(t.TempDir())
	defer st.Close()
	ctx := context.Background()
	now := time.Now()

	for i := 0; i < accountThreshold+2; i++ {
		_ = st.RecordAttempt(ctx, "테스트1", "", AttemptWrongPass, now)
	}
	g, _ := st.CheckLogin(ctx, "테스트1", "", now)
	if !g.Allowed && g.Retry <= time.Minute {
		t.Fatalf("lock should escalate past 1분, got %v", g.Retry)
	}
}

func TestSuccessClearsAccountLock(t *testing.T) {
	st, _ := Open(t.TempDir())
	defer st.Close()
	ctx := context.Background()
	now := time.Now()

	for i := 0; i < accountThreshold-1; i++ {
		_ = st.RecordAttempt(ctx, "테스트1", "", AttemptWrongPass, now)
	}
	_ = st.RecordAttempt(ctx, "테스트1", "", AttemptSuccess, now)
	// 성공이 연속 실패를 지웠으니 다시 threshold 만큼 버틴다.
	for i := 0; i < accountThreshold-1; i++ {
		_ = st.RecordAttempt(ctx, "테스트1", "", AttemptWrongPass, now)
	}
	if g, _ := st.CheckLogin(ctx, "테스트1", "", now); !g.Allowed {
		t.Fatal("success should have reset the failure count")
	}
}

func TestUnknownUserBlocksIP(t *testing.T) {
	st, _ := Open(t.TempDir())
	defer st.Close()
	ctx := context.Background()
	now := time.Now()

	for i := 0; i < unknownThreshold; i++ {
		if g, _ := st.CheckLogin(ctx, "admin", "9.9.9.9", now); !g.Allowed {
			t.Fatalf("blocked too early at %d", i)
		}
		_ = st.RecordAttempt(ctx, "admin", "9.9.9.9", AttemptUnknownUser, now)
	}
	// 그 IP는 이제 실제 계정으로도 막힌다 — 스캐너를 통째로 세운다.
	g, _ := st.CheckLogin(ctx, "테스트1", "9.9.9.9", now)
	if g.Allowed {
		t.Fatal("IP should be blocked after repeated unknown usernames")
	}
	if g.Retry < 30*time.Minute {
		t.Fatalf("unknown-user block should be long, got %v", g.Retry)
	}
	// 다른 IP는 멀쩡하다.
	if g, _ := st.CheckLogin(ctx, "테스트1", "1.1.1.1", now); !g.Allowed {
		t.Fatal("other IP should be unaffected")
	}
	// 영구는 아니다.
	if g, _ := st.CheckLogin(ctx, "테스트1", "9.9.9.9", now.Add(unknownLock+time.Minute)); !g.Allowed {
		t.Fatal("block must expire, not be permanent")
	}
}

func TestIPWindowCapsAttempts(t *testing.T) {
	st, _ := Open(t.TempDir())
	defer st.Close()
	ctx := context.Background()
	now := time.Now()

	// 매번 다른 '있는 이름'으로 시도해 IP 상한만 시험한다.
	for i := 0; i < ipMax; i++ {
		if g, _ := st.CheckLogin(ctx, "", "5.5.5.5", now); !g.Allowed {
			t.Fatalf("blocked too early at %d", i)
		}
		_ = st.RecordAttempt(ctx, "", "5.5.5.5", AttemptWrongPass, now)
	}
	if g, _ := st.CheckLogin(ctx, "", "5.5.5.5", now); g.Allowed {
		t.Fatal("IP window cap should stop further attempts")
	}
	// 창이 지나면 다시 열린다.
	if g, _ := st.CheckLogin(ctx, "", "5.5.5.5", now.Add(ipWindow+time.Minute)); !g.Allowed {
		t.Fatal("IP window should reset")
	}
}

func TestResetPasswordKillsSessionsAndUnlocks(t *testing.T) {
	st, _ := Open(t.TempDir())
	defer st.Close()
	ctx := context.Background()
	now := time.Now()

	u, err := st.CreateUser(ctx, "테스트1", "oldpass123")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := st.CreateSession(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	// 잠긴 상태를 만든다.
	for i := 0; i < accountThreshold; i++ {
		_ = st.RecordAttempt(ctx, "테스트1", "", AttemptWrongPass, now)
	}
	if g, _ := st.CheckLogin(ctx, "테스트1", "", now); g.Allowed {
		t.Fatal("expected locked")
	}

	if _, err := st.SetPasswordByID(ctx, u.ID, "newpass123"); err != nil {
		t.Fatal(err)
	}
	// 세션이 끊겼다.
	if _, ok, _ := st.UserBySession(ctx, tok, now); ok {
		t.Fatal("reset must invalidate existing sessions")
	}
	// 잠금이 풀렸다 — 안 그러면 새 비밀번호로도 못 들어간다.
	if g, _ := st.CheckLogin(ctx, "테스트1", "", now); !g.Allowed {
		t.Fatal("reset must clear the account lock")
	}
	// 새 비밀번호가 먹는다.
	if _, ok, _ := st.Authenticate(ctx, "테스트1", "newpass123"); !ok {
		t.Fatal("new password should work")
	}
	if _, ok, _ := st.Authenticate(ctx, "테스트1", "oldpass123"); ok {
		t.Fatal("old password must stop working")
	}
}

func TestShortPasswordRejected(t *testing.T) {
	st, _ := Open(t.TempDir())
	defer st.Close()
	ctx := context.Background()
	if _, err := st.CreateUser(ctx, "테스트3", "1234"); err == nil {
		t.Fatal("password shorter than MinPasswordLen must be rejected")
	}
}

func TestUserExists(t *testing.T) {
	st, _ := Open(t.TempDir())
	defer st.Close()
	ctx := context.Background()
	if _, err := st.CreateUser(ctx, "테스트1", "pass1234"); err != nil {
		t.Fatal(err)
	}
	if !st.UserExists(ctx, "테스트1") || !st.UserExists(ctx, "  테스트1  ") {
		t.Fatal("existing user not found")
	}
	if st.UserExists(ctx, "admin") {
		t.Fatal("nonexistent user reported as existing")
	}
}
