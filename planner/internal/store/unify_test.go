package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// 0006 이전 DB 를 열면 events 가 cards 로 옮겨지고 표가 사라진다.
func TestEventsAreMovedIntoCards(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "planner.db")

	// 0006 직전 상태를 만든다: 0005까지 적용 + events 테이블.
	migs, err := embeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migs {
		if m.Version >= 6 {
			break
		}
		if _, err := raw.Exec(m.body); err != nil {
			t.Fatalf("apply %s: %v", m.Name, err)
		}
	}
	now := time.Now().Unix()
	if _, err := raw.Exec(`INSERT INTO users (id,name,password_hash,created_at) VALUES (1,'t','x',?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO boards (id,name,created_at) VALUES (1,'b',?)`, now); err != nil {
		t.Fatal(err)
	}
	for i, n := range []string{"할 일", "진행 중", "완료"} {
		if _, err := raw.Exec(`INSERT INTO columns (id,board_id,name,position) VALUES (?,1,?,?)`, i+1, n, i); err != nil {
			t.Fatal(err)
		}
	}
	// 첫 컬럼에 기존 카드 하나 — position이 이어 붙는지 본다.
	if _, err := raw.Exec(`INSERT INTO cards (column_id,title,description,position,created_by,created_at,updated_at)
		VALUES (1,'기존','',0,1,?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	past, future := "2020-01-01", "2099-12-31"
	for _, e := range []struct {
		title, start string
		end          any
	}{
		{"지난 일정", past, nil},
		{"앞으로 일정", future, nil},
		{"여행", future, "2100-01-02"},
	} {
		if _, err := raw.Exec(`INSERT INTO events (title,start_at,end_at,all_day,description,created_by,created_at)
			VALUES (?,?,?,0,'메모',1,?)`, e.title, e.start, e.end, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := raw.Exec(`PRAGMA user_version = 5`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	st, err := Open(dir)
	if err != nil {
		t.Fatalf("0006 적용 실패: %v", err)
	}
	defer st.Close()
	ctx := context.Background()

	// events 테이블이 사라졌다.
	var n int
	if err := st.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='events'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("events 테이블이 남아 있다")
	}

	d, err := st.GetBoard(ctx, 1, SortManual, Asc)
	if err != nil {
		t.Fatal(err)
	}
	first, last := d.Columns[0], d.Columns[2]

	// 앞으로의 일정 2건이 첫 컬럼에, 기존 카드 뒤에 붙었다.
	eq(t, titlesOf(first.Cards), []string{"기존", "앞으로 일정", "여행"})
	// 지난 일정은 마지막 컬럼에. 첫 컬럼에 있으면 '연체된 할 일'로 보인다.
	eq(t, titlesOf(last.Cards), []string{"지난 일정"})

	// position이 컬럼마다 0부터 조밀하다.
	for _, col := range d.Columns {
		for i, c := range col.Cards {
			if c.Position != i {
				t.Fatalf("%s의 %q position=%d, want %d", col.Name, c.Title, c.Position, i)
			}
		}
	}

	// 날짜와 메모가 보존됐다.
	var trip Card
	for _, c := range first.Cards {
		if c.Title == "여행" {
			trip = c
		}
	}
	if trip.DueAt == nil || *trip.DueAt != future || trip.EndAt == nil || *trip.EndAt != "2100-01-02" {
		t.Fatalf("여행 날짜 유실: due=%v end=%v", trip.DueAt, trip.EndAt)
	}
	if trip.Description != "메모" {
		t.Fatalf("메모 유실: %q", trip.Description)
	}

	// 캘린더가 옮겨진 항목을 본다 — 통합의 요점이다.
	cards, err := st.CalendarCards(ctx, "2099-01-01", "2100-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 {
		t.Fatalf("캘린더에 %d건, want 2: %+v", len(cards), titlesOf(cards))
	}

	// 다시 열어도 안전하다(멱등).
	st.Close()
	if st2, err := Open(dir); err != nil {
		t.Fatalf("재기동 실패: %v", err)
	} else {
		st2.Close()
	}
}

func titlesOf(cards []Card) []string {
	out := make([]string, len(cards))
	for i, c := range cards {
		out[i] = c.Title
	}
	return out
}
