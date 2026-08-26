package store

import (
	"context"
	"testing"
)

func TestStatReportsUsage(t *testing.T) {
	st, by, cols := openTest(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		title := "카드"
		if _, err := st.CreateCard(ctx, cols[0], CardInput{Title: &title}, by); err != nil {
			t.Fatal(err)
		}
	}
	s, err := st.Stat(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.Cards != 5 || s.Boards != 1 || s.Users != 1 {
		t.Fatalf("행 수가 안 맞음: %+v", s)
	}
	if s.DBBytes <= 0 {
		t.Fatal("DB 크기가 0")
	}
	if s.UsedBytes != s.DBBytes+s.BackupBytes {
		t.Fatalf("UsedBytes = %d, want %d", s.UsedBytes, s.DBBytes+s.BackupBytes)
	}
	if s.Quota != DefaultQuota {
		t.Fatalf("기본 한도 = %d", s.Quota)
	}
	if s.DiskTotal == 0 || s.DiskFree == 0 {
		t.Fatalf("디스크 수치가 0: total=%d free=%d", s.DiskTotal, s.DiskFree)
	}
	if s.BytesPerCard <= 0 {
		t.Fatal("카드당 크기 추정이 0")
	}
}

// 한도를 넘으면 새로 만드는 것만 막고, 수정·삭제는 계속 돼야 한다.
// 삭제까지 막으면 공간을 비울 방법이 없어진다.
func TestQuotaBlocksCreateButNotDelete(t *testing.T) {
	st, by, cols := openTest(t)
	ctx := context.Background()
	title := "기존"
	c, err := st.CreateCard(ctx, cols[0], CardInput{Title: &title}, by)
	if err != nil {
		t.Fatal(err)
	}

	st.SetQuota(1) // 1바이트 — 반드시 초과
	over, s, err := st.QuotaExceeded(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !over {
		t.Fatalf("초과로 나와야 함: used=%d quota=%d", s.UsedBytes, s.Quota)
	}

	// 스토어 자체는 막지 않는다(막는 건 API의 생성 경로다) — 수정·삭제가
	// 여기서 실패하지 않는지 확인한다.
	newTitle := "수정됨"
	if _, err := st.UpdateCard(ctx, c.ID, CardInput{Title: &newTitle}); err != nil {
		t.Fatalf("한도 초과 중 수정이 실패: %v", err)
	}
	if err := st.DeleteCard(ctx, c.ID); err != nil {
		t.Fatalf("한도 초과 중 삭제가 실패: %v", err)
	}

	st.SetQuota(DefaultQuota)
	if over, _, _ := st.QuotaExceeded(ctx); over {
		t.Fatal("한도를 되돌렸는데 여전히 초과")
	}
}
