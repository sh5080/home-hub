package store

import (
	"context"
	"testing"
	"time"
)

// 알림은 "조건이 아직 풀리지 않았을 때만" 간다. 끝낸 일에는 안 온다.
func TestNotifyTasksOnlyWhileUnresolved(t *testing.T) {
	s, uid, _ := newBFStore(t)
	ctx := context.Background()
	loc := time.Local
	at := func(hm string) time.Time {
		tm, _ := time.ParseInLocation("2006-01-02 15:04", "2026-01-01 "+hm, loc)
		return tm
	}

	boards, _ := s.ListBoards(ctx)
	b, err := s.GetBoard(ctx, boards[0].ID, SortTime, Asc)
	if err != nil {
		t.Fatal(err)
	}
	todo, done := b.Columns[0].ID, b.Columns[len(b.Columns)-1].ID
	title, due := "빨래", "2026-01-01"
	card, err := s.CreateCard(ctx, todo, CardInput{Title: &title, DueAt: &due}, uid)
	if err != nil {
		t.Fatal(err)
	}

	// 목요일(2026-01-01) 21:00, 매일.
	rule, err := s.NotifySave(ctx, NotifyRule{Kind: "tasks", DaysMask: 127, Times: []string{"21:00"}, Enabled: true}, uid)
	if err != nil {
		t.Fatal(err)
	}

	if due, _ := s.NotifyDue(ctx, at("20:59")); len(due) != 0 {
		t.Fatal("시각 전에 울렸다")
	}
	got, err := s.NotifyDue(ctx, at("21:00"))
	if err != nil || len(got) != 1 || got[0].Body != "빨래" {
		t.Fatalf("21:00 에 할 일 알림이 와야 한다: %+v %v", got, err)
	}
	// 10분 안이면 늦게 돌아도 보낸다(서버가 잠깐 멈췄던 경우).
	if got, _ := s.NotifyDue(ctx, at("21:07")); len(got) != 1 {
		t.Fatal("몇 분 늦게 확인했을 때 놓쳤다")
	}
	// 한 번 보냈으면 다시 안 보낸다.
	if err := s.NotifyMarkSent(ctx, rule.ID, got[0].Key); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.NotifyDue(ctx, at("21:05")); len(got) != 0 {
		t.Fatal("같은 알림이 두 번 갔다")
	}

	// 다음 날 같은 시각: 카드를 끝냈으면 안 온다.
	if _, err := s.MoveCard(ctx, card.ID, done, 0); err != nil {
		t.Fatal(err)
	}
	next, _ := time.ParseInLocation("2006-01-02 15:04", "2026-01-02 21:00", loc)
	if got, _ := s.NotifyDue(ctx, next); len(got) != 0 {
		t.Fatalf("끝낸 일인데 알림이 왔다: %+v", got)
	}

	// 요일에 없는 날은 안 온다(1월 3일 토요일, 평일만).
	if _, err := s.NotifySave(ctx, NotifyRule{ID: rule.ID, Kind: "custom", Title: "물 마시기", DaysMask: 0b0011111, Times: []string{"09:00"}, Enabled: true}, uid); err != nil {
		t.Fatal(err)
	}
	sat, _ := time.ParseInLocation("2006-01-02 15:04", "2026-01-03 09:00", loc)
	if got, _ := s.NotifyDue(ctx, sat); len(got) != 0 {
		t.Fatal("주말에 평일 알림이 왔다")
	}
	fri, _ := time.ParseInLocation("2006-01-02 15:04", "2026-01-02 09:00", loc)
	if got, _ := s.NotifyDue(ctx, fri); len(got) != 1 || got[0].Body != "물 마시기" {
		t.Fatalf("평일 알림 = %+v", got)
	}
}

