package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 재정 내보내기(xlsx) 읽기. 시트·절·열 이름은 FinFormat(설정 파일)이 정한다.

type FinItem struct {
	Group       string   `json:"group"`
	Category    string   `json:"category"`
	Institution string   `json:"institution"`
	Name        string   `json:"name"`
	Amount      int64    `json:"amount"`
	Principal   *int64   `json:"principal"`
	Rate        *float64 `json:"rate"`
}

type FinTx struct {
	At      string `json:"at"`
	Type    string `json:"type"`
	Cat1    string `json:"cat1"`
	Cat2    string `json:"cat2"`
	Content string `json:"content"`
	Amount  int64  `json:"amount"`
	Method  string `json:"method"`
	Memo    string `json:"memo"`
}

type FinParsed struct {
	TakenAt string
	Items   []FinItem
	Tx      []FinTx
}

var finSection = regexp.MustCompile(`^(\d+)\.\s*(\S+)`)
var finFileDate = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})\D*\.xlsx$`)

func finNum(s string) (float64, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

func finWon(s string) int64 {
	f, _ := finNum(s)
	return int64(math.Round(f))
}

// excelDateTime: 일련번호(1899-12-30 기준) + 하루 비율.
func excelDateTime(day, frac string) (string, error) {
	d, ok := finNum(day)
	if !ok {
		return "", fmt.Errorf("날짜가 아니에요: %q", day)
	}
	f, _ := finNum(frac)
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	t := base.AddDate(0, 0, int(d)).Add(time.Duration(math.Round(f*1440)) * time.Minute)
	return t.Format("2006-01-02T15:04"), nil
}

func formatErr(format string, a ...any) error {
	return invalid("파일 형식이 설정된 규격과 달라요 — " + fmt.Sprintf(format, a...))
}

// ParseFinanceExport 는 파일을 읽어 스냅샷 항목과 거래를 낸다. filename 에서 기준일을 뽑고, 없으면 오늘.
func ParseFinanceExport(data []byte, filename string, f *FinFormat) (FinParsed, error) {
	if f == nil {
		return FinParsed{}, invalid("재정 파일 규격이 설정되지 않았어요(PLANNER_FIN_FORMAT)")
	}
	sheets, order, err := readXLSX(data)
	if err != nil {
		return FinParsed{}, err
	}
	sum, ok := sheets[f.SummarySheet]
	if !ok {
		return FinParsed{}, formatErr("요약 시트 '%s'가 없어요(있는 시트: %s)", f.SummarySheet, strings.Join(order, ", "))
	}
	out := FinParsed{TakenAt: time.Now().Format("2006-01-02")}
	if m := finFileDate.FindStringSubmatch(filename); m != nil {
		out.TakenAt = m[1]
	}

	sections := map[string]int{}
	for i, r := range sum {
		if m := finSection.FindStringSubmatch(r["B"]); m != nil {
			sections[m[2]] = i
		}
	}
	start, ok := sections[f.BalanceSection]
	if !ok {
		return FinParsed{}, formatErr("요약 시트에 '%s' 절이 없어요", f.BalanceSection)
	}
	// 자산 쪽: B 분류(비면 위 줄 이어받음) · C 이름 · E 금액 / 부채 쪽: F · G · I
	var aCat, dCat string
	ended := false
	for i := start + 1; i < len(sum); i++ {
		r := sum[i]
		if strings.HasPrefix(r["B"], f.BalanceEnd) {
			ended = true
			break
		}
		if finSection.MatchString(r["B"]) {
			break
		}
		if r["B"] != "" && r["C"] == "" && r["E"] == "" && r["F"] != "" && r["G"] == "" {
			continue // '자산 | 부채' 같은 표 제목 줄
		}
		if r["C"] != "" && r["E"] != "" {
			if _, isNum := finNum(r["E"]); !isNum {
				continue // 머리줄
			}
		}
		if r["B"] != "" {
			aCat = r["B"]
		}
		if r["F"] != "" {
			dCat = r["F"]
		}
		if _, isNum := finNum(r["E"]); r["C"] != "" && isNum {
			g := f.AssetGroups[aCat]
			if g == "" {
				g = "other"
			}
			out.Items = append(out.Items, FinItem{Group: g, Category: aCat, Name: r["C"], Amount: finWon(r["E"])})
		}
		if _, isNum := finNum(r["I"]); r["G"] != "" && isNum {
			out.Items = append(out.Items, FinItem{Group: "debt", Category: dCat, Name: r["G"], Amount: finWon(r["I"])})
		}
	}
	if !ended {
		return FinParsed{}, formatErr("'%s' 절의 끝('%s…' 줄)을 못 찾았어요", f.BalanceSection, f.BalanceEnd)
	}

	// 투자·대출 절: C 기관 · D 이름 · F 원금 · H 수익률(금리). 같은 이름의 항목에 붙인다.
	enrich := func(section, group string) {
		start, ok := sections[section]
		if section == "" || !ok {
			return
		}
		for i := start + 3; i < len(sum); i++ {
			r := sum[i]
			if finSection.MatchString(r["B"]) {
				break
			}
			name := r["D"]
			if name == "" {
				continue
			}
			for k := range out.Items {
				it := &out.Items[k]
				if it.Group != group || it.Name != name || it.Institution != "" {
					continue
				}
				it.Institution = r["C"]
				if v, ok := finNum(r["F"]); ok {
					p := int64(math.Round(v))
					it.Principal = &p
				}
				if v, ok := finNum(r["H"]); ok {
					it.Rate = &v
				}
				break
			}
		}
	}
	enrich(f.InvestSection, "invest")
	enrich(f.LoanSection, "debt")

	rows, ok := sheets[f.TxSheet]
	if !ok {
		return FinParsed{}, formatErr("거래 시트 '%s'가 없어요(있는 시트: %s)", f.TxSheet, strings.Join(order, ", "))
	}
	if len(rows) > 0 {
		var diff []string
		for col, want := range f.TxHeader {
			if got := strings.TrimSpace(rows[0][col]); got != want {
				diff = append(diff, fmt.Sprintf("%s열 '%s'(기대 '%s')", col, got, want))
			}
		}
		if len(diff) > 0 {
			return FinParsed{}, formatErr("거래 시트 머리줄이 달라요: %s", strings.Join(diff, ", "))
		}
	}
	c := func(h string) string { return f.txCol(h) }
	for i, r := range rows {
		if i == 0 || r[c("날짜")] == "" {
			continue
		}
		at, err := excelDateTime(r[c("날짜")], r[c("시간")])
		if err != nil {
			continue
		}
		typ := r[c("타입")]
		switch typ {
		case f.TypeIn:
			typ = "수입"
		case f.TypeOut:
			typ = "지출"
		case f.TypeTransfer:
			typ = "이체"
		default:
			return FinParsed{}, formatErr("거래 %d번째 줄의 타입 '%s'를 모르겠어요", i+1, typ)
		}
		out.Tx = append(out.Tx, FinTx{
			At: at, Type: typ, Cat1: r[c("대분류")], Cat2: r[c("소분류")], Content: r[c("내용")],
			Amount: finWon(r[c("금액")]), Method: r[c("결제수단")], Memo: r[c("메모")],
		})
	}
	if len(out.Items) == 0 && len(out.Tx) == 0 {
		return FinParsed{}, invalid("읽을 수 있는 내용이 없어요")
	}
	return out, nil
}

func finTxHash(t FinTx, n int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%d|%s|%d", t.At, t.Type, t.Content, t.Amount, t.Method, n)))
	return hex.EncodeToString(h[:12])
}

// FinImportResult 는 미리보기이자 결과다.
type FinImportResult struct {
	TakenAt     string           `json:"taken_at"`
	Items       int              `json:"items"`
	Groups      map[string]int64 `json:"groups"`
	TotalAsset  int64            `json:"total_asset"`
	TotalDebt   int64            `json:"total_debt"`
	TxTotal     int              `json:"tx_total"`
	TxNew       int              `json:"tx_new"`
	TxFrom      string           `json:"tx_from"`
	TxTo        string           `json:"tx_to"`
	ReplaceSnap bool             `json:"replace_snapshot"` // 같은 날 스냅샷이 있어 바꾼다
}

// FinanceImport 는 apply=false 면 세기만, true 면 넣는다. 거래는 (사람, 내용 해시)로 겹침을 거른다.
// 같은 시각·내용·금액 거래가 한 파일에 여럿이면 순번을 해시에 넣어 모두 살린다.
func (s *Store) FinanceImport(ctx context.Context, ownerID int64, p FinParsed, apply bool, by int64) (FinImportResult, error) {
	res := FinImportResult{TakenAt: p.TakenAt, Items: len(p.Items), Groups: map[string]int64{}, TxTotal: len(p.Tx)}
	for _, it := range p.Items {
		if it.Group == "debt" {
			res.TotalDebt += it.Amount
		} else {
			res.TotalAsset += it.Amount
		}
		res.Groups[it.Group] += it.Amount
	}
	seen := map[string]int{}
	hashes := make([]string, len(p.Tx))
	for i, t := range p.Tx {
		k := fmt.Sprintf("%s|%s|%s|%d|%s", t.At, t.Type, t.Content, t.Amount, t.Method)
		seen[k]++
		hashes[i] = finTxHash(t, seen[k])
		if res.TxFrom == "" || t.At < res.TxFrom {
			res.TxFrom = t.At
		}
		if t.At > res.TxTo {
			res.TxTo = t.At
		}
	}

	existing := map[string]bool{}
	rows, err := s.db.QueryContext(ctx, `SELECT hash FROM fin_tx WHERE owner_id=?`, ownerID)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			rows.Close()
			return res, err
		}
		existing[h] = true
	}
	rows.Close()
	for _, h := range hashes {
		if !existing[h] {
			res.TxNew++
		}
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM fin_snapshots WHERE owner_id=? AND taken_at=?`, ownerID, p.TakenAt).Scan(&n); err != nil {
		return res, err
	}
	res.ReplaceSnap = n > 0
	if !apply {
		return res, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	if len(p.Items) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM fin_snapshots WHERE owner_id=? AND taken_at=?`, ownerID, p.TakenAt); err != nil {
			return res, err
		}
		r, err := tx.ExecContext(ctx,
			`INSERT INTO fin_snapshots (owner_id, taken_at, total_asset, total_debt, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			ownerID, p.TakenAt, res.TotalAsset, res.TotalDebt, by, time.Now().Unix())
		if err != nil {
			return res, err
		}
		sid, _ := r.LastInsertId()
		for _, it := range p.Items {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO fin_items (snapshot_id, group_, category, institution, name, amount, principal, rate) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				sid, it.Group, it.Category, it.Institution, it.Name, it.Amount, it.Principal, it.Rate); err != nil {
				return res, err
			}
		}
	}
	for i, t := range p.Tx {
		if existing[hashes[i]] {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO fin_tx (owner_id, at, type, cat1, cat2, content, amount, method, memo, hash)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(owner_id, hash) DO NOTHING`,
			ownerID, t.At, t.Type, t.Cat1, t.Cat2, t.Content, t.Amount, t.Method, t.Memo, hashes[i]); err != nil {
			return res, err
		}
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}
	// 업로드 알림이 없으면 하나 깔아둔다(매일 09:00, 올린 사람 본인에게). 알림 화면에서 끄거나 고칠 수 있다.
	var n2 int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM notify_rules WHERE kind='finance'`).Scan(&n2); err == nil && n2 == 0 {
		_, _ = s.NotifySave(ctx, NotifyRule{Kind: "finance", DaysMask: 127, Times: []string{"09:00"}, Enabled: true,
			Params: NotifyParams{Cond: "since", Amount: 30, Unit: "days"}}, by)
	}
	return res, nil
}

