package store

import (
	"context"
	"testing"
	"time"
)

func TestCareLogsDaySummary(t *testing.T) {
	s, uid, kid := newBFStore(t)
	ctx := context.Background()
	str := func(v string) *string { return &v }
	num := func(v int) *int { return &v }

	// 분유 두 번, 기저귀, 진행 중인 수면.
	for _, in := range []CareInput{
		{Kind: str("formula"), At: str("2026-01-01T07:30"), AmountML: num(120)},
		{Kind: str("formula"), At: str("2026-01-01T11:00"), AmountML: num(160)},
		{Kind: str("diaper"), At: str("2026-01-01T11:05"), Detail: str("소변")},
		{Kind: str("sleep"), At: str("2026-01-01T12:00")},
		{Kind: str("solids"), At: str("2026-01-01T17:00"), AmountML: num(75), Detail: str("쌀미음, 소고기")},
	} {
		if _, err := s.CareAdd(ctx, kid, in, uid); err != nil {
			t.Fatal(err)
		}
	}
	day, err := s.CareList(ctx, kid, "2026-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(day.Logs) != 5 || day.Logs[0].Kind != "solids" {
		t.Fatalf("늦은 것부터여야 한다: %+v", day.Logs)
	}
	if day.FormulaML != 280 || day.SolidsML != 75 || day.Diapers != 1 || day.Feedings != 2 || day.SleepMin != 0 {
		t.Fatalf("합계가 이상하다: %+v", day)
	}

	// 재우고 나서 깼을 때 분을 채운다.
	var sleep CareLog
	for _, l := range day.Logs {
		if l.Kind == "sleep" {
			sleep = l
		}
	}
	if sleep.Minutes != nil {
		t.Fatal("진행 중인 수면에 분이 있다")
	}
	if _, err := s.CareUpdate(ctx, sleep.ID, CareInput{Minutes: num(95)}); err != nil {
		t.Fatal(err)
	}
	day, _ = s.CareList(ctx, kid, "2026-01-01")
	if day.SleepMin != 95 {
		t.Fatalf("수면 합계 = %d", day.SleepMin)
	}

	// 마지막 기록은 종류별로.
	last, err := s.CareLast(ctx, kid)
	if err != nil {
		t.Fatal(err)
	}
	if last["formula"].At != "2026-01-01T11:00" || last["diaper"].Detail == nil {
		t.Fatalf("마지막 기록이 이상하다: %+v", last)
	}

	// 고를 수 없는 상세, 모르는 종류, 다른 날은 빈 목록.
	if _, err := s.CareAdd(ctx, kid, CareInput{Kind: str("diaper"), Detail: str("아무거나")}, uid); err == nil {
		t.Fatal("기저귀에 아무 값이나 들어갔다")
	}
	if _, err := s.CareAdd(ctx, kid, CareInput{Kind: str("nap")}, uid); err == nil {
		t.Fatal("모르는 종류를 받았다")
	}
	if d, _ := s.CareList(ctx, kid, "2026-01-02"); len(d.Logs) != 0 {
		t.Fatal("다른 날 기록이 섞였다")
	}
}

// 기록은 이미 일어난 일이다. 앞으로의 시각(자정 넘겨 날짜가 오늘로 붙은
// "23:59")은 받지 않는다.
func TestCareRejectsFutureTime(t *testing.T) {
	s, uid, kid := newBFStore(t)
	ctx := context.Background()
	kind := "formula"
	future := time.Now().Add(3 * time.Hour).Format("2006-01-02T15:04")
	if _, err := s.CareAdd(ctx, kid, CareInput{Kind: &kind, At: &future}, uid); err == nil {
		t.Fatal("앞으로의 시각을 받아들였다")
	}
	past := time.Now().Add(-3 * time.Hour).Format("2006-01-02T15:04")
	l, err := s.CareAdd(ctx, kid, CareInput{Kind: &kind, At: &past}, uid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CareUpdate(ctx, l.ID, CareInput{At: &future}); err == nil {
		t.Fatal("고치면서 앞으로의 시각을 넣었다")
	}
}

// 이유식 기록은 식단 재료에 id 로 걸리고, 끼니에 이으면 식단에 '먹었어요'가 뜬다.
func TestCareSolidsLinkToPlan(t *testing.T) {
	s, uid, kid := newBFStore(t)
	seedBabyfood(t, s, kid)
	ctx := context.Background()
	day, err := s.BFRange(ctx, kid, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	meal := day[0].Meals[0]
	kind, at := "solids", "2026-01-01T12:10"
	names := []string{"가재료", "새재료", "가재료"}
	amt := 40
	l, err := s.CareAdd(ctx, kid, CareInput{Kind: &kind, At: &at, AmountML: &amt, Ingredients: &names, MealID: &meal.ID}, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Items) != 2 || l.Items[0].Name != "가재료" || l.Items[1].Name != "새재료" {
		t.Fatalf("재료가 이상하다(중복 제거·순서): %+v", l.Items)
	}
	if l.Detail == nil || *l.Detail != "가재료, 새재료" {
		t.Fatalf("detail 사본 = %v", l.Detail)
	}
	// 식단에 없던 재료는 재료 목록에 생긴다.
	foods, _ := s.BFFoods(ctx, kid)
	found := false
	for _, f := range foods {
		found = found || f.Name == "새재료"
	}
	if !found {
		t.Fatal("새 재료가 재료 목록에 안 생겼다")
	}
	day, _ = s.BFRange(ctx, kid, 100, 100)
	if e := day[0].Meals[0].Eaten; e == nil || e.LogID != l.ID || e.AmountML == nil || *e.AmountML != 40 {
		t.Fatalf("식단에 먹은 기록이 안 붙었다: %+v", day[0].Meals[0].Eaten)
	}
	// 연결을 풀면 식단에서도 사라진다.
	zero := int64(0)
	if _, err := s.CareUpdate(ctx, l.ID, CareInput{MealID: &zero}); err != nil {
		t.Fatal(err)
	}
	day, _ = s.BFRange(ctx, kid, 100, 100)
	if day[0].Meals[0].Eaten != nil {
		t.Fatal("연결을 풀었는데 남았다")
	}
}