// 기록 공백: 마지막 수유가 정한 시간을 넘으면 한 번, 새로 기록하면 다시 센다.
func TestNotifyCareGap(t *testing.T) {
	s, uid, kid := newBFStore(t)
	ctx := context.Background()
	str := func(v string) *string { return &v }
	at := func(v string) time.Time {
		tm, _ := time.ParseInLocation("2006-01-02T15:04", v, time.Local)
		return tm
	}
	if _, err := s.CareAdd(ctx, kid, CareInput{Kind: str("formula"), At: str("2026-01-01T10:00")}, uid); err != nil {
		t.Fatal(err)
	}
	rule, err := s.NotifySave(ctx, NotifyRule{
		Kind: "care", DaysMask: 127, Times: []string{"08:00", "22:00"}, Enabled: true,
		Params: NotifyParams{Cond: "since", Amount: 3, Unit: "hours", Watch: true, ChildID: kid, CareKinds: []string{"formula", "breast"}},
	}, uid)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s.NotifyDue(ctx, at("2026-01-01T12:59")); len(got) != 0 {
		t.Fatal("3시간 전에 울렸다")
	}
	got, _ := s.NotifyDue(ctx, at("2026-01-01T13:01"))
	if len(got) != 1 {
		t.Fatal("3시간이 지났는데 안 울렸다")
	}
	_ = s.NotifyMarkSent(ctx, rule.ID, got[0].Key)
	if got, _ := s.NotifyDue(ctx, at("2026-01-01T14:00")); len(got) != 0 {
		t.Fatal("같은 공백으로 두 번 울렸다")
	}
	// 새로 먹이면 다시 기다린다.
	if _, err := s.CareAdd(ctx, kid, CareInput{Kind: str("breast"), At: str("2026-01-01T14:30")}, uid); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.NotifyDue(ctx, at("2026-01-01T15:00")); len(got) != 0 {
		t.Fatal("방금 먹였는데 울렸다")
	}
	if got, _ := s.NotifyDue(ctx, at("2026-01-01T17:31")); len(got) != 1 {
		t.Fatal("다음 공백에 안 울렸다")
	}
	// 감시 구간 밖(밤)엔 안 울린다.
	if got, _ := s.NotifyDue(ctx, at("2026-01-01T23:30")); len(got) != 0 {
		t.Fatal("감시 구간 밖에서 울렸다")
	}
}

func TestNotifyRuleValidation(t *testing.T) {
	s, uid, _ := newBFStore(t)
	ctx := context.Background()
	for _, bad := range []NotifyRule{
		{Kind: "nope", DaysMask: 127, Times: []string{"09:00"}},
		{Kind: "tasks", DaysMask: 0, Times: []string{"09:00"}},
		{Kind: "tasks", DaysMask: 127, Times: []string{"25:00"}},
		{Kind: "care", DaysMask: 127, Times: []string{"08:00"}, Params: NotifyParams{Cond: "since", Amount: 3, Unit: "hours", Watch: true, CareKinds: []string{"formula"}}},
		{Kind: "tasks", DaysMask: 127, Times: []string{"09:00"}, Params: NotifyParams{Cond: "since", Amount: 3, Unit: "hours"}},
		{Kind: "finance", DaysMask: 127, Times: []string{"09:00"}, Params: NotifyParams{Cond: "since", Amount: 0, Unit: "days"}},
		{Kind: "custom", DaysMask: 127, Times: []string{"09:00"}},
	} {
		if _, err := s.NotifySave(ctx, bad, uid); err == nil {
			t.Errorf("%+v 를 받아들였다", bad)
		}
	}
}

// 재정 파일: 마지막 업로드 후 정한 날이 지나면 올릴 때까지 매일 그 사람에게.
func TestNotifyFinanceSince(t *testing.T) {
	s, uid := newDiaryStore(t)
	ctx := context.Background()
	if _, err := s.FinanceImport(ctx, uid, FinParsed{TakenAt: "2026-01-01", Items: []FinItem{{Group: "liquid", Name: "통장", Amount: 1000}}}, true, uid); err != nil {
		t.Fatal(err)
	}
	rules, _ := s.NotifyRules(ctx)
	if len(rules) != 1 || rules[0].Kind != "finance" || rules[0].Params.Cond != "since" || rules[0].Params.Amount != 30 {
		t.Fatalf("업로드 알림이 기본으로 안 생겼다: %+v", rules)
	}
	if d := s.finUploadDays(ctx); d != 30 {
		t.Fatalf("주기 %d", d)
	}
	day := func(n int) time.Time {
		return time.Now().AddDate(0, 0, n).Truncate(24 * time.Hour).Add(9 * time.Hour).Local()
	}
	at9 := func(n int) time.Time {
		d := day(n)
		return time.Date(d.Year(), d.Month(), d.Day(), 9, 0, 0, 0, time.Local)
	}
	if got, _ := s.NotifyDue(ctx, at9(10)); len(got) != 0 {
		t.Fatal("주기 전에 울렸다")
	}
	got, _ := s.NotifyDue(ctx, at9(31))
	if len(got) != 1 || got[0].Recipients[0] != uid {
		t.Fatalf("주기가 지났는데 %+v", got)
	}
	_ = s.NotifyMarkSent(ctx, got[0].RuleID, got[0].Key)
	if got, _ := s.NotifyDue(ctx, at9(32)); len(got) != 1 {
		t.Fatal("다음 날 다시 안 울렸다(올릴 때까지 매일)")
	}
}
