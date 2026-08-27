package store

import (
	"context"
	"testing"
)

// 정렬 기준 세 가지가 실제로 다른 순서를 내는지, 그리고 각 기준의 동점자
// 처리가 의도대로인지.
func TestSortOrders(t *testing.T) {
	st, by, cols := openTest(t)
	ctx := context.Background()
	col := cols[0]

	mk := func(title, due string, prio int) {
		in := CardInput{Title: &title, Priority: &prio}
		if due != "" {
			in.DueAt = &due
		}
		if _, err := st.CreateCard(ctx, col, in, by); err != nil {
			t.Fatalf("%s: %v", title, err)
		}
	}
	// 만든 순서 = 수동 순서
	mk("마감없음-별3", "", 3)
	mk("내일-별1", "2026-09-24", 1)
	mk("오늘아침-별0", "2026-09-23T09:00", 0)
	mk("오늘저녁-별2", "2026-09-23T19:00", 2)

	names := func(s Sort) []string {
		d, err := st.GetBoard(ctx, 1, s, Asc)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, c := range d.Columns[0].Cards {
			out = append(out, c.Title)
		}
		return out
	}

	eq(t, names(SortManual), []string{"마감없음-별3", "내일-별1", "오늘아침-별0", "오늘저녁-별2"})
	// 시간순: 날짜 있는 것 먼저, 같은 날이면 시각순. 마감 없는 건 맨 뒤.
	eq(t, names(SortTime), []string{"오늘아침-별0", "오늘저녁-별2", "내일-별1", "마감없음-별3"})
	// 중요도순: 별 많은 것 먼저. 동점이면 마감 빠른 순.
	eq(t, names(SortPriority), []string{"마감없음-별3", "오늘저녁-별2", "내일-별1", "오늘아침-별0"})
}

// 마감에 시각이 들어가도 날짜 경계 범위 쿼리가 그대로 동작해야 한다.
// 이게 깨지면 캘린더와 /api/today가 조용히 항목을 놓친다.
func TestDatetimeDueStillMatchesDateRange(t *testing.T) {
	st, by, cols := openTest(t)
	ctx := context.Background()

	title, due := "오후 진료", "2026-09-23T14:00"
	if _, err := st.CreateCard(ctx, cols[0], CardInput{Title: &title, DueAt: &due}, by); err != nil {
		t.Fatal(err)
	}
	got, err := st.CalendarCards(ctx, "2026-09-23", "2026-09-24")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != title {
		t.Fatalf("시각 있는 마감이 날짜 범위에 안 잡힘: %+v", got)
	}
	// 하루 전 범위에는 안 잡혀야 한다.
	if got, _ := st.CalendarCards(ctx, "2026-09-22", "2026-09-23"); len(got) != 0 {
		t.Fatalf("범위 밖인데 잡힘: %+v", got)
	}
}

func TestDueAcceptsBothFormatsAndRejectsGarbage(t *testing.T) {
	st, by, cols := openTest(t)
	ctx := context.Background()
	for _, due := range []string{"2026-09-23", "2026-09-23T14:00"} {
		title := "ok " + due
		if _, err := st.CreateCard(ctx, cols[0], CardInput{Title: &title, DueAt: &due}, by); err != nil {
			t.Errorf("%q 거부됨: %v", due, err)
		}
	}
	for _, due := range []string{"2026-09-23 14:00", "26-09-23", "2026-13-01", "내일"} {
		title := "bad"
		if _, err := st.CreateCard(ctx, cols[0], CardInput{Title: &title, DueAt: &due}, by); err == nil {
			t.Errorf("%q 는 거부돼야 함", due)
		}
	}
}

func TestPriorityClamped(t *testing.T) {
	st, by, cols := openTest(t)
	ctx := context.Background()
	for in, want := range map[int]int{-5: 0, 0: 0, 2: 2, 3: 3, 99: 3} {
		title, p := "p", in
		c, err := st.CreateCard(ctx, cols[0], CardInput{Title: &title, Priority: &p}, by)
		if err != nil {
			t.Fatal(err)
		}
		if c.Priority != want {
			t.Errorf("priority %d → %d, want %d", in, c.Priority, want)
		}
	}
}

