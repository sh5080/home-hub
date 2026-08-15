package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func nowFixed() time.Time { return time.Unix(1_800_000_000, 0) }

// 모든 SQL은 ? 플레이스홀더로 값을 바인딩한다. 동적으로 조립하는 유일한 곳
// (UpdateCard/UpdateRoutine의 SET 절)도 조각이 전부 하드코딩 리터럴이고
// 값은 args로 나간다. 이 테스트는 그 불변식을 고정한다 — 누군가 나중에
// fmt.Sprintf로 SQL을 만들면 여기서 깨져야 한다.
var payloads = []string{
	`'; DROP TABLE users;--`,
	`" OR "1"="1`,
	`admin'--`,
	`'); DELETE FROM cards; --`,
	`1; UPDATE users SET password_hash='x'`,
	`\'`,
	"\x00truncated",
	`%' OR name LIKE '%`,
}

func TestInjectionPayloadsAreStoredAsData(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	owner, err := st.CreateUser(ctx, "테스트1", "pass1234")
	if err != nil {
		t.Fatal(err)
	}
	boards, _ := st.ListBoards(ctx)
	d, _ := st.GetBoard(ctx, boards[0].ID, SortManual)
	col := d.Columns[0].ID

	for _, p := range payloads {
		p := p

		// 사용자 이름
		u, err := st.CreateUser(ctx, p, "pass1234")
		if err != nil {
			t.Fatalf("CreateUser(%q): %v", p, err)
		}
		if u.Name != strings.TrimSpace(p) {
			t.Fatalf("name round-trip: got %q want %q", u.Name, strings.TrimSpace(p))
		}
		if !st.UserExists(ctx, p) {
			t.Fatalf("UserExists(%q) false — payload was not stored literally", p)
		}

		// 카드 제목·설명 (생성 + 부분 수정 = 동적 SET 절 경로)
		c, err := st.CreateCard(ctx, col, CardInput{Title: &p}, owner.ID)
		if err != nil {
			t.Fatalf("CreateCard(%q): %v", p, err)
		}
		doc := PlainToContent(p)
		c2, err := st.UpdateCard(ctx, c.ID, CardInput{Content: &doc})
		if err != nil {
			t.Fatalf("UpdateCard(%q): %v", p, err)
		}
		if c2.Title != strings.TrimSpace(p) {
			t.Fatalf("title round-trip: %q", c2.Title)
		}
		// description은 content에서 파생된 평문이다.
		if c2.Description != strings.TrimSpace(p) {
			t.Fatalf("derived plain text: got %q want %q", c2.Description, strings.TrimSpace(p))
		}

		// 루틴 제목 (다른 동적 SET 절)
		mask := 127
		rt, err := st.CreateRoutine(ctx, RoutineInput{Title: &p, WeekdaysMask: &mask})
		if err != nil {
			t.Fatalf("CreateRoutine(%q): %v", p, err)
		}
		if _, err := st.UpdateRoutine(ctx, rt.ID, RoutineInput{Title: &p}); err != nil {
			t.Fatalf("UpdateRoutine(%q): %v", p, err)
		}

		// 로그인 경로: 이름·레이트리밋 키에 그대로 들어간다
		if _, ok, err := st.Authenticate(ctx, p, "wrongpass"); err != nil || ok {
			t.Fatalf("Authenticate(%q): ok=%v err=%v", p, ok, err)
		}
		if err := st.RecordAttempt(ctx, p, p, AttemptWrongPass, nowFixed()); err != nil {
			t.Fatalf("RecordAttempt(%q): %v", p, err)
		}
		if _, err := st.CheckLogin(ctx, p, p, nowFixed()); err != nil {
			t.Fatalf("CheckLogin(%q): %v", p, err)
		}

		// 범위 쿼리 파라미터
		if _, err := st.ListEvents(ctx, p, p); err != nil {
			t.Fatalf("ListEvents(%q): %v", p, err)
		}
		if _, err := st.DueCards(ctx, p, p); err != nil {
			t.Fatalf("DueCards(%q): %v", p, err)
		}
	}

	// 테이블이 전부 살아 있고 행 수가 기대대로인지 — DROP/DELETE가 실행됐다면
	// 여기서 무너진다.
	for _, tbl := range []string{"users", "boards", "columns", "cards", "events", "routines", "sessions", "login_attempts", "schema_migrations"} {
		var n int
		if err := st.db.QueryRow(`SELECT count(*) FROM ` + tbl).Scan(&n); err != nil {
			t.Fatalf("table %s missing or broken: %v", tbl, err)
		}
	}
	var users, cards int
	st.db.QueryRow(`SELECT count(*) FROM users`).Scan(&users)
	st.db.QueryRow(`SELECT count(*) FROM cards`).Scan(&cards)
	if users != len(payloads)+1 {
		t.Fatalf("users = %d, want %d — rows were deleted or not inserted", users, len(payloads)+1)
	}
	if cards != len(payloads) {
		t.Fatalf("cards = %d, want %d", cards, len(payloads))
	}
	// 비밀번호 해시가 덮어써지지 않았다 (UPDATE users SET password_hash 페이로드).
	if _, ok, _ := st.Authenticate(ctx, "테스트1", "pass1234"); !ok {
		t.Fatal("owner password no longer works — an UPDATE payload executed")
	}
}
