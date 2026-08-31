package store

import (
	"context"
	"testing"
)

func TestValidateRecur(t *testing.T) {
	ok := []struct{ in, want string }{
		{"", ""}, {"daily", "daily"}, {" daily ", "daily"},
		{"every:14", "every:14"}, {"weekly:5", "weekly:5"},
		{"monthly:31", "monthly:31"}, {"yearly:04-07", "yearly:04-07"},
	}
	for _, c := range ok {
		got, err := ValidateRecur(c.in)
		if err != nil || got != c.want {
			t.Fatalf("ValidateRecur(%q) = %q, %v — want %q", c.in, got, err, c.want)
		}
	}
	bad := []string{"every:0", "every:366", "weekly:0", "weekly:128", "monthly:0", "monthly:32",
		"yearly:13-01", "yearly:3-5", "daily:1", "무엇", "every:하루"}
	for _, c := range bad {
		if _, err := ValidateRecur(c); err == nil {
			t.Fatalf("ValidateRecur(%q) 가 통과했다", c)
		}
	}
}

func TestNextOccurrence(t *testing.T) {
	cases := []struct{ rule, from, want, why string }{
		{"daily", "2026-08-31", "2026-09-01", "달을 넘는다"},
		{"daily", "2026-08-31T09:00", "2026-09-01T09:00", "시각은 유지된다"},
		{"every:14", "2026-08-31", "2026-09-14", ""},
		// 월=bit0. 2026-08-31은 월요일. weekly:4(=수)면 다음 수요일.
		{"weekly:4", "2026-08-31", "2026-09-02", "마스크의 다음 요일"},
		{"weekly:1", "2026-08-31", "2026-09-07", "오늘이 그 요일이어도 다음 주로"},
		{"weekly:127", "2026-08-31", "2026-09-01", "매일 마스크는 바로 다음 날"},
		// 없는 날은 그 달 마지막 날로 당긴다
		{"monthly:31", "2026-01-31", "2026-02-28", "2월에는 31일이 없다"},
		{"monthly:31", "2026-02-28", "2026-03-31", "당겨졌던 규칙이 되살아난다"},
		{"monthly:15", "2026-12-15", "2027-01-15", "해를 넘는다"},
		{"yearly:04-07", "2026-04-07", "2027-04-07", ""},
		{"yearly:02-29", "2026-02-28", "2027-02-28", "평년에는 28일로 당긴다"},
		{"yearly:02-29", "2027-02-28", "2028-02-29", "윤년에는 29일 그대로"},
		{"yearly:01-10", "2026-04-07", "2027-01-10", "이미 지난 날짜는 내년으로"},
	}
	for _, c := range cases {
		got, err := NextOccurrence(c.rule, c.from)
		if err != nil {
			t.Fatalf("%s from %s: %v", c.rule, c.from, err)
		}
		if got != c.want {
			t.Fatalf("%s from %s = %s, want %s  (%s)", c.rule, c.from, got, c.want, c.why)
		}
	}
}

// 다음 회차는 반드시 현재보다 뒤여야 한다. 아니면 완료할 때마다 같은 날짜가
// 다시 생겨 무한히 쌓인다.
func TestNextOccurrenceAlwaysMovesForward(t *testing.T) {
	rules := []string{"daily", "every:1", "weekly:1", "weekly:127", "monthly:1", "monthly:31", "yearly:01-01", "yearly:02-29"}
	for _, r := range rules {
		cur := "2026-01-01"
		for i := 0; i < 40; i++ {
			next, err := NextOccurrence(r, cur)
			if err != nil {
				t.Fatalf("%s: %v", r, err)
			}
			if next <= cur {
				t.Fatalf("%s: %s → %s 는 앞으로 가지 않는다", r, cur, next)
			}
			cur = next
		}
	}
}

func TestNextOccurrenceRejectsBadInput(t *testing.T) {
	if _, err := NextOccurrence("", "2026-01-01"); err == nil {
		t.Fatal("빈 규칙이 통과했다")
	}
	if _, err := NextOccurrence("daily", "어제"); err == nil {
		t.Fatal("이상한 마감이 통과했다")
	}
}

func TestRecurLabel(t *testing.T) {
	cases := map[string]string{
		"daily": "매일", "every:14": "14일마다", "weekly:5": "매주 월·수",
		"weekly:127": "매일", "monthly:15": "매월 15일", "yearly:04-07": "매년 4월 7일",
	}
	for rule, want := range cases {
		if got := RecurLabel(rule); got != want {
			t.Fatalf("RecurLabel(%q) = %q, want %q", rule, got, want)
		}
	}
}

