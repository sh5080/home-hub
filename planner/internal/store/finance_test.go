package store

import (
	"context"
	"testing"
	"time"
)

func TestFinanceFlowAndFixed(t *testing.T) {
	s, uid := newDiaryStore(t)
	ctx := context.Background()
	now, _ := time.Parse("2006-01-02", "2026-09-28")

	var tx []FinTx
	for m := 3; m <= 8; m++ {
		mon := time.Date(2026, time.Month(m), 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
		tx = append(tx,
			FinTx{At: mon + "-25T09:00", Type: "이체", Cat1: "저축", Content: "주식회사 우리회사", Amount: 3_000_000, Method: "입출금"}, // 월급(이체로 옴)
			FinTx{At: mon + "-10T10:00", Type: "이체", Cat1: "미분류", Content: "아파트관리비", Amount: -200_000 - int64(m*1000), Method: "입출금"},
			FinTx{At: mon + "-05T10:00", Type: "지출", Cat1: "미분류", Content: "Apple", Amount: -14_900, Method: "카드"},
			FinTx{At: mon + "-12T12:00", Type: "지출", Cat1: "식사", Content: "식당", Amount: -30_000, Method: "카드"},
			// 내 계좌끼리 옮긴 것: 나가고 들어온 한 쌍
			FinTx{At: mon + "-15T08:00", Type: "이체", Cat1: "미분류", Content: "MMF", Amount: -500_000, Method: "입출금"},
			FinTx{At: mon + "-15T08:03", Type: "이체", Cat1: "투자", Content: "입금", Amount: 500_000, Method: "MMF"},
			// 가족 이름으로 보낸 것
			FinTx{At: mon + "-20T08:00", Type: "이체", Cat1: "미분류", Content: "토스 테스트1", Amount: -100_000, Method: "입출금"},
		)
	}
	tx = append(tx, FinTx{At: "2026-08-18T15:00", Type: "지출", Cat1: "식사", Content: "호텔 뷔페", Amount: -400_000, Method: "카드"})
	p := FinParsed{TakenAt: "2026-09-28", Tx: tx, Items: []FinItem{
		{Group: "liquid", Category: "자유입출금 자산", Name: "입출금", Amount: 5_000_000},
		{Group: "invest", Category: "투자성 자산", Name: "MMF", Amount: 2_000_000},
		{Group: "debt", Category: "장기대출", Name: "대출", Amount: 1_000_000},
	}}
	if _, err := s.FinanceImport(ctx, uid, p, true, uid); err != nil {
		t.Fatal(err)
	}
	ov, err := s.FinanceOverview(ctx, 0, "2026-08", now)
	if err != nil {
		t.Fatal(err)
	}
	if ov.NetWorth != 6_000_000 {
		t.Fatalf("순자산 = %d", ov.NetWorth)
	}
	var aug FinMonth
	for _, m := range ov.Months {
		if m.Month == "2026-08" {
			aug = m
		}
	}
	// 들어온 돈: 월급 3,000,000 (MMF 들어온 쪽은 짝이라 빠짐)
	if aug.In != 3_000_000 {
		t.Fatalf("8월 들어온 돈 = %d", aug.In)
	}
	// 쓴 돈: 관리비 208,000 + Apple 14,900 + 식당 30,000 + 뷔페 400,000 (가족 이체·MMF 이동은 빠짐)
	if aug.Spend != 208_000+14_900+30_000+400_000 {
		t.Fatalf("8월 쓴 돈 = %d", aug.Spend)
	}
	fixed := map[string]bool{}
	for _, f := range ov.Fixed {
		if f.Fixed {
			fixed[f.Label] = true
		}
	}
	if !fixed["아파트관리비"] || !fixed["Apple"] {
		t.Fatalf("고정지출 판정 = %+v", ov.Fixed)
	}
	if len(ov.Unexpected) != 1 || ov.Unexpected[0].Content != "호텔 뷔페" {
		t.Fatalf("예상 외 = %+v", ov.Unexpected)
	}
	// 사람이 뒤집으면 따른다.
	no := false
	if err := s.FinanceSetFixed(ctx, finKey("Apple"), &no); err != nil {
		t.Fatal(err)
	}
	ov, _ = s.FinanceOverview(ctx, 0, "2026-08", now)
	for _, f := range ov.Fixed {
		if f.Label == "Apple" && f.Fixed {
			t.Fatal("고정 해제가 안 먹었다")
		}
	}
	// 월 가용 = 수입 기준 − 고정지출 − 목표 적립
	inc := int64(3_000_000)
	_ = s.FinanceSetSettings(ctx, &inc, nil)
	_, _ = s.FinanceSaveGoal(ctx, FinGoal{Name: "비상금", Target: 10_000_000, Monthly: 500_000, Items: []string{"MMF"}}, uid)
	ov, _ = s.FinanceOverview(ctx, 0, "2026-08", now)
	if ov.Plan.Spendable != 3_000_000-ov.Plan.FixedSpend-ov.Plan.FixedSave-500_000 {
		t.Fatalf("월 가용 = %+v", ov.Plan)
	}
	if ov.Goals[0].Current != 2_000_000 {
		t.Fatalf("목표 현재 금액 = %d", ov.Goals[0].Current)
	}
}

func TestFinanceIncomeSources(t *testing.T) {
	s, uid := newDiaryStore(t)
	ctx := context.Background()
	now, _ := time.Parse("2006-01-02", "2026-09-28")
	var tx []FinTx
	for m := 3; m <= 8; m++ {
		mon := time.Date(2026, time.Month(m), 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
		tx = append(tx,
			FinTx{At: mon + "-25T09:00", Type: "이체", Cat1: "저축", Content: "주식회사 우리회사", Amount: 3_000_000},
			FinTx{At: mon + "-03T09:00", Type: "이체", Cat1: "저축", Content: "MMF박스(7235)", Amount: 2_000_000}, // 비상금: 수입도 기타도 아니다
		)
	}
	tx = append(tx, FinTx{At: "2026-05-02T09:00", Type: "이체", Cat1: "현금", Content: "적금 해지", Amount: 5_000_000})
	if _, err := s.FinanceImport(ctx, uid, FinParsed{TakenAt: "2026-09-28", Tx: tx}, true, uid); err != nil {
		t.Fatal(err)
	}
	ov, err := s.FinanceOverview(ctx, uid, "", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ov.Months {
		if m.Month == "2026-05" && (m.In != 3_000_000 || m.OtherIn != 5_000_000) {
			t.Fatalf("5월 수입 %d, 기타 %d", m.In, m.OtherIn)
		}
	}
	if ov.Plan.IncomeHint != 0 && ov.Plan.IncomeHint != 3_000_000 {
		t.Fatalf("hint %d", ov.Plan.IncomeHint)
	}
	// 사람이 적금 해지를 수입으로 켜면 들어간다.
	on := true
	if err := s.FinanceSetIncome(ctx, finKey("적금 해지"), &on); err != nil {
		t.Fatal(err)
	}
	ov, _ = s.FinanceOverview(ctx, uid, "", now)
	for _, m := range ov.Months {
		if m.Month == "2026-05" && m.In != 8_000_000 {
			t.Fatalf("켠 뒤 5월 수입 %d", m.In)
		}
	}
}

func TestFinanceTags(t *testing.T) {
	s, uid := newDiaryStore(t)
	ctx := context.Background()
	now, _ := time.Parse("2006-01-02", "2026-09-28")
	tags, err := s.FinanceTags(ctx)
	if err != nil || len(tags) < 10 {
		t.Fatalf("기본 태그 %d %v", len(tags), err)
	}
	mine, err := s.FinanceCreateTag(ctx, "단아", "pink")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinanceCreateTag(ctx, "단아", "pink"); err == nil {
		t.Fatal("같은 이름이 또 만들어짐")
	}
	tx := []FinTx{
		{At: "2026-08-03T10:00", Type: "지출", Cat1: "육아", Content: "쿠팡", Amount: -50_000},
		{At: "2026-08-09T10:00", Type: "지출", Cat1: "육아", Content: "쿠팡", Amount: -30_000},
		{At: "2026-08-10T10:00", Type: "지출", Cat1: "식사", Content: "식당", Amount: -20_000},
	}
	if _, err := s.FinanceImport(ctx, uid, FinParsed{TakenAt: "2026-09-28", Tx: tx}, true, uid); err != nil {
		t.Fatal(err)
	}
	if err := s.FinanceSetTags(ctx, finKey("쿠팡"), []int64{mine.ID, tags[0].ID}); err != nil {
		t.Fatal(err)
	}
	ov, err := s.FinanceOverview(ctx, uid, "2026-08", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(ov.TagSpend) != 2 || ov.TagSpend[0].Amount != 80_000 || ov.TagSpend[0].Count != 2 || ov.Untagged != 20_000 {
		t.Fatalf("태그별 %+v, 태그 없음 %d", ov.TagSpend, ov.Untagged)
	}
	// 태그를 지우면 연결도 사라진다.
	if err := s.FinanceDeleteTag(ctx, mine.ID); err != nil {
		t.Fatal(err)
	}
	ov, _ = s.FinanceOverview(ctx, uid, "2026-08", now)
	if got := ov.TagLinks[finKey("쿠팡")]; len(got) != 1 {
		t.Fatalf("지운 뒤 %v", got)
	}
}

func TestFinanceLabelsMergeIncome(t *testing.T) {
	s, uid := newDiaryStore(t)
	ctx := context.Background()
	now, _ := time.Parse("2006-01-02", "2026-09-28")
	tx := []FinTx{
		{At: "2026-06-25T09:00", Type: "이체", Cat1: "저축", Content: "주식회사 우리회사", Amount: 3_000_000},
		{At: "2026-07-25T09:00", Type: "수입", Cat1: "급여", Content: "급여_2026.07.", Amount: 3_000_000},
		{At: "2026-08-25T09:00", Type: "수입", Cat1: "사업수입", Content: "우리회사_8월 급여", Amount: 3_100_000},
	}
	if _, err := s.FinanceImport(ctx, uid, FinParsed{TakenAt: "2026-09-28", Tx: tx}, true, uid); err != nil {
		t.Fatal(err)
	}
	keys := []string{finKey(tx[0].Content), finKey(tx[1].Content), finKey(tx[2].Content)}
	if err := s.FinanceSetLabel(ctx, keys, "급여", "급여"); err != nil {
		t.Fatal(err)
	}
	ov, err := s.FinanceOverview(ctx, uid, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(ov.Income) != 1 || len(ov.Income[0].Keys) != 3 || !ov.Income[0].Income || !ov.Income[0].Edited || ov.Income[0].Months != 3 || ov.Income[0].Cat1 != "급여" {
		t.Fatalf("묶이지 않음 %+v", ov.Income)
	}
	if !ov.Income[0].Fixed || ov.Plan.IncomeHint != 3_000_000 || len(ov.IncomeTx) != 0 {
		t.Fatalf("고정수입 %+v hint %d 예상외 %d", ov.Income[0], ov.Plan.IncomeHint, len(ov.IncomeTx))
	}
	// 고정에서 빼면 8월 급여가 예상 외 수입으로 간다.
	off := false
	for _, k := range keys {
		if err := s.FinanceSetFixed(ctx, k, &off); err != nil {
			t.Fatal(err)
		}
	}
	ov2, _ := s.FinanceOverview(ctx, uid, "2026-08", now)
	if ov2.Income[0].Fixed || len(ov2.IncomeTx) != 1 || ov2.IncomeTx[0].Amount != 3_100_000 {
		t.Fatalf("고정 뺀 뒤 %+v %+v", ov2.Income[0], ov2.IncomeTx)
	}
	for _, k := range keys {
		_ = s.FinanceSetFixed(ctx, k, nil)
	}
	d, err := s.FinanceDetail(ctx, uid, ov.Income[0].Keys, now)
	if err != nil || len(d.Tx) != 3 || len(d.Months) != 3 {
		t.Fatalf("상세 %+v %v", d, err)
	}
	// 비우면 원래대로 셋으로.
	if err := s.FinanceSetLabel(ctx, keys, "", ""); err != nil {
		t.Fatal(err)
	}
	ov, _ = s.FinanceOverview(ctx, uid, "", now)
	if len(ov.Income) != 3 {
		t.Fatalf("되돌린 뒤 %d", len(ov.Income))
	}
}

func TestFinanceNotSpend(t *testing.T) {
	s, uid := newDiaryStore(t)
	ctx := context.Background()
	now, _ := time.Parse("2006-01-02", "2026-09-28")
	tx := []FinTx{
		{At: "2026-08-03T10:00", Type: "이체", Cat1: "미분류", Content: "내 다른 통장", Amount: -1_000_000},
		{At: "2026-08-10T10:00", Type: "지출", Cat1: "식사", Content: "식당", Amount: -20_000},
	}
	if _, err := s.FinanceImport(ctx, uid, FinParsed{TakenAt: "2026-09-28", Tx: tx}, true, uid); err != nil {
		t.Fatal(err)
	}
	spendOf := func() int64 {
		ov, err := s.FinanceOverview(ctx, uid, "2026-08", now)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range ov.Months {
			if m.Month == "2026-08" {
				return m.Spend
			}
		}
		return -1
	}
	if got := spendOf(); got != 1_020_000 {
		t.Fatalf("처음 %d", got)
	}
	if err := s.FinanceSetSpend(ctx, finKey("내 다른 통장"), false); err != nil {
		t.Fatal(err)
	}
	if got := spendOf(); got != 20_000 {
		t.Fatalf("뺀 뒤 %d", got)
	}
	ov, _ := s.FinanceOverview(ctx, uid, "2026-08", now)
	if len(ov.NotSpend) != 1 || ov.NotSpend[0].Total != 1_000_000 {
		t.Fatalf("목록 %+v", ov.NotSpend)
	}
	_ = s.FinanceSetSpend(ctx, finKey("내 다른 통장"), true)
	if got := spendOf(); got != 1_020_000 {
		t.Fatalf("되돌린 뒤 %d", got)
	}
}