// 노션에서 가져온 카드의 중요도가 본문 콜아웃에만 있었던 것을 컬럼으로 복구한다.
func TestPriorityBackfillFromCallout(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	u, _ := st.CreateUser(ctx, "테스트1", "pass1234")
	d, _ := st.GetBoard(ctx, 1, SortManual, Asc)
	col := d.Columns[0].ID

	content := `[{"type":"callout","props":{"emoji":"📥"},"content":[{"type":"text","text":"분류: 쇼핑 · 중요도: ⭐⭐ · 얼른","styles":{}}],"children":[]}]`
	title := "비데 구입"
	c, err := st.CreateCard(ctx, col, CardInput{Title: &title, Content: &content}, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.Priority != 0 {
		t.Fatalf("생성 시에는 0이어야 함: %d", c.Priority)
	}
	st.Close()

	// 재기동 시 복구된다.
	st, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.GetCard(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Priority != 2 {
		t.Fatalf("복구된 priority = %d, want 2", got.Priority)
	}
	// 콜아웃은 그대로 남아야 한다 — 분류와 메모가 거기 있다.
	if got.Content == nil || !contains(*got.Content, "분류: 쇼핑") {
		t.Fatal("콜아웃을 지우면 안 된다")
	}
	// 멱등: 다시 열어도 값이 안 바뀐다.
	st.Close()
	st, _ = Open(dir)
	if again, _ := st.GetCard(ctx, c.ID); again.Priority != 2 {
		t.Fatalf("멱등하지 않음: %d", again.Priority)
	}
}

func TestStarsAfter(t *testing.T) {
	cases := map[string]int{
		"분류: 쇼핑 · 중요도: ⭐⭐⭐ · 메모": 3,
		"중요도: ⭐":            1,
		"중요도: ⭐⭐⭐⭐⭐":        3, // maxPriority로 자른다
		"중요도: 없음":           0,
		"별 ⭐⭐ 은 있지만 표식이 없음": 0,
	}
	for in, want := range cases {
		if got := starsAfter(in, "중요도:"); got != want {
			t.Errorf("starsAfter(%q) = %d, want %d", in, got, want)
		}
	}
}

// 역순. 방향을 뒤집어도 "마감 없는 것은 뒤"는 유지돼야 한다 — 날짜 없는
// 카드가 맨 앞에 몰리면 목록을 읽을 수 없다.
func TestSortDescending(t *testing.T) {
	st, by, cols := openTest(t)
	ctx := context.Background()
	col := cols[0]

	mk := func(title, due string, prio int) {
		in := CardInput{Title: &title, Priority: &prio}
		if due != "" {
			in.DueAt = &due
		}
		if _, err := st.CreateCard(ctx, col, in, by); err != nil {
			t.Fatal(err)
		}
	}
	mk("마감없음-별3", "", 3)
	mk("내일-별1", "2026-09-24", 1)
	mk("오늘아침-별0", "2026-09-23T09:00", 0)
	mk("오늘저녁-별2", "2026-09-23T19:00", 2)

	names := func(s Sort, o Order) []string {
		d, err := st.GetBoard(ctx, 1, s, o)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, c := range d.Columns[0].Cards {
			out = append(out, c.Title)
		}
		return out
	}

	// 시간 역순: 늦은 마감이 먼저. 마감 없는 건 여전히 맨 뒤.
	eq(t, names(SortTime, Desc), []string{"내일-별1", "오늘저녁-별2", "오늘아침-별0", "마감없음-별3"})
	// 중요도 역순: 별 적은 것이 먼저.
	eq(t, names(SortPriority, Desc), []string{"오늘아침-별0", "내일-별1", "오늘저녁-별2", "마감없음-별3"})
	// 수동 역순: 만든 순서의 반대.
	eq(t, names(SortManual, Desc), []string{"오늘저녁-별2", "오늘아침-별0", "내일-별1", "마감없음-별3"})
	// 오름차순은 그대로.
	eq(t, names(SortTime, Asc), []string{"오늘아침-별0", "오늘저녁-별2", "내일-별1", "마감없음-별3"})
}

func TestParseOrder(t *testing.T) {
	for in, want := range map[string]Order{"desc": Desc, "asc": Asc, "": Asc, "DESC": Asc, "쓰레기": Asc} {
		if got := ParseOrder(in); got != want {
			t.Errorf("ParseOrder(%q) = %v, want %v", in, got, want)
		}
	}
}
