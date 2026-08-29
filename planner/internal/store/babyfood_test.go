package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// 픽스처는 전부 지어낸 것이다. 실제 식단 데이터는 저장소에 두지 않는다.
const testPlanJSON = `{
 "schema": 1,
 "default_track": ["s1", "s2"],
 "plans": [
  {"id":"s1","label":"1구간","kind":"topping","from":100,"to":102,"days":[
    {"d":100,"new":"가재료","meals":[
      {"slot":"아침","base":"베이스A","toppings":["가재료","나재료"],"snack":null}]},
    {"d":101,"meals":[
      {"slot":"아침","base":"베이스A","toppings":["가재료","나재료"],"snack":"다재료"}]},
    {"d":102,"meals":[
      {"slot":"아침","base":"베이스A","toppings":["가재료"],"snack":null},
      {"slot":"점심","base":"베이스B","toppings":["나재료","다재료"],"snack":null}]}
  ]},
  {"id":"s2","label":"2구간","kind":"menu","from":103,"to":103,"days":[
    {"d":103,"meals":[{"slot":"아침","base":"어떤요리","toppings":[],"snack":null}]}
  ]},
  {"id":"alt","label":"대체","kind":"topping","from":100,"to":100,"days":[
    {"d":100,"meals":[{"slot":"아침","base":"베이스C","toppings":["라재료"],"snack":null}]}
  ]}
 ],
 "ingredients": [
  {"name":"베이스A","kind":"base"},{"name":"베이스B","kind":"base"},
  {"name":"가재료","kind":"cube"},{"name":"나재료","kind":"cube"},
  {"name":"다재료","kind":"cube"},{"name":"어떤요리","kind":"dish"}
 ]
}`

// newBFStore는 빈 DB와 고친 사람으로 기록될 사용자 id를 준다. 수정자·실사자는
// 실제 사용자를 참조하므로(FK) 지어낸 id로는 쓸 수 없다.
func newBFStore(t *testing.T) (*Store, int64) {
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

// freezeToday는 '오늘'을 고정한다. 재고 계산이 전부 오늘 기준이라
// 고정하지 않으면 테스트가 날짜에 따라 달라진다.
func freezeToday(t *testing.T, date string) {
	t.Helper()
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		t.Fatal(err)
	}
	prev := bfClock
	bfClock = func() time.Time { return d }
	t.Cleanup(func() { bfClock = prev })
}

