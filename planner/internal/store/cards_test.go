package store

import (
	"context"
	"testing"
)

func openTest(t *testing.T) (*Store, int64, []int64) {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	ctx := context.Background()
	u, err := st.CreateUser(ctx, "테스트1", "pass1234")
	if err != nil {
		t.Fatal(err)
	}
	boards, err := st.ListBoards(ctx)
	if err != nil || len(boards) != 1 {
		t.Fatalf("seed board missing: %v %d", err, len(boards))
	}
	d, err := st.GetBoard(ctx, boards[0].ID, SortManual, Asc)
	if err != nil {
		t.Fatal(err)
	}
	cols := make([]int64, len(d.Columns))
	for i, c := range d.Columns {
		cols[i] = c.ID
	}
	return st, u.ID, cols
}

func titles(t *testing.T, st *Store, col int64) []string {
	t.Helper()
	d, err := st.GetBoard(context.Background(), 1, SortManual, Asc)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range d.Columns {
		if c.ID != col {
			continue
		}
		out := make([]string, len(c.Cards))
		for i, card := range c.Cards {
			out[i] = card.Title
			if card.Position != i {
				t.Errorf("column %d: card %q has position %d, want %d (positions must be dense)", col, card.Title, card.Position, i)
			}
		}
		return out
	}
	t.Fatalf("column %d not found", col)
	return nil
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func seed(t *testing.T, st *Store, col, by int64, names ...string) []int64 {
	t.Helper()
	ids := make([]int64, len(names))
	for i, n := range names {
		n := n
		c, err := st.CreateCard(context.Background(), col, CardInput{Title: &n}, by)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = c.ID
	}
	return ids
}

func TestMoveSameColumnDown(t *testing.T) {
	st, by, cols := openTest(t)
	ids := seed(t, st, cols[0], by, "a", "b", "c", "d")
	// a b c d → move a to index 2 → b c a d
	if _, err := st.MoveCard(context.Background(), ids[0], cols[0], 2); err != nil {
		t.Fatal(err)
	}
	eq(t, titles(t, st, cols[0]), []string{"b", "c", "a", "d"})
}

func TestMoveSameColumnUp(t *testing.T) {
	st, by, cols := openTest(t)
	ids := seed(t, st, cols[0], by, "a", "b", "c", "d")
	// a b c d → move d to index 1 → a d b c
	if _, err := st.MoveCard(context.Background(), ids[3], cols[0], 1); err != nil {
		t.Fatal(err)
	}
	eq(t, titles(t, st, cols[0]), []string{"a", "d", "b", "c"})
}

func TestMoveSamePositionIsNoop(t *testing.T) {
	st, by, cols := openTest(t)
	ids := seed(t, st, cols[0], by, "a", "b", "c")
	if _, err := st.MoveCard(context.Background(), ids[1], cols[0], 1); err != nil {
		t.Fatal(err)
	}
	eq(t, titles(t, st, cols[0]), []string{"a", "b", "c"})
}

func TestMoveAcrossColumnsMiddle(t *testing.T) {
	st, by, cols := openTest(t)
	src := seed(t, st, cols[0], by, "a", "b", "c")
	seed(t, st, cols[1], by, "x", "y")
	// move b into column 1 at index 1 → x b y ; source → a c
	if _, err := st.MoveCard(context.Background(), src[1], cols[1], 1); err != nil {
		t.Fatal(err)
	}
	eq(t, titles(t, st, cols[0]), []string{"a", "c"})
	eq(t, titles(t, st, cols[1]), []string{"x", "b", "y"})
}

func TestMoveIntoEmptyColumn(t *testing.T) {
	st, by, cols := openTest(t)
	src := seed(t, st, cols[0], by, "a")
	if _, err := st.MoveCard(context.Background(), src[0], cols[2], 5); err != nil {
		t.Fatal(err)
	}
	eq(t, titles(t, st, cols[0]), []string{})
	eq(t, titles(t, st, cols[2]), []string{"a"})
}

func TestMoveClampsPosition(t *testing.T) {
	st, by, cols := openTest(t)
	src := seed(t, st, cols[0], by, "a")
	seed(t, st, cols[1], by, "x", "y")
	// index 99 → end; index -3 → start
	if _, err := st.MoveCard(context.Background(), src[0], cols[1], 99); err != nil {
		t.Fatal(err)
	}
	eq(t, titles(t, st, cols[1]), []string{"x", "y", "a"})
	if _, err := st.MoveCard(context.Background(), src[0], cols[1], -3); err != nil {
		t.Fatal(err)
	}
	eq(t, titles(t, st, cols[1]), []string{"a", "x", "y"})
}

func TestMoveToEndOfSameColumn(t *testing.T) {
	st, by, cols := openTest(t)
	ids := seed(t, st, cols[0], by, "a", "b", "c")
	// a b c → move a to the end (index 2 after removal) → b c a
	if _, err := st.MoveCard(context.Background(), ids[0], cols[0], 2); err != nil {
		t.Fatal(err)
	}
	eq(t, titles(t, st, cols[0]), []string{"b", "c", "a"})
}

func TestDeleteCompacts(t *testing.T) {
	st, by, cols := openTest(t)
	ids := seed(t, st, cols[0], by, "a", "b", "c")
	if err := st.DeleteCard(context.Background(), ids[1]); err != nil {
		t.Fatal(err)
	}
	eq(t, titles(t, st, cols[0]), []string{"a", "c"})
}

func TestMoveRejectsOtherBoard(t *testing.T) {
	st, by, cols := openTest(t)
	ids := seed(t, st, cols[0], by, "a")
	other, err := st.CreateBoard(context.Background(), "다른 보드", by)
	if err != nil {
		t.Fatal(err)
	}
	od, err := st.GetBoard(context.Background(), other.ID, SortManual, Asc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.MoveCard(context.Background(), ids[0], od.Columns[0].ID, 0); err != ErrNotFound {
		t.Fatalf("cross-board move should be rejected, got %v", err)
	}
	eq(t, titles(t, st, cols[0]), []string{"a"})
}

func TestDeleteColumnCompacts(t *testing.T) {
	st, _, cols := openTest(t)
	if err := st.DeleteColumn(context.Background(), cols[1]); err != nil {
		t.Fatal(err)
	}
	d, err := st.GetBoard(context.Background(), 1, SortManual, Asc)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Columns) != 2 || d.Columns[0].Position != 0 || d.Columns[1].Position != 1 {
		t.Fatalf("columns not compacted: %+v", d.Columns)
	}
}

// 홈의 '오늘 할 일'은 이름 그대로여야 한다. 칸 전체를 주면 다음 달 마감까지
// 섞여 들어와 5줄 제한에 오늘 것이 밀려나고, 바로 아래 '앞으로 7일'에만
// 보이는 모순이 생긴다.
func TestTodoCardsStopsAtToday(t *testing.T) {
	st, by, cols := openTest(t)
	ctx := context.Background()

	mk := func(title, due string) {
		var d *string
		if due != "" {
			d = &due
		}
		if _, err := st.CreateCard(ctx, cols[0], CardInput{Title: &title, DueAt: d}, by); err != nil {
			t.Fatal(err)
		}
	}
	mk("밀린 것", "2026-09-23")
	mk("오늘 것", "2026-09-24")
	mk("오늘 오후", "2026-09-24T13:00")
	mk("내일 것", "2026-09-25")
	mk("내일 아침", "2026-09-25T09:00")
	mk("다음 달", "2026-10-20")
	mk("마감 없음", "")

	got, err := st.TodoCards(ctx, "2026-09-24", SortTime, Asc)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, c := range got {
		have[c.Title] = true
	}
	for _, want := range []string{"밀린 것", "오늘 것", "오늘 오후", "마감 없음"} {
		if !have[want] {
			t.Fatalf("%q 가 빠졌다: %v", want, have)
		}
	}
	for _, no := range []string{"내일 것", "내일 아침", "다음 달"} {
		if have[no] {
			t.Fatalf("%q 는 오늘 할 일이 아니다", no)
		}
	}
}