// finPairTransfers 는 계좌 사이 이동(같은 금액이 나가고 들어온 이체 한 쌍)을 찾는다.
// 짝이 맞으면 내 돈이 자리만 옮긴 것이라 수입·지출 어디에도 넣지 않는다.
// 시각은 몇 분 어긋날 수 있어 ±10분 안에서 맞춘다.
func finPairTransfers(tx []FinTx) map[int]bool {
	paired := map[int]bool{}
	for i := range finPairs(tx) {
		paired[i] = true
	}
	return paired
}

// finPairs 는 짝이 맞는 이체의 서로의 인덱스(나간 줄 ↔ 들어온 줄).
func finPairs(tx []FinTx) map[int]int {
	outs := map[int64][]int{}
	for i, t := range tx {
		if t.Type == "이체" && t.Amount < 0 {
			outs[-t.Amount] = append(outs[-t.Amount], i)
		}
	}
	paired := map[int]int{}
	minutes := func(at string) int64 {
		tm, err := time.Parse("2006-01-02T15:04", at)
		if err != nil {
			return 0
		}
		return tm.Unix() / 60
	}
	for i, t := range tx {
		if t.Type != "이체" || t.Amount <= 0 {
			continue
		}
		m := minutes(t.At)
		for _, j := range outs[t.Amount] {
			if _, used := paired[j]; used {
				continue
			}
			d := minutes(tx[j].At) - m
			if d >= -10 && d <= 10 {
				paired[i], paired[j] = j, i
				break
			}
		}
	}
	return paired
}
