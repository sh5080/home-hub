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

// newBFStore 는 빈 DB 와 (고친 사람 id, 아이 id)를 준다.
func newBFStore(t *testing.T) (*Store, int64, int64) {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "테스트1", "testpass1")
	if err != nil {
		t.Fatal(err)
	}
	// 아이는 D+100 이 2026-01-01 이 되도록 잡는다. 테스트들이 그 기준을 쓴다.
	kid, err := st.CreateUser(ctx, "테스트아기", "testpass2")
	if err != nil {
		t.Fatal(err)
	}
	birth := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -100).Format("2006-01-02")
	if _, err := st.SetBirthDate(ctx, kid.ID, birth); err != nil {
		t.Fatal(err)
	}
	if err := st.BFAddChild(ctx, kid.ID, 21); err != nil {
		t.Fatal(err)
	}
	return st, u.ID, kid.ID
}

// freezeToday 는 '오늘'을 고정한다(재고 계산이 오늘 기준).
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

func seedBabyfood(t *testing.T, s *Store, childID int64) *BFPlanFile {
	t.Helper()
	f, err := ParseBFPlan([]byte(testPlanJSON))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BFImport(context.Background(), childID, f, nil, true, false, 0); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestBFImportIsDryRunByDefault(t *testing.T) {
	s, _, kid := newBFStore(t)
	f, err := ParseBFPlan([]byte(testPlanJSON))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := s.BFImport(context.Background(), kid, f, nil, false, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if plan.NewDays != 4 {
		t.Fatalf("dry-run이 4일을 세야 하는데 %d", plan.NewDays)
	}
	days, err := s.BFRange(context.Background(), kid, 0, 400)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 0 {
		t.Fatalf("dry-run인데 %d일이 들어갔다", len(days))
	}
}

func TestBFImportSkipsExistingDays(t *testing.T) {
	s, uid, kid := newBFStore(t)
	ctx := context.Background()
	f := seedBabyfood(t, s, kid)

	// 손으로 고친 뒤 다시 가져오면 그 날은 건드리지 않아야 한다.
	days, err := s.BFRange(ctx, kid, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	tops := []string{"바꾼재료"}
	if _, err := s.BFUpdateMeal(ctx, days[0].Meals[0].ID, BFMealInput{Toppings: &tops}, uid); err != nil {
		t.Fatal(err)
	}
	plan, err := s.BFImport(ctx, kid, f, nil, true, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if plan.NewDays != 0 || plan.Existing != 4 {
		t.Fatalf("두 번째 가져오기: new=%d existing=%d", plan.NewDays, plan.Existing)
	}
	days, err = s.BFRange(ctx, kid, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got := days[0].Meals[0].Toppings; len(got) != 1 || got[0] != "바꾼재료" {
		t.Fatalf("수정이 덮어써졌다: %v", got)
	}
}

func TestBFEditKeepsOriginalAndResetRestoresIt(t *testing.T) {
	s, uid, kid := newBFStore(t)
	ctx := context.Background()
	seedBabyfood(t, s, kid)

	days, _ := s.BFRange(ctx, kid, 101, 101)
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

// 실사 이후의 소모·제조만 반영되고, 오늘 것은 '필요'로 잡혀야 한다.
func TestBFStockLedger(t *testing.T) {
	s, uid, kid := newBFStore(t)
	ctx := context.Background()
	seedBabyfood(t, s, kid)

	// 생일을 D+100 이 2026-01-01 이 되게 잡고, 오늘을 D+102 로 고정한다.
	freezeToday(t, "2026-01-03") // D+102

	view, err := s.BFStockList(ctx, kid, 3) // D+102 ~ D+104
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

	// 어제(D+101) 기준 실사 → 오늘(D+102) 소모는 빠지지 않는다.
	if err := s.BFCount(ctx, kid, "가재료", 10, uid); err != nil {
		t.Fatal(err)
	}
	view, err = s.BFStockList(ctx, kid, 3)
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
	if err := s.BFAddBatch(ctx, kid, "가재료", 5, "만듦", uid); err != nil {
		t.Fatal(err)
	}
	view, _ = s.BFStockList(ctx, kid, 3)
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
	s, uid, kid := newBFStore(t)
	ctx := context.Background()
	seedBabyfood(t, s, kid)
	freezeToday(t, "2026-01-03") // D+102
	// D+100 에 실사 0 → D+101 소모가 그대로 빠진다.
	if err := s.BFCount(ctx, kid, "나재료", 0, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE bf_ingredients SET count_dday=100 WHERE name='나재료'`); err != nil {
		t.Fatal(err)
	}
	view, _ := s.BFStockList(ctx, kid, 1)
	var before int
	for _, it := range view.Items {
		if it.Name == "나재료" {
			before = it.Used
		}
	}
	if before != 1 {
		t.Fatalf("D+101 소모가 1이어야 한다: %d", before)
	}

	days, _ := s.BFRange(ctx, kid, 101, 101)
	skip := true
	if _, err := s.BFUpdateMeal(ctx, days[0].Meals[0].ID, BFMealInput{Skipped: &skip}, uid); err != nil {
		t.Fatal(err)
	}
	view, _ = s.BFStockList(ctx, kid, 1)
	for _, it := range view.Items {
		if it.Name == "나재료" && it.Used != 0 {
			t.Fatalf("건너뛴 끼니가 소모로 잡혔다: %d", it.Used)
		}
	}
}

// 생일이 없으면 대상 등록 단계에서 막는다.
func TestBFChildNeedsBirthDate(t *testing.T) {
	s, _, _ := newBFStore(t)
	ctx := context.Background()
	u, err := s.CreateUser(ctx, "생일없는아이", "testpass3")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BFAddChild(ctx, u.ID, 21); err == nil {
		t.Fatal("생일 없이 이유식 대상이 됐다")
	}
	if _, err := s.SetBirthDate(ctx, u.ID, "2026-02-01"); err != nil {
		t.Fatal(err)
	}
	if err := s.BFAddChild(ctx, u.ID, 21); err != nil {
		t.Fatalf("생일을 넣었는데도 거절됐다: %v", err)
	}
	kids, err := s.BFChildren(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(kids) != 2 {
		t.Fatalf("아이 %d명, 2명이어야 한다", len(kids))
	}
}

// 아이가 둘이면 자료가 섞이지 않아야 한다.
func TestBFChildrenAreIsolated(t *testing.T) {
	s, uid, kid := newBFStore(t)
	ctx := context.Background()
	seedBabyfood(t, s, kid)

	other, err := s.CreateUser(ctx, "둘째", "testpass4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetBirthDate(ctx, other.ID, "2026-02-01"); err != nil {
		t.Fatal(err)
	}
	if err := s.BFAddChild(ctx, other.ID, 21); err != nil {
		t.Fatal(err)
	}

	// 첫째에게만 식단이 있다.
	if d, _ := s.BFRange(ctx, kid, 0, 400); len(d) == 0 {
		t.Fatal("첫째 식단이 없다")
	}
	if d, _ := s.BFRange(ctx, other.ID, 0, 400); len(d) != 0 {
		t.Fatalf("둘째에게 첫째 식단이 보인다: %d일", len(d))
	}

	// 첫째에게 남긴 기록이 둘째에게 보이면 안 된다.
	yes := true
	if _, err := s.BFSetLog(ctx, kid, 100, "가재료", BFFoodTag{Reaction: &yes}, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BFSetLog(ctx, other.ID, 100, "가재료", BFFoodTag{Reaction: &yes}, uid); err == nil {
		t.Fatal("둘째에게 없는 날짜에 기록이 남았다")
	}
	foods, err := s.BFFoods(ctx, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(foods) != 0 {
		t.Fatalf("둘째의 재료 목록에 %d종이 섞였다", len(foods))
	}
}

func TestBFSrcSurvivesJSONRoundTrip(t *testing.T) {
	s, _, kid := newBFStore(t)
	seedBabyfood(t, s, kid)
	days, err := s.BFRange(context.Background(), kid, 102, 102)
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
	s, uid, kid := newBFStore(t)
	ctx := context.Background()
	f := seedBabyfood(t, s, kid)

	days, _ := s.BFRange(ctx, kid, 100, 100)
	tops := []string{"손으로고친것"}
	if _, err := s.BFUpdateMeal(ctx, days[0].Meals[0].ID, BFMealInput{Toppings: &tops}, uid); err != nil {
		t.Fatal(err)
	}

	// D+101 부터만, 덮어쓰기로 다시 넣는다.
	plan, err := s.BFImport(ctx, kid, f, nil, true, true, 101)
	if err != nil {
		t.Fatal(err)
	}
	if plan.NewDays != 3 { // 101, 102, 103
		t.Fatalf("D+101부터면 3일이어야 한다: %d", plan.NewDays)
	}
	days, _ = s.BFRange(ctx, kid, 100, 100)
	if got := days[0].Meals[0].Toppings; len(got) != 1 || got[0] != "손으로고친것" {
		t.Fatalf("지나간 날이 덮어써졌다: %v", got)
	}
}

// 재고 → 장보기 카드. 본문이 편집기가 받는 형식인지도 본다.
func TestBFShoppingCard(t *testing.T) {
	s, uid, kid := newBFStore(t)
	ctx := context.Background()
	seedBabyfood(t, s, kid)
	freezeToday(t, "2026-01-03") // D+102

	card, err := s.BFShoppingCard(ctx, kid, 3, uid)
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
		if err := s.BFCount(ctx, kid, n, 99, uid); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.BFShoppingCard(ctx, kid, 3, uid); err == nil {
		t.Fatal("만들 게 없는데 카드를 만들었다")
	}
}

// 같은 재료를 먹인 날마다 따로 기록된다.
func TestBFLogIsPerDay(t *testing.T) {
	s, uid, kid := newBFStore(t)
	seedBabyfood(t, s, kid)
	ctx := context.Background()
	yes, no := true, false

	// 100일엔 잘 먹었고, 101일엔 반응이 왔다.
	if _, err := s.BFSetLog(ctx, kid, 100, "가재료", BFFoodTag{Liked: &yes}, uid); err != nil {
		t.Fatal(err)
	}
	day, err := s.BFSetLog(ctx, kid, 101, "가재료", BFFoodTag{Reaction: &yes}, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(day.Logs) != 1 || day.Logs[0].Name != "가재료" || !day.Logs[0].Reaction {
		t.Fatalf("101일 기록이 이상하다: %+v", day.Logs)
	}
	if day.Logs[0].Date != "2026-01-02" {
		t.Fatalf("기록에 날짜가 안 붙었다: %q", day.Logs[0].Date)
	}

	// 재료 요약은 두 날을 모두 안다.
	foods, err := s.BFFoods(ctx, kid)
	if err != nil {
		t.Fatal(err)
	}
	var f BFFood
	for _, x := range foods {
		if x.Name == "가재료" {
			f = x
		}
	}
	if !f.Reaction || !f.Liked {
		t.Fatalf("요약이 둘 다 잡지 못했다: %+v", f)
	}
	if len(f.ReactionDates) != 1 || f.ReactionDates[0] != "2026-01-02" {
		t.Fatalf("반응 날짜가 이상하다: %v", f.ReactionDates)
	}
	if len(f.LikedDates) != 1 || f.LikedDates[0] != "2026-01-01" {
		t.Fatalf("좋아함 날짜가 이상하다: %v", f.LikedDates)
	}

	// 좋아함과 싫어함은 서로를 끈다.
	day, err = s.BFSetLog(ctx, kid, 100, "가재료", BFFoodTag{Disliked: &yes}, uid)
	if err != nil {
		t.Fatal(err)
	}
	if !day.Logs[0].Disliked || day.Logs[0].Liked {
		t.Fatalf("싫어함을 켰는데 좋아함이 남았다: %+v", day.Logs[0])
	}

	// 표시를 전부 끄면 행이 사라진다.
	day, err = s.BFSetLog(ctx, kid, 101, "가재료", BFFoodTag{Reaction: &no}, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(day.Logs) != 0 {
		t.Fatalf("표시를 껐는데 기록이 남았다: %+v", day.Logs)
	}

	// 식단이 없는 날에는 남기지 않는다.
	if _, err := s.BFSetLog(ctx, kid, 900, "가재료", BFFoodTag{Liked: &yes}, uid); err == nil {
		t.Fatal("식단이 없는 날에 기록이 남았다")
	}
}

// 끼니 이름은 끼니 수로(1끼=점심), 시각은 기본값을 따른다.
func TestBFMealTitlesAndTimes(t *testing.T) {
	s, uid, kid := newBFStore(t)
	seedBabyfood(t, s, kid)
	ctx := context.Background()

	// D+100 은 한 끼, D+102 는 두 끼다.
	one, err := s.BFRange(ctx, kid, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got := one[0].Meals[0].Title; got != "점심" {
		t.Fatalf("한 끼인 날의 이름 = %q, want 점심 (저장된 slot 은 %q)", got, one[0].Meals[0].Slot)
	}
	if got := one[0].Meals[0].At; got != "12:00" {
		t.Fatalf("기본 시각 = %q, want 12:00", got)
	}
	if one[0].Meals[0].AtSet {
		t.Fatal("기본값을 따르는데 직접 넣은 것으로 표시됐다")
	}

	two, err := s.BFRange(ctx, kid, 102, 102)
	if err != nil {
		t.Fatal(err)
	}
	if a, b := two[0].Meals[0].Title, two[0].Meals[1].Title; a != "점심" || b != "저녁" {
		t.Fatalf("두 끼인 날의 이름 = %q, %q, want 점심, 저녁", a, b)
	}
	if a, b := two[0].Meals[0].At, two[0].Meals[1].At; a != "12:00" || b != "18:00" {
		t.Fatalf("두 끼인 날의 시각 = %q, %q", a, b)
	}

	// 기본값을 바꾸면 직접 넣지 않은 끼니가 따라온다.
	if err := s.BFSetMealTimes(ctx, kid, 2, []string{"11:30", "17:30"}); err != nil {
		t.Fatal(err)
	}
	two, _ = s.BFRange(ctx, kid, 102, 102)
	if a := two[0].Meals[0].At; a != "11:30" {
		t.Fatalf("기본값을 바꿨는데 %q 그대로다", a)
	}

	// 한 끼니만 시각을 직접 넣으면 그것만 고정되고 '수정됨'이 된다.
	at := "13:15"
	day, err := s.BFUpdateMeal(ctx, two[0].Meals[0].ID, BFMealInput{At: &at}, uid)
	if err != nil {
		t.Fatal(err)
	}
	if !day.Meals[0].AtSet || day.Meals[0].At != "13:15" || !day.Meals[0].Edited {
		t.Fatalf("직접 넣은 시각이 안 붙었다: %+v", day.Meals[0])
	}
	if day.Meals[1].At != "17:30" {
		t.Fatalf("옆 끼니까지 바뀌었다: %q", day.Meals[1].At)
	}

	// 원래대로 누르면 다시 기본값을 따른다.
	day, err = s.BFUpdateMeal(ctx, two[0].Meals[0].ID, BFMealInput{Reset: true}, uid)
	if err != nil {
		t.Fatal(err)
	}
	if day.Meals[0].AtSet || day.Meals[0].At != "11:30" {
		t.Fatalf("되돌렸는데 시각이 남았다: %+v", day.Meals[0])
	}

	// 말이 안 되는 시간표는 받지 않는다.
	for _, bad := range [][]string{{"25:00", "18:00"}, {"18:00", "12:00"}, {"12:00"}} {
		if err := s.BFSetMealTimes(ctx, kid, 2, bad); err == nil {
			t.Fatalf("%v 를 받아들였다", bad)
		}
	}
}
