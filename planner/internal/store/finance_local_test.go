package store

import (
	"context"
	"os"
	"testing"
	"time"
)

// 실제 파일로 확인할 때만 돈다: FIN_XLSX=파일.xlsx FIN_FORMAT=규격.json go test -run TestFinanceRealFile
func TestFinanceRealFile(t *testing.T) {
	path := os.Getenv("FIN_XLSX")
	if path == "" {
		t.Skip("FIN_XLSX 없음")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ff, err := LoadFinFormat(os.Getenv("FIN_FORMAT"))
	if err != nil || ff == nil {
		t.Fatalf("FIN_FORMAT: %v", err)
	}
	p, err := ParseFinanceExport(b, path, ff)
	if err != nil {
		t.Fatal(err)
	}
	groups := map[string]int{}
	withInst := 0
	for _, it := range p.Items {
		groups[it.Group]++
		if it.Institution != "" {
			withInst++
		}
	}
	types := map[string]int{}
	for _, x := range p.Tx {
		types[x.Type]++
	}
	t.Logf("기준일 %s | 항목 %d (그룹별 개수 %v, 기관 붙은 것 %d) | 거래 %d %v %s~%s",
		p.TakenAt, len(p.Items), groups, withInst, len(p.Tx), types, p.Tx[len(p.Tx)-1].At, p.Tx[0].At)
	s, uid := newDiaryStore(t)
	ctx := context.Background()
	r1, err := s.FinanceImport(ctx, uid, p, true, uid)
	if err != nil {
		t.Fatal(err)
	}
	r2, _ := s.FinanceImport(ctx, uid, p, false, uid)
	t.Logf("첫 가져오기 새 거래 %d / 다시 하면 새 거래 %d, 같은 날 스냅샷 교체 %v", r1.TxNew, r2.TxNew, r2.ReplaceSnap)

	now, _ := time.Parse("2006-01-02", "2026-09-28")
	ov, err := s.FinanceOverview(ctx, 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	// 금액은 로그에 남기지 않는다 — 비율과 개수만.
	t.Logf("고정 후보 %d개 (저축형 %d), 예상 외(%s) %d건, 달 %d개", len(ov.Fixed), countKind(ov.Fixed, "save"), ov.Month, len(ov.Unexpected), len(ov.Months))
	for _, f := range ov.Fixed[:min(12, len(ov.Fixed))] {
		t.Logf("  고정 %s [%s/%s] %d달", f.Label, f.Cat1, f.Kind, f.Months)
	}
	for _, u := range ov.Unexpected[:min(8, len(ov.Unexpected))] {
		t.Logf("  예상 외 %s %s — %s", u.At[:10], u.Content, string([]rune(u.Reason)[:min(len([]rune(u.Reason)), 14)]))
	}
	for _, m := range ov.Months {
		ratio := 0.0
		if m.In > 0 {
			ratio = float64(m.Spend) / float64(m.In)
		}
		t.Logf("  %s 쓴 돈/들어온 돈 %.2f, 고정 비중 %.2f", m.Month, ratio, float64(m.Fixed)/float64(max(m.Spend, 1)))
	}
}

func countKind(fs []FinFixed, k string) int {
	n := 0
	for _, f := range fs {
		if f.Kind == k {
			n++
		}
	}
	return n
}
