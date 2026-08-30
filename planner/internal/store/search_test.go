package store

import (
	"context"
	"strings"
	"testing"
)

func searchStore(t *testing.T) (*Store, int64, int64) {
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
	d, err := st.GetBoard(ctx, 1, SortManual, Asc)
	if err != nil {
		t.Fatal(err)
	}
	return st, d.Columns[0].ID, u.ID
}

func mkCard(t *testing.T, st *Store, col, by int64, title, body string) {
	t.Helper()
	c := PlainToContent(body)
	if _, err := st.CreateCard(context.Background(), col, CardInput{Title: &title, Content: &c}, by); err != nil {
		t.Fatal(err)
	}
}

func hitTitles(rs []SearchResult) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Title)
	}
	return out
}

func TestSearchFindsKoreanSubstring(t *testing.T) {
	st, col, by := searchStore(t)
	mkCard(t, st, col, by, "이유식 장보기", "소고기 21개 당근 21개")
	mkCard(t, st, col, by, "어린이집 상담", "담임 선생님 면담")

	// 두 글자 부분어. FTS5를 쓸 수 없던 이유가 정확히 이것이다.
	got, err := st.SearchCards(context.Background(), "고기")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "이유식 장보기" {
		t.Fatalf("'고기' → %v", hitTitles(got))
	}
	if got[0].Snippet == "" || !strings.Contains(got[0].Snippet, "소고기") {
		t.Fatalf("본문 조각이 비었다: %q", got[0].Snippet)
	}
	// 보드/칸까지 알려줘야 어디로 갈지 안다.
	if got[0].BoardName == "" || got[0].ColumnName == "" {
		t.Fatalf("위치 정보가 없다: %+v", got[0])
	}
}

func TestSearchTermsAreAnded(t *testing.T) {
	st, col, by := searchStore(t)
	mkCard(t, st, col, by, "장보기 (이유식)", "")
	mkCard(t, st, col, by, "이유식 식단 정리", "")

	// 어순이 달라도 찾아야 한다 — 통째로 비교하면 놓친다.
	got, _ := st.SearchCards(context.Background(), "이유식 장보기")
	if len(got) != 1 || got[0].Title != "장보기 (이유식)" {
		t.Fatalf("AND 검색 실패: %v", hitTitles(got))
	}
}

func TestSearchEscapesWildcards(t *testing.T) {
	st, col, by := searchStore(t)
	mkCard(t, st, col, by, "보고서 100% 완료", "")
	mkCard(t, st, col, by, "그냥 카드", "")

	// '%' 를 그대로 흘리면 모든 카드가 걸린다.
	if got, _ := st.SearchCards(context.Background(), "%"); len(got) != 1 {
		t.Fatalf("'%%' → %v, 리터럴로 다뤄야 한다", hitTitles(got))
	}
	if got, _ := st.SearchCards(context.Background(), "_"); len(got) != 0 {
		t.Fatalf("'_' → %v, 없어야 한다", hitTitles(got))
	}
	if got, _ := st.SearchCards(context.Background(), "100%"); len(got) != 1 {
		t.Fatalf("'100%%' → %v", hitTitles(got))
	}
}

func TestSearchIsCaseInsensitiveForAscii(t *testing.T) {
	st, col, by := searchStore(t)
	mkCard(t, st, col, by, "Costco 장보기", "")
	if got, _ := st.SearchCards(context.Background(), "costco"); len(got) != 1 {
		t.Fatalf("대소문자 무시 실패: %v", hitTitles(got))
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	st, col, by := searchStore(t)
	mkCard(t, st, col, by, "아무거나", "")
	if got, _ := st.SearchCards(context.Background(), "   "); len(got) != 0 {
		t.Fatalf("빈 질의에 %v 를 돌려줬다", hitTitles(got))
	}
}

func TestSearchDoesNotShipFullBody(t *testing.T) {
	st, col, by := searchStore(t)
	mkCard(t, st, col, by, "긴 카드", strings.Repeat("본문 ", 500))
	got, _ := st.SearchCards(context.Background(), "긴")
	if len(got) != 1 {
		t.Fatal("못 찾음")
	}
	if got[0].Content != nil {
		t.Fatal("목록에 본문 전체가 실려 있다")
	}
}
