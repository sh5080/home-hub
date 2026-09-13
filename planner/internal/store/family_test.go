package store

import (
	"context"
	"testing"
	"time"
)

// 일기를 매일 쓰면 연속 기록이 이어지고, 물방울이 하루 한 번만 쌓인다. 5단계가 되면 위시리스트 하나를 산다.
func TestFamilyPlant(t *testing.T) {
	s, uid := newDiaryStore(t)
	ctx := context.Background()
	day := func(d string) time.Time {
		tm, _ := time.ParseInLocation("2006-01-02 15:04", d+" 21:00", time.Local)
		return tm
	}
	b, err := s.Family(ctx, day("2026-10-01"))
	if err != nil {
		t.Fatal(err)
	}
	if b.Plant.Stage != 1 || b.Plant.Drops != 0 || len(b.Quests) == 0 {
		t.Fatalf("처음 %+v", b.Plant)
	}
	for _, d := range []string{"2026-10-01", "2026-10-02", "2026-10-03"} {
		if _, err := s.DiaryCreate(ctx, d, "", uid); err != nil {
			t.Fatal(err)
		}
	}
	b, _ = s.Family(ctx, day("2026-10-03"))
	var diary FamilyStreak
	for _, st := range b.Streaks {
		if st.Key == "diary" {
			diary = st
		}
	}
	if diary.Current != 3 || !diary.TodayDone {
		t.Fatalf("일기 연속 %+v", diary)
	}
	first := b.Plant.Drops
	if first < 3 {
		t.Fatalf("물방울 %d", first)
	}
	again, _ := s.Family(ctx, day("2026-10-03"))
	if again.Plant.Drops != first {
		t.Fatalf("다시 열었더니 물방울이 또 쌓였다 %d → %d", first, again.Plant.Drops)
	}
	wid, err := s.FamilySaveWish(ctx, 0, "아기 띠", "", 89000, uid)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FamilyGrowNext(ctx, day("2026-10-03")); err == nil {
		t.Fatal("다 안 자랐는데 다음 식물로 넘어갔다")
	}
	if err := s.FamilyBuyWish(ctx, wid, day("2026-10-03")); err == nil {
		t.Fatal("구매권 없이 샀다")
	}
	if err := s.FamilyRenamePlant(ctx, "콩콩이", day("2026-10-03")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO family_drops (key, amount, day, label) VALUES ('test', 300, '2026-10-03', '시험')`); err != nil {
		t.Fatal(err)
	}
	b, _ = s.Family(ctx, day("2026-10-03"))
	if !b.Plant.Ready || b.Plant.Stage != 5 {
		t.Fatalf("5단계 %+v", b.Plant)
	}
	if err := s.FamilyGrowNext(ctx, day("2026-10-03")); err != nil {
		t.Fatal(err)
	}
	b, _ = s.Family(ctx, day("2026-10-04"))
	if b.Plant.Stage != 1 || b.Plant.Count != 1 || b.Plant.Credits != 1 || b.Plant.Name != "콩콩이" {
		t.Fatalf("새 씨앗 %+v", b.Plant)
	}
	if err := s.FamilyBuyWish(ctx, wid, day("2026-10-04")); err != nil {
		t.Fatal(err)
	}
	b, _ = s.Family(ctx, day("2026-10-04"))
	if b.Plant.Credits != 0 || b.Wishes[0].BoughtAt == nil {
		t.Fatalf("산 뒤 %+v %+v", b.Plant, b.Wishes)
	}
}

// 할 일은 처음 정한 마감까지 끝내야 물방울을 받는다. 마감을 미뤄도 처음 날짜로 본다.
func TestTaskReward(t *testing.T) {
	s, uid := newDiaryStore(t)
	ctx := context.Background()
	boards, _ := s.ListBoards(ctx)
	b, _ := s.GetBoard(ctx, boards[0].ID, SortTime, Asc)
	todo, done := b.Columns[0].ID, b.Columns[len(b.Columns)-1].ID
	today := time.Now().Format("2006-01-02")
	past := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	future := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	mk := func(title, due string) Card {
		c, err := s.CreateCard(ctx, todo, CardInput{Title: &title, DueAt: &due}, uid)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	onTime := mk("빨래", today)
	c, err := s.MoveCard(ctx, onTime.ID, done, 0)
	if err != nil || c.Reward == nil || !c.Reward.OnTime || c.Reward.Drops != 2 {
		t.Fatalf("제때 %+v %v", c.Reward, err)
	}
	// 되돌렸다 다시 끝내도 두 번 받지 않는다.
	s.MoveCard(ctx, onTime.ID, todo, 0)
	if c, _ := s.MoveCard(ctx, onTime.ID, done, 0); c.Reward == nil || c.Reward.Drops != 0 {
		t.Fatalf("두 번 받음 %+v", c.Reward)
	}
	// 지난 마감을 뒤로 미뤄도 처음 날짜로 본다.
	late := mk("장보기", past)
	if _, err := s.UpdateCard(ctx, late.ID, CardInput{DueAt: &future}); err != nil {
		t.Fatal(err)
	}
	if c, _ := s.MoveCard(ctx, late.ID, done, 0); c.Reward == nil || c.Reward.OnTime || c.Reward.Drops != 0 || c.Reward.FirstDue != past {
		t.Fatalf("늦음 %+v", c.Reward)
	}
	// 마감 없는 할 일은 판정하지 않는다.
	title := "정리"
	free, _ := s.CreateCard(ctx, todo, CardInput{Title: &title}, uid)
	if c, _ := s.MoveCard(ctx, free.ID, done, 0); c.Reward != nil {
		t.Fatalf("마감 없음 %+v", c.Reward)
	}
}