// 반복 카드를 완료하면 완료 칸에 남고, 다음 회차가 첫 칸에 새로 생긴다.
func TestRecurringCardSpawnsNextOnComplete(t *testing.T) {
	st, col, by := searchStore(t)
	ctx := context.Background()
	d, _ := st.GetBoard(ctx, 1, SortManual, Asc)
	done := d.Columns[len(d.Columns)-1].ID

	title, due, rule := "관리비 내기", "2026-01-15", "monthly:15"
	c, err := st.CreateCard(ctx, col, CardInput{Title: &title, DueAt: &due, Recur: &rule}, by)
	if err != nil {
		t.Fatal(err)
	}
	if c.Recur == nil || c.RecurLabel != "매월 15일" {
		t.Fatalf("규칙이 안 붙었다: %+v", c)
	}

	moved, err := st.MoveCard(ctx, c.ID, done, 0)
	if err != nil {
		t.Fatal(err)
	}
	// 완료한 카드는 완료 칸에 그대로 남는다 — 사라지면 완료가 안 된 줄 안다.
	if moved.ColumnID != done {
		t.Fatalf("완료 칸에 없다: %+v", moved)
	}
	if moved.Recur != nil {
		t.Fatal("지나간 회차에 규칙이 남아 있다")
	}

	d, _ = st.GetBoard(ctx, 1, SortManual, Asc)
	var next *Card
	for i := range d.Columns[0].Cards {
		if d.Columns[0].Cards[i].Title == title {
			next = &d.Columns[0].Cards[i]
		}
	}
	if next == nil {
		t.Fatal("다음 회차가 안 생겼다")
	}
	if next.DueAt == nil || *next.DueAt != "2026-02-15" {
		t.Fatalf("다음 마감 = %v, 2026-02-15 이어야 한다", next.DueAt)
	}
	if next.Recur == nil || *next.Recur != rule {
		t.Fatal("다음 회차가 규칙을 못 물려받았다")
	}
	if next.RecurParentID == nil || *next.RecurParentID != c.ID {
		t.Fatalf("부모 연결이 없다: %v", next.RecurParentID)
	}
}

// 종료일을 넘으면 더 만들지 않는다.
func TestRecurStopsAtUntil(t *testing.T) {
	st, col, by := searchStore(t)
	ctx := context.Background()
	d, _ := st.GetBoard(ctx, 1, SortManual, Asc)
	done := d.Columns[len(d.Columns)-1].ID

	title, due, rule, until := "끝나는 반복", "2026-01-15", "monthly:15", "2026-01-31"
	c, err := st.CreateCard(ctx, col, CardInput{Title: &title, DueAt: &due, Recur: &rule, RecurUntil: &until}, by)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.MoveCard(ctx, c.ID, done, 0); err != nil {
		t.Fatal(err)
	}
	d, _ = st.GetBoard(ctx, 1, SortManual, Asc)
	for _, x := range d.Columns[0].Cards {
		if x.Title == title {
			t.Fatalf("종료일을 넘었는데 다음 회차가 생겼다: %v", x.DueAt)
		}
	}
}

// 반복이 아닌 카드는 예전과 똑같이 동작해야 한다.
func TestPlainCardCompletesNormally(t *testing.T) {
	st, col, by := searchStore(t)
	ctx := context.Background()
	d, _ := st.GetBoard(ctx, 1, SortManual, Asc)
	done := d.Columns[len(d.Columns)-1].ID

	title, due := "한 번만", "2026-01-15"
	c, _ := st.CreateCard(ctx, col, CardInput{Title: &title, DueAt: &due}, by)
	if _, err := st.MoveCard(ctx, c.ID, done, 0); err != nil {
		t.Fatal(err)
	}
	d, _ = st.GetBoard(ctx, 1, SortManual, Asc)
	if n := len(d.Columns[0].Cards); n != 0 {
		t.Fatalf("첫 칸에 %d장이 남았다", n)
	}
}

// 마감 없는 반복은 다음 회차를 구할 기준이 없다.
func TestRecurNeedsDue(t *testing.T) {
	st, col, by := searchStore(t)
	title, rule := "마감 없음", "daily"
	if _, err := st.CreateCard(context.Background(), col, CardInput{Title: &title, Recur: &rule}, by); err == nil {
		t.Fatal("마감 없이 반복이 통과했다")
	}
}

// 화면은 '매주/매월/매년'만 보낸다. 값은 마감에서 끌어온다.
func TestExpandRecurFromDue(t *testing.T) {
	cases := []struct{ rule, due, want string }{
		{"daily", "2026-08-31", "daily"},
		{"weekly", "2026-08-31", "weekly:1"},  // 월요일 → bit0
		{"weekly", "2026-09-06", "weekly:64"}, // 일요일 → bit6
		{"monthly", "2026-08-31", "monthly:31"},
		{"yearly", "2026-04-07", "yearly:04-07"},
		{"weekly:5", "2026-08-31", "weekly:5"}, // 이미 값이 있으면 그대로
		{"", "2026-08-31", ""},
	}
	for _, c := range cases {
		got, err := ExpandRecur(c.rule, c.due)
		if err != nil || got != c.want {
			t.Fatalf("ExpandRecur(%q, %q) = %q, %v — want %q", c.rule, c.due, got, err, c.want)
		}
	}
	if _, err := ExpandRecur("weekly", ""); err == nil {
		t.Fatal("마감 없이 통과했다")
	}
}

// 화면에서 '매월'만 눌러도 저장 후 다음 회차가 제대로 나와야 한다.
func TestUpdateCardExpandsRecur(t *testing.T) {
	st, col, by := searchStore(t)
	ctx := context.Background()
	title, due := "관리비", "2026-01-15"
	c, _ := st.CreateCard(ctx, col, CardInput{Title: &title, DueAt: &due}, by)

	bare := "monthly"
	got, err := st.UpdateCard(ctx, c.ID, CardInput{Recur: &bare})
	if err != nil {
		t.Fatal(err)
	}
	if got.Recur == nil || *got.Recur != "monthly:15" {
		t.Fatalf("규칙 = %v, monthly:15 여야 한다", got.Recur)
	}
	if got.RecurLabel != "매월 15일" {
		t.Fatalf("설명 = %q", got.RecurLabel)
	}
}
