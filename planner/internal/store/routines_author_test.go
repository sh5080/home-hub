package store

import (
	"context"
	"testing"
)

// 가족이 같이 쓰는 앱에서 "이거 누가 만들었어"를 물을 곳이 있어야 한다.
func TestRoutineRecordsAuthor(t *testing.T) {
	st, by, _ := openTest(t)
	ctx := context.Background()

	title, mask := "저녁 산책", 127
	r, err := st.CreateRoutine(ctx, RoutineInput{Title: &title, WeekdaysMask: &mask}, by)
	if err != nil {
		t.Fatal(err)
	}
	if r.CreatedBy == nil || *r.CreatedBy != by {
		t.Fatalf("만든 사람 = %v, want %d", r.CreatedBy, by)
	}
	if r.CreatedAt == 0 {
		t.Fatal("만든 시각이 비어 있다")
	}

	// 목록·날짜별 조회에서도 같이 따라와야 화면에서 쓸 수 있다.
	list, err := st.ListRoutines(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range list {
		if x.ID == r.ID && (x.CreatedBy == nil || *x.CreatedBy != by) {
			t.Fatalf("목록에서 만든 사람이 빠졌다: %+v", x)
		}
	}
	day, err := st.RoutinesForDate(ctx, "2026-09-25")
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range day {
		if x.ID == r.ID && x.CreatedBy == nil {
			t.Fatal("날짜별 조회에서 만든 사람이 빠졌다")
		}
	}
}