func seedBabyfood(t *testing.T, s *Store) *BFPlanFile {
	t.Helper()
	f, err := ParseBFPlan([]byte(testPlanJSON))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BFImport(context.Background(), f, nil, true, false, 0); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestBFImportIsDryRunByDefault(t *testing.T) {
	s, _ := newBFStore(t)
	f, err := ParseBFPlan([]byte(testPlanJSON))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := s.BFImport(context.Background(), f, nil, false, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if plan.NewDays != 4 {
		t.Fatalf("dry-run이 4일을 세야 하는데 %d", plan.NewDays)
	}
	days, err := s.BFRange(context.Background(), 0, 400)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 0 {
		t.Fatalf("dry-run인데 %d일이 들어갔다", len(days))
	}
}

func TestBFImportSkipsExistingDays(t *testing.T) {
	s, uid := newBFStore(t)
	ctx := context.Background()
	f := seedBabyfood(t, s)

	// 손으로 고친 뒤 다시 가져오면 그 날은 건드리지 않아야 한다.
	days, err := s.BFRange(ctx, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	tops := []string{"바꾼재료"}
	if _, err := s.BFUpdateMeal(ctx, days[0].Meals[0].ID, BFMealInput{Toppings: &tops}, uid); err != nil {
		t.Fatal(err)
	}
	plan, err := s.BFImport(ctx, f, nil, true, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if plan.NewDays != 0 || plan.Existing != 4 {
		t.Fatalf("두 번째 가져오기: new=%d existing=%d", plan.NewDays, plan.Existing)
	}
	days, err = s.BFRange(ctx, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got := days[0].Meals[0].Toppings; len(got) != 1 || got[0] != "바꾼재료" {
		t.Fatalf("수정이 덮어써졌다: %v", got)
	}
}

func TestBFEditKeepsOriginalAndResetRestoresIt(t *testing.T) {
	s, uid := newBFStore(t)
	ctx := context.Background()
	seedBabyfood(t, s)

	days, _ := s.BFRange(ctx, 101, 101)
	m := days[0].Meals[0]
	if m.Edited {
		t.Fatal("갓 넣은 끼니가 '수정됨'이다")
	}

	snack := "" // 간식을 지운다
	tops := []string{"나재료", "가재료"}
	day, err := s.BFUpdateMeal(ctx, m.ID, BFMealInput{Toppings: &tops, Snack: &snack}, uid)
	if err != nil {
		t.Fatal(err)
	}
	got := day.Meals[0]
	if !got.Edited {
		t.Fatal("고쳤는데 '수정됨'이 아니다")
	}
	if got.Snack != nil {
		t.Fatalf("간식이 안 지워졌다: %v", *got.Snack)
	}
	// 원본은 그대로여야 "원래 …"를 보여줄 수 있다.
	if len(got.Src.Toppings) != 2 || got.Src.Toppings[0] != "가재료" {
		t.Fatalf("원본이 바뀌었다: %v", got.Src.Toppings)
	}
	if got.Src.Snack == nil || *got.Src.Snack != "다재료" {
		t.Fatalf("원본 간식이 바뀌었다: %v", got.Src.Snack)
	}
	if got.EditedBy == nil || *got.EditedBy != uid {
		t.Fatalf("누가 고쳤는지 기록이 없다: %v", got.EditedBy)
	}

	day, err = s.BFUpdateMeal(ctx, m.ID, BFMealInput{Reset: true}, uid)
	if err != nil {
		t.Fatal(err)
	}
	back := day.Meals[0]
	if back.Edited {
		t.Fatal("되돌렸는데 '수정됨'이 남아 있다")
	}
	if back.Snack == nil || *back.Snack != "다재료" {
		t.Fatal("되돌렸는데 간식이 안 돌아왔다")
	}
	if back.EditedBy != nil {
		t.Fatal("되돌렸는데 수정자가 남아 있다")
	}
}

func TestBFSelectTrackRejectsOverlap(t *testing.T) {
	f, err := ParseBFPlan([]byte(testPlanJSON))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BFSelectTrack(f, []string{"s1", "alt"}); err == nil {
		t.Fatal("같은 날을 두 구간이 채우는데 통과했다")
	}
	if _, err := BFSelectTrack(f, []string{"없는구간"}); err == nil {
		t.Fatal("없는 구간인데 통과했다")
	}
}

// 재고 계산의 핵심. 실사 이후의 소모와 제조만 반영되고, 오늘 것은 '소모'가
// 아니라 '필요'로 잡혀야 한다 — 겹치면 오늘치가 두 번 빠진다.
func TestBFStockLedger(t *testing.T) {
	s, uid := newBFStore(t)
	ctx := context.Background()
	seedBabyfood(t, s)

	// 생일을 D+100 이 2026-01-01 이 되게 잡고, 오늘을 D+102 로 고정한다.
	birth := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -100).Format("2006-01-02")
	freezeToday(t, "2026-01-03") // D+102
	if _, err := s.BFProfileSet(ctx, BFProfileInput{BirthDate: &birth}); err != nil {
		t.Fatal(err)
	}

	view, err := s.BFStockList(ctx, 3) // D+102 ~ D+104
	if err != nil {
		t.Fatal(err)
	}
	if view.FromDDay != 102 || view.ToDDay != 104 {
		t.Fatalf("기간이 이상하다: %d~%d", view.FromDDay, view.ToDDay)
	}
	byName := map[string]BFStock{}
	for _, it := range view.Items {
		byName[it.Name] = it
	}
	// D+102 만 topping 구간이다(103은 menu라 제외). 가재료 1, 나재료 1, 다재료 1.
	if got := byName["가재료"].Need; got != 1 {
		t.Fatalf("가재료 필요 = %d, 1이어야 한다", got)
	}
	if got := byName["어떤요리"].Need; got != 0 {
		t.Fatalf("menu 구간이 재고 계산에 들어왔다: %d", got)
	}
	// 실사 전에는 재고를 '모름'으로 둔다 — 0으로 단정하면 있는 걸 또 만든다.
	if byName["가재료"].Stock != nil {
		t.Fatal("실사도 안 했는데 재고가 숫자다")
	}
	if byName["가재료"].Make != 1 {
		t.Fatalf("실사 전 제조필요는 필요량 그대로여야 한다: %d", byName["가재료"].Make)
	}

	// 어제(D+101) 기준으로 10개 있었다고 실사 → 오늘(D+102) 소모는 아직 안 뺀다.
	// 소모 구간은 '실사 다음날 ~ 어제'이므로 D+102 는 빠지지 않아야 한다.
	if err := s.BFCount(ctx, "가재료", 10, uid); err != nil {
		t.Fatal(err)
	}
	view, err = s.BFStockList(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range view.Items {
		if it.Name != "가재료" {
			continue
		}
		if it.Stock == nil || *it.Stock != 10 {
			t.Fatalf("실사 직후 재고는 10이어야 한다: %v", it.Stock)
		}
		if it.Used != 0 {
			t.Fatalf("오늘 것이 소모로 빠졌다: used=%d", it.Used)
		}
		if it.Make != 0 {
			t.Fatalf("10개 있는데 제조필요 %d", it.Make)
		}
	}

	// 제조 기록은 실사 이후의 것만 더해진다.
	if err := s.BFAddBatch(ctx, "가재료", 5, "만듦", uid); err != nil {
		t.Fatal(err)
	}
	view, _ = s.BFStockList(ctx, 3)
	for _, it := range view.Items {
		if it.Name == "가재료" {
			if it.Made != 5 || it.Stock == nil || *it.Stock != 15 {
				t.Fatalf("제조 반영 실패: made=%d stock=%v", it.Made, it.Stock)
			}
		}
	}
}

// 건너뛴 끼니는 먹지 않았으므로 재고에서 빼면 안 된다.
func TestBFSkippedMealIsNotConsumed(t *testing.T) {
	s, uid := newBFStore(t)
	ctx := context.Background()
	seedBabyfood(t, s)
	birth := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -100).Format("2006-01-02")
	freezeToday(t, "2026-01-03") // D+102
	if _, err := s.BFProfileSet(ctx, BFProfileInput{BirthDate: &birth}); err != nil {
		t.Fatal(err)
	}
	// D+100 에 실사 0 → D+101 소모가 그대로 빠진다.
	if err := s.BFCount(ctx, "나재료", 0, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE bf_ingredients SET count_dday=100 WHERE name='나재료'`); err != nil {
		t.Fatal(err)
	}
	view, _ := s.BFStockList(ctx, 1)
	var before int
	for _, it := range view.Items {
		if it.Name == "나재료" {
			before = it.Used
		}
	}
	if before != 1 {
		t.Fatalf("D+101 소모가 1이어야 한다: %d", before)
	}

	days, _ := s.BFRange(ctx, 101, 101)
	skip := true
	if _, err := s.BFUpdateMeal(ctx, days[0].Meals[0].ID, BFMealInput{Skipped: &skip}, uid); err != nil {
		t.Fatal(err)
	}
	view, _ = s.BFStockList(ctx, 1)
	for _, it := range view.Items {
		if it.Name == "나재료" && it.Used != 0 {
			t.Fatalf("건너뛴 끼니가 소모로 잡혔다: %d", it.Used)
		}
	}
}

func TestBFStockNeedsBirthDate(t *testing.T) {
	s, _ := newBFStore(t)
	seedBabyfood(t, s)
	if _, err := s.BFStockList(context.Background(), 21); err == nil {
		t.Fatal("생일 없이 재고가 계산됐다")
	}
}

func TestBFSrcSurvivesJSONRoundTrip(t *testing.T) {
	s, _ := newBFStore(t)
	seedBabyfood(t, s)
	days, err := s.BFRange(context.Background(), 102, 102)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(days[0])
	if err != nil {
		t.Fatal(err)
	}
	var back BFDay
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Meals) != 2 || back.Meals[1].Base != "베이스B" {
		t.Fatalf("왕복 후 달라졌다: %s", b)
	}
}

// 중간에 구성을 바꿀 때(두 끼 → 세 끼) 지나간 날은 건드리지 않아야 한다.
func TestBFImportFromDDayOnly(t *testing.T) {
	s, uid := newBFStore(t)
	ctx := context.Background()
	f := seedBabyfood(t, s)

	days, _ := s.BFRange(ctx, 100, 100)
	tops := []string{"손으로고친것"}
	if _, err := s.BFUpdateMeal(ctx, days[0].Meals[0].ID, BFMealInput{Toppings: &tops}, uid); err != nil {
		t.Fatal(err)
	}

	// D+101 부터만, 덮어쓰기로 다시 넣는다.
	plan, err := s.BFImport(ctx, f, nil, true, true, 101)
	if err != nil {
		t.Fatal(err)
	}
	if plan.NewDays != 3 { // 101, 102, 103
		t.Fatalf("D+101부터면 3일이어야 한다: %d", plan.NewDays)
	}
	days, _ = s.BFRange(ctx, 100, 100)
	if got := days[0].Meals[0].Toppings; len(got) != 1 || got[0] != "손으로고친것" {
		t.Fatalf("지나간 날이 덮어써졌다: %v", got)
	}
}

// 재고에서 장보기 카드로 넘어가는 길. 카드 본문은 서버가 만들므로
// 편집기가 받아들이는 형식인지도 같이 본다.
func TestBFShoppingCard(t *testing.T) {
	s, uid := newBFStore(t)
	ctx := context.Background()
	seedBabyfood(t, s)
	birth := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -100).Format("2006-01-02")
	freezeToday(t, "2026-01-03") // D+102
	if _, err := s.BFProfileSet(ctx, BFProfileInput{BirthDate: &birth}); err != nil {
		t.Fatal(err)
	}

	card, err := s.BFShoppingCard(ctx, 3, uid)
	if err != nil {
		t.Fatal(err)
	}
	if card.Title != "이유식 장보기" {
		t.Fatalf("제목 = %q", card.Title)
	}
	if card.DueAt == nil || *card.DueAt != "2026-01-03" {
		t.Fatalf("마감 = %v, 오늘이어야 한다", card.DueAt)
	}
	if card.Content == nil {
		t.Fatal("본문이 없다")
	}
	plain, err := ValidateContent(*card.Content)
	if err != nil {
		t.Fatalf("본문을 서버가 거절했다: %v", err)
	}
	for _, want := range []string{"가재료 1개", "나재료 1개"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("%q 가 본문에 없다: %q", want, plain)
		}
	}

	// 전부 채워두면 만들 게 없다 — 빈 카드를 만들지 않고 거절한다.
	for _, n := range []string{"가재료", "나재료", "다재료", "베이스A", "베이스B"} {
		if err := s.BFCount(ctx, n, 99, uid); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.BFShoppingCard(ctx, 3, uid); err == nil {
		t.Fatal("만들 게 없는데 카드를 만들었다")
	}
}
