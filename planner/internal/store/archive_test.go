package store

import (
	"context"
	"testing"
	"time"
)

// 완료한 지 한 달 지난 카드는 보드에서 빠져 보관함으로, 다른 칸으로
// 옮기면 다시 보드로 돌아온다.
func TestCardArchive(t *testing.T) {
	s, uid, _ := newBFStore(t)
	ctx := context.Background()
	boards, _ := s.ListBoards(ctx)
	bid := boards[0].ID
	b, _ := s.GetBoard(ctx, bid, SortTime, Asc)
	todo, done := b.Columns[0].ID, b.Columns[len(b.Columns)-1].ID

	mk := func(title string) Card {
		c, err := s.CreateCard(ctx, todo, CardInput{Title: &title}, uid)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.MoveCard(ctx, c.ID, done, 0); err != nil {
			t.Fatal(err)
		}
		return c
	}
	old, fresh := mk("오래된 빨래"), mk("어제 장보기")
	// 오래된 쪽은 40일 전에 끝난 걸로 돌려놓는다.
	if _, err := s.db.ExecContext(ctx, `UPDATE cards SET done_at=? WHERE id=?`, time.Now().Add(-40*24*time.Hour).Unix(), old.ID); err != nil {
		t.Fatal(err)
	}

	b, _ = s.GetBoard(ctx, bid, SortTime, Asc)
	var visible []string
	for _, c := range b.Columns[len(b.Columns)-1].Cards {
		visible = append(visible, c.Title)
	}
	if len(visible) != 1 || visible[0] != "어제 장보기" {
		t.Fatalf("완료 칸 = %v, 오래된 건 빠져야 한다", visible)
	}
	arch, err := s.ArchivedCards(ctx, bid, "")
	if err != nil || len(arch) != 1 || arch[0].ID != old.ID || arch[0].DoneAt == nil {
		t.Fatalf("보관함 = %+v %v", arch, err)
	}
	if got, _ := s.ArchivedCards(ctx, bid, "빨래"); len(got) != 1 {
		t.Fatal("보관함 검색이 안 된다")
	}
	if got, _ := s.ArchivedCards(ctx, bid, "장보기"); len(got) != 0 {
		t.Fatal("보관 안 된 카드가 보관함 검색에 나왔다")
	}

	// 다시 할 일로 옮기면 보관이 풀리고 보드에 돌아온다.
	if _, err := s.MoveCard(ctx, old.ID, todo, 0); err != nil {
		t.Fatal(err)
	}
	c, _ := s.GetCard(ctx, old.ID)
	if c.ArchivedAt != nil || c.DoneAt != nil {
		t.Fatalf("옮겼는데 보관·완료 시각이 남았다: %+v", c)
	}
	if arch, _ := s.ArchivedCards(ctx, bid, ""); len(arch) != 0 {
		t.Fatal("보관함에 남았다")
	}
	// 손으로 보관하기: 완료 카드만. 풀면 완료 칸으로 돌아오고 바로 다시
	// 보관되지 않는다.
	if _, err := s.ArchiveCard(ctx, old.ID); err == nil {
		t.Fatal("할 일 칸의 카드를 보관했다")
	}
	if _, err := s.ArchiveCard(ctx, fresh.ID); err != nil {
		t.Fatal(err)
	}
	if arch, _ := s.ArchivedCards(ctx, bid, ""); len(arch) != 1 || arch[0].ID != fresh.ID {
		t.Fatalf("손으로 보관한 게 보관함에 없다: %+v", arch)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE cards SET done_at=? WHERE id=?`, time.Now().Add(-60*24*time.Hour).Unix(), fresh.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UnarchiveCard(ctx, fresh.ID); err != nil {
		t.Fatal(err)
	}
	b, _ = s.GetBoard(ctx, bid, SortTime, Asc)
	if n := len(b.Columns[len(b.Columns)-1].Cards); n != 1 {
		t.Fatalf("보관을 풀었는데 완료 칸에 %d장 — 바로 다시 보관됐다", n)
	}
}
