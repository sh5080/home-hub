package store

import (
	"context"
	"testing"
)

func newDiaryStore(t *testing.T) (*Store, int64) {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	u, err := st.CreateUser(context.Background(), "테스트1", "testpass1")
	if err != nil {
		t.Fatal(err)
	}
	return st, u.ID
}

func TestDiaryWritesAndFinds(t *testing.T) {
	s, uid := newDiaryStore(t)
	ctx := context.Background()

	e, err := s.DiaryCreate(ctx, "2026-09-26", "", uid)
	if err != nil {
		t.Fatal(err)
	}
	if e.Date != "2026-09-26" || e.CreatedBy == nil || *e.CreatedBy != uid {
		t.Fatalf("만들어진 글이 이상하다: %+v", e)
	}

	// 본문은 카드와 같은 블록 형식이고, 평문이 따로 뽑힌다.
	body := PlainToContent("오늘은 단아가 완두콩을 처음 먹었다")
	title := "완두콩 첫날"
	e, err = s.DiaryUpdate(ctx, e.ID, DiaryInput{Title: &title, Content: &body})
	if err != nil {
		t.Fatal(err)
	}
	if e.Plain == "" {
		t.Fatal("평문이 안 뽑혔다")
	}

	// 참조는 통째로 바뀐다.
	refs := []DiaryRef{{Kind: "babyfood", RefDate: "2026-09-26", Label: "단아 · 9/26 식단"}}
	e, err = s.DiaryUpdate(ctx, e.ID, DiaryInput{Refs: &refs})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Refs) != 1 || e.Refs[0].Label != "단아 · 9/26 식단" {
		t.Fatalf("참조가 안 붙었다: %+v", e.Refs)
	}

	// 지워진 카드를 가리키면 링크는 죽고 이름만 남는다.
	var missing int64 = 999999
	refs = []DiaryRef{{Kind: "card", RefID: &missing, Label: "없어진 카드"}}
	e, _ = s.DiaryUpdate(ctx, e.ID, DiaryInput{Refs: &refs})
	if len(e.Refs) != 1 || !e.Refs[0].Gone {
		t.Fatalf("사라진 대상이 표시되지 않았다: %+v", e.Refs)
	}

	// 목록은 본문 대신 미리보기만 싣는다.
	list, err := s.DiaryList(ctx, "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Content != nil {
		t.Fatalf("목록에 본문이 실렸다: %+v", list[0])
	}
	if len(list[0].Refs) != 1 {
		t.Fatalf("목록에 참조가 안 붙었다: %+v", list[0].Refs)
	}

	// 검색은 제목과 본문 평문을 본다.
	found, err := s.SearchDiary(ctx, "완두콩")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("일기를 못 찾았다: %+v", found)
	}
	if all, _ := s.SearchAll(ctx, "완두콩"); len(all.Diary) != 1 {
		t.Fatal("통합 검색에 일기가 빠졌다")
	}

	// 날짜 목록은 캘린더의 점이다.
	dates, err := s.DiaryDates(ctx, "2026-09-01", "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	if len(dates) != 1 || dates[0] != "2026-09-26" {
		t.Fatalf("날짜 = %v", dates)
	}

	if err := s.DiaryDelete(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DiaryGet(ctx, e.ID); err == nil {
		t.Fatal("지웠는데 읽힌다")
	}
}

// 가져오기는 출처 키로 중복을 막는다. 같은 내보내기를 두 번 돌려도 한 번만.
func TestDiaryImportSkipsDuplicates(t *testing.T) {
	s, uid := newDiaryStore(t)
	ctx := context.Background()
	es := []DiaryImportEntry{
		{Source: "babytime:25_1", Date: "2026-03-29", Content: PlainToContent("첫째 글")},
		{Source: "babytime:25_2", Date: "2026-03-29", Content: PlainToContent("둘째 글")},
	}
	added, skipped, err := s.DiaryImport(ctx, es, uid)
	if err != nil || added != 2 || skipped != 0 {
		t.Fatalf("첫 실행 = %d/%d, %v", added, skipped, err)
	}
	added, skipped, err = s.DiaryImport(ctx, es, uid)
	if err != nil || added != 0 || skipped != 2 {
		t.Fatalf("두 번째 실행 = %d/%d, %v", added, skipped, err)
	}
	list, _ := s.DiaryList(ctx, "", "", 0)
	if len(list) != 2 || list[0].Source == nil {
		t.Fatalf("목록 = %d장, source=%v", len(list), list[0].Source)
	}
}

// 더 보기는 (날짜, id) 커서로 이어진다. 같은 날 여러 장이 있어도 겹치거나
// 빠지지 않는다.
func TestDiaryListPagesByCursor(t *testing.T) {
	s, uid := newDiaryStore(t)
	ctx := context.Background()
	for _, d := range []string{"2026-05-01", "2026-05-02", "2026-05-02", "2026-05-03", "2026-05-03"} {
		if _, err := s.DiaryCreate(ctx, d, "", uid); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[int64]bool{}
	var cur DiaryPage
	cur.Limit = 2
	for round := 0; round < 5; round++ {
		page, err := s.DiaryListPage(ctx, cur)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		for _, e := range page {
			if seen[e.ID] {
				t.Fatalf("%d 가 두 번 나왔다", e.ID)
			}
			seen[e.ID] = true
		}
		last := page[len(page)-1]
		cur.BeforeDate, cur.BeforeID = last.Date, last.ID
	}
	if len(seen) != 5 {
		t.Fatalf("다섯 장을 다 못 봤다: %d", len(seen))
	}
}
