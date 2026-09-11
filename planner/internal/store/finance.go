package store

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 재정 분석. 돈의 흐름 정의:
//   수입      = 수입원으로 본 것만. '수입' 타입은 기본으로, 이체로 들어온 건 매달 오는 것(월급이
//               '이체'로 오는 경우)만 기본으로. 적금 해지·파킹통장 인출 같은 건 내 돈이 돌아온 것이라 뺀다.
//               사람이 수입원을 켜고 끌 수 있다(fin_income).
//   쓴 돈     = 지출(환불은 뺌) + 짝 없이 나간 이체 중 저축·투자가 아닌 것
//   저축      = 저축·투자로 나간 이체(내 계좌로 옮긴 것 포함)
// 짝이 맞는 이체(같은 금액이 ±10분 안에 나가고 들어옴)는 내 계좌끼리의 이동이라 뺀다.
// 합산해서 볼 때는 두 사람 사이 이체도 짝으로 맞춰 뺀다.

type FinItemView struct {
	OwnerID int64 `json:"owner_id"`
	FinItem
}

type FinMonth struct {
	Month      string `json:"month"`
	In         int64  `json:"in"`
	Spend      int64  `json:"spend"`
	Save       int64  `json:"save"`
	Fixed      int64  `json:"fixed"`      // 쓴 돈 중 고정지출
	Variable   int64  `json:"variable"`   // 나머지
	OtherIn    int64  `json:"other_in"`   // 수입이 아닌 들어온 돈(적금 해지 등)
	FixedIn    int64  `json:"fixed_in"`   // 수입 중 고정수입
	FixedSave  int64  `json:"fixed_save"` // 저축 중 고정(적금 자동이체 등)
	Unexpected int64  `json:"unexpected"` // 쓴 돈 중 예상 외 지출
	Wallet     int64  `json:"wallet"`     // 쓴 돈 중 지역화폐로 쓴 것
	Partial    bool   `json:"partial"`    // 이번 달(아직 안 끝남)
}

type FinFixed struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Cat1     string `json:"cat1"`
	Kind     string `json:"kind"` // spend | save
	Monthly  int64  `json:"monthly"`
	Months   int    `json:"months"` // 최근 6개월 중 나온 달 수
	Auto     bool   `json:"auto"`   // 자동 판정
	Fixed    bool   `json:"fixed"`  // 최종(사람이 뒤집었으면 그 값)
	Override *bool  `json:"override"`
	Edited   bool   `json:"edited"` // 이름·분류를 사람이 고침
}

type FinTagSpend struct {
	TagID  int64 `json:"tag_id"`
	Amount int64 `json:"amount"`
	Count  int   `json:"count"`
}

type FinIncome struct {
	Key      string   `json:"key"`
	Keys     []string `json:"keys"` // 같은 이름으로 고쳐 묶인 항목들
	Edited   bool     `json:"edited"`
	Label    string   `json:"label"`
	Cat1     string   `json:"cat1"`
	Monthly  int64    `json:"monthly"` // 들어온 달의 중앙값
	Months   int      `json:"months"`  // 최근 12개월 중 들어온 달 수
	Total    int64    `json:"total"`
	Auto     bool     `json:"auto"`
	Income   bool     `json:"income"`
	Override *bool    `json:"override"`
	// 고정수입 판정. 수입이 아니면 의미 없다.
	Months6       int   `json:"months6"`
	Fixed         bool  `json:"fixed"`
	FixedAuto     bool  `json:"fixed_auto"`
	FixedOverride *bool `json:"fixed_override"`
}

// FinRegular 는 평소 지출 한 건.
type FinRegular struct {
	Key     string `json:"key"`
	Ref     string `json:"ref"`
	Edited  bool   `json:"edited"`
	At      string `json:"at"`
	Content string `json:"content"`
	Cat1    string `json:"cat1"`
	Amount  int64  `json:"amount"`
	Wallet  bool   `json:"wallet"` // 지역화폐로 쓴 것
}

// FinNotSpend 는 '지출 아님'으로 뺀 항목(최근 13개월 합).
type FinNotSpend struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Cat1   string `json:"cat1"`
	Total  int64  `json:"total"`
	Count  int    `json:"count"`
	Edited bool   `json:"edited"`
}

// FinIncomeTx 는 그 달의 예상 외 수입(고정수입이 아닌 수입) 한 건.
type FinIncomeTx struct {
	Group   string   `json:"group"` // FinIncome.Key
	Ref     string   `json:"ref"`
	Key     string   `json:"key"` // 이 거래의 항목 key
	Keys    []string `json:"keys"`
	At      string   `json:"at"`
	Content string   `json:"content"`
	Cat1    string   `json:"cat1"`
	Amount  int64    `json:"amount"`
	Edited  bool     `json:"edited"`
}

type FinUnexpected struct {
	Key     string `json:"key"`
	Ref     string `json:"ref"`
	At      string `json:"at"`
	Content string `json:"content"`
	Cat1    string `json:"cat1"`
	Amount  int64  `json:"amount"`
	Reason  string `json:"reason"`
	Edited  bool   `json:"edited"`
}

type FinPlan struct {
	IncomeBase   int64 `json:"income_base"`   // 사람이 정한 월 수입(0 이면 아직 안 정함)
	IncomeHint   int64 `json:"income_hint"`   // 고정수입 합(없으면 달별 수입의 중앙값)
	FixedIncome  int64 `json:"fixed_income"`  // 최근 6개월, 달마다 들어온 고정수입 합의 중앙값
	FixedSpend   int64 `json:"fixed_spend"`   // 고정지출 월 합
	FixedSave    int64 `json:"fixed_save"`    // 고정 저축 월 합
	GoalsMonthly int64 `json:"goals_monthly"` // 목표 월 적립 합
	Spendable    int64 `json:"spendable"`     // 월에 자유롭게 쓸 수 있는 돈
	VariableAvg  int64 `json:"variable_avg"`  // 최근 3개월 변동지출 평균(예상 외 지출 뺌, 비교용)
	BufferMonths int   `json:"buffer_months"`
	Liquid       int64 `json:"liquid"`    // 바로 쓸 수 있는 돈(입출금·현금·전자금융)
	SpendAvg     int64 `json:"spend_avg"` // 최근 6개월 평소 생활비 평균(쓴 돈 - 예상 외 지출)
	Emergency    int64 `json:"emergency"` // 비상금: MMF·파킹 자산(투자·저축 묶음에 있어도)
	Free         int64 `json:"free"`      // 여유자금 = Liquid + Emergency - SpendAvg × BufferMonths
	// 이번 달 진행: 쓸 수 있는 돈에서 이번 달 변동지출(지역화폐 포함)을 뺀 것.
	MonthSpent      int64 `json:"month_spent"`
	MonthWallet     int64 `json:"month_wallet"`
	MonthUnexpected int64 `json:"month_unexpected"` // 이번 달 예상 외(남은 돈 계산에선 뺀다)
	MonthLeft       int64 `json:"month_left"`
	DaysPassed      int   `json:"days_passed"`
	DaysInMonth     int   `json:"days_in_month"`
}

type FinGoal struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Target  int64    `json:"target"`
	Due     *string  `json:"due"`
	Monthly int64    `json:"monthly"`
	Items   []string `json:"items"`
	Current int64    `json:"current"`
}

type FinOverview struct {
	Owners     []int64          `json:"owners"`
	AsOf       map[int64]string `json:"as_of"`
	TotalAsset int64            `json:"total_asset"`
	TotalDebt  int64            `json:"total_debt"`
	NetWorth   int64            `json:"net_worth"`
	Groups     map[string]int64 `json:"groups"`
	Items      []FinItemView    `json:"items"`
	History    []struct {
		Month    string `json:"month"`
		NetWorth int64  `json:"net_worth"`
	} `json:"history"`
	Months     []FinMonth          `json:"months"`
	Fixed      []FinFixed          `json:"fixed"`
	Income     []FinIncome         `json:"income"`
	IncomeTx   []FinIncomeTx       `json:"income_tx"` // month 의 예상 외 수입
	NotSpend   []FinNotSpend       `json:"not_spend"`
	Wallets    []FinWallet         `json:"wallets"` // 충전식 지갑(지역화폐 등). 잔액은 자산(바로 쓸 수 있는 돈)에 더한다
	Tags       []FinTag            `json:"tags"`
	Labels     map[string]FinLabel `json:"labels"`      // 사람이 고친 이름·분류
	TagLinks   map[string][]int64  `json:"tag_links"`   // 항목 key → 태그 id
	TxTags     map[string][]int64  `json:"tx_tags"`     // 거래(ref) → 태그 id. 목록에 나온 거래만
	KeyTxTags  map[string][]int64  `json:"key_tx_tags"` // 항목 key → 그 이름의 다른 거래에 썼던 태그(불러오기 제안용)
	TagSpend   []FinTagSpend       `json:"tag_spend"`   // month 의 태그별 쓴 돈. 한 거래가 태그 여러 개면 각각에 센다
	Untagged   int64               `json:"untagged"`    // month 에 쓴 돈 중 태그 없는 것
	Month      string              `json:"month"`       // 예상 외 지출을 본 달
	Unexpected []FinUnexpected     `json:"unexpected"`
	Regular    []FinRegular        `json:"regular"` // month 의 평소 지출
	Plan       FinPlan             `json:"plan"`
	Goals      []FinGoal           `json:"goals"`
	TxCount    int                 `json:"tx_count"`
	// 업로드 주기와 사람별 마지막 업로드(유닉스 초). 올린 적 없는 가족은 빠진다.
	UploadDays int             `json:"upload_days"`
	Uploaded   map[int64]int64 `json:"uploaded"`
}

type finRow struct {
	FinTx
	owner  int64
	kind   string // in | spend | save | skip
	key    string // finKey(Content)
	label  string // 사람이 고친 이름(없으면 Content)
	orig   string // 파일의 분류(Cat1 은 고친 값으로 바뀐다)
	wallet bool   // 지역화폐로 쓴 것(직접 적은 기록)
	ref    string // 태그가 붙는 거래: 't<fin_tx.id>' | 'w<지갑 사용 id>'
}

var finKeyStrip = regexp.MustCompile(`[0-9\s()\[\]_\-.,*/]+|청구$`)

func finKey(content string) string {
	k := strings.ToLower(finKeyStrip.ReplaceAllString(strings.TrimSuffix(content, "_청구"), ""))
	if k == "" {
		return strings.ToLower(strings.TrimSpace(content))
	}
	return k
}

func median(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	c := append([]int64(nil), xs...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	return c[len(c)/2]
}

func (s *Store) finSettings(ctx context.Context) (income int64, buffer int) {
	buffer = 3
	if v, ok, _ := s.KVGet(ctx, "fin.income"); ok {
		income, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, ok, _ := s.KVGet(ctx, "fin.buffer"); ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 24 {
			buffer = n
		}
	}
	return
}

// finUploadDays: 재정 파일 업로드 주기(일) — 켜 둔 '재정 파일' 알림 중 가장 짧은 것, 없으면 30.
func (s *Store) finUploadDays(ctx context.Context) int {
	rules, err := s.NotifyRules(ctx)
	if err != nil {
		return 30
	}
	best := 0
	for _, r := range rules {
		if !r.Enabled || r.Kind != "finance" || r.Params.Cond != "since" {
			continue
		}
		d := int(r.Params.sinceDur().Hours() / 24)
		if d >= 1 && (best == 0 || d < best) {
			best = d
		}
	}
	if best == 0 {
		return 30
	}
	return best
}

// FinanceSetSettings 는 월 수입 기준과 비상금 개월 수를 정한다.
func (s *Store) FinanceSetSettings(ctx context.Context, income *int64, buffer *int) error {
	if income != nil {
		if *income < 0 {
			return invalid("수입은 0 이상이어야 해요")
		}
		if err := s.KVSet(ctx, "fin.income", strconv.FormatInt(*income, 10)); err != nil {
			return err
		}
	}
	if buffer != nil {
		if *buffer < 0 || *buffer > 24 {
			return invalid("0~24개월 사이로 정해주세요")
		}
		if err := s.KVSet(ctx, "fin.buffer", strconv.Itoa(*buffer)); err != nil {
			return err
		}
	}
	return nil
}

var (
	finSaveRe = regexp.MustCompile(`(?i)적금|예금|청약|펀드|연금|ISA|IRP|저금통|납입|저축|보험|해상|화재|생명|손보|라이프`)
	// '롯데카드', '신한카드', '스마일카드 대금' 같은 카드값 이체.
	finCardBill = regexp.MustCompile(`^\s*(\S{1,8})카드\s*(대금)?\s*$`)
	// '네이버페이충전', '카카오페이 충전' 같은 간편결제 충전.
	finPayCharge = regexp.MustCompile(`^\s*(\S+페이)\s*충전\s*$`)
	// 결제수단이 영어로 오는 카드.
	finCardEn = map[string]string{"스마일": "smile", "삼성": "samsung", "현대": "hyundai", "국민": "kb", "하나": "hana", "우리": "woori", "농협": "nh"}
	// 계좌(결제수단) 이름으로 보는 저축 상품. '저축예금'은 보통 입출금 통장이라 넣지 않는다.
	finSaveAcct = regexp.MustCompile(`(?i)적금|청약|도약|펀드|연금|ISA|IRP|정기예금|정기예탁|주택드림`)
	finParkRe   = regexp.MustCompile(`(?i)박스|MMF|CMA|파킹|여윳돈`)
)

// 내 돈이 돌아온 것(적금·예금 해지, 파킹통장·증권 인출)은 매달 와도 수입이 아니다.
var finOwnMoney = regexp.MustCompile(`해지|적금|예금|박스|MMF|CMA|파킹|환급|이자`)

// FinanceSetSpend 는 '지출 아님'을 켜고 끈다(spend=false 면 뺀다).
func (s *Store) FinanceSetSpend(ctx context.Context, key string, spend bool) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return invalid("항목이 비었어요")
	}
	var err error
	if spend {
		_, err = s.db.ExecContext(ctx, `DELETE FROM fin_not_spend WHERE key=?`, key)
	} else {
		_, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO fin_not_spend (key) VALUES (?)`, key)
	}
	return err
}

// FinanceSetIncome 는 수입원 판정을 뒤집는다. income 이 nil 이면 자동으로 되돌린다.
func (s *Store) FinanceSetIncome(ctx context.Context, key string, income *bool) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return invalid("항목이 비었어요")
	}
	if income == nil {
		_, err := s.db.ExecContext(ctx, `DELETE FROM fin_income WHERE key=?`, key)
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO fin_income (key, income) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET income=excluded.income`, key, boolInt(*income))
	return err
}

// FinanceSetRegular 는 항목을 평소 지출로 둔다 — 고정도 아니고, 금액이 커도 예상 외로 보지 않는다.
func (s *Store) FinanceSetRegular(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return invalid("항목이 비었어요")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO fin_fixed (key, fixed) VALUES (?, -1) ON CONFLICT(key) DO UPDATE SET fixed=-1`, key)
	return err
}

// FinanceSetFixed 는 고정지출 판정을 뒤집는다. fixed 가 nil 이면 자동으로 되돌린다.
func (s *Store) FinanceSetFixed(ctx context.Context, key string, fixed *bool) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return invalid("항목이 비었어요")
	}
	if fixed == nil {
		_, err := s.db.ExecContext(ctx, `DELETE FROM fin_fixed WHERE key=?`, key)
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO fin_fixed (key, fixed) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET fixed=excluded.fixed`, key, boolInt(*fixed))
	return err
}

// FinanceOverview 는 owner(0 이면 가족 전체)의 재정 요약이다. month 는 예상 외 지출을 볼 달('YYYY-MM', 비면 지난달).
func (s *Store) FinanceOverview(ctx context.Context, owner int64, month string, now time.Time) (FinOverview, error) {
	return s.financeOverview(ctx, owner, month, now, false)
}

// financeOverview: allMonths 면 평소·예상 외 지출과 예상 외 수입 목록을 한 달이 아니라 모든 달로 채운다(미분류 보기).
func (s *Store) financeOverview(ctx context.Context, owner int64, month string, now time.Time, allMonths bool) (FinOverview, error) {
	out := FinOverview{AsOf: map[int64]string{}, Groups: map[string]int64{}, Items: []FinItemView{}, Fixed: []FinFixed{}, Unexpected: []FinUnexpected{}, Regular: []FinRegular{}, Income: []FinIncome{}, IncomeTx: []FinIncomeTx{}, NotSpend: []FinNotSpend{}, Goals: []FinGoal{}, Uploaded: map[int64]int64{}, Wallets: []FinWallet{}}
	out.UploadDays = s.finUploadDays(ctx)
	tags, err := s.FinanceTags(ctx)
	if err != nil {
		return out, err
	}
	links, err := s.finTagLinks(ctx)
	if err != nil {
		return out, err
	}
	out.Tags, out.TagLinks, out.TagSpend = tags, links, []FinTagSpend{}
	if err := s.finMigrateTxTags(ctx, links); err != nil {
		return out, err
	}
	txTags, err := s.finTxTags(ctx)
	if err != nil {
		return out, err
	}
	out.TxTags, out.KeyTxTags = map[string][]int64{}, map[string][]int64{}
	labels, err := s.finLabels(ctx)
	if err != nil {
		return out, err
	}
	out.Labels = labels
	urows, err := s.db.QueryContext(ctx, `SELECT owner_id, max(created_at) FROM fin_snapshots GROUP BY owner_id`)
	if err != nil {
		return out, err
	}
	for urows.Next() {
		var o, at int64
		if err := urows.Scan(&o, &at); err != nil {
			urows.Close()
			return out, err
		}
		out.Uploaded[o] = at
	}
	urows.Close()

	// 1) 스냅샷: 사람마다 가장 최근 것.
	type snap struct {
		id, owner int64
		at        string
	}
	var snaps []snap
	rows, err := s.db.QueryContext(ctx, `SELECT id, owner_id, taken_at FROM fin_snapshots ORDER BY taken_at`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var x snap
		if err := rows.Scan(&x.id, &x.owner, &x.at); err != nil {
			rows.Close()
			return out, err
		}
		if owner == 0 || x.owner == owner {
			snaps = append(snaps, x)
		}
	}
	rows.Close()
	latest := map[int64]snap{}
	for _, x := range snaps {
		latest[x.owner] = x
	}
	for o, x := range latest {
		out.Owners = append(out.Owners, o)
		out.AsOf[o] = x.at
	}
	sort.Slice(out.Owners, func(i, j int) bool { return out.Owners[i] < out.Owners[j] })

	for _, o := range out.Owners {
		irows, err := s.db.QueryContext(ctx,
			`SELECT group_, category, institution, name, amount, principal, rate FROM fin_items WHERE snapshot_id=? ORDER BY group_, amount DESC`, latest[o].id)
		if err != nil {
			return out, err
		}
		for irows.Next() {
			var it FinItemView
			it.OwnerID = o
			if err := irows.Scan(&it.Group, &it.Category, &it.Institution, &it.Name, &it.Amount, &it.Principal, &it.Rate); err != nil {
				irows.Close()
				return out, err
			}
			out.Items = append(out.Items, it)
			out.Groups[it.Group] += it.Amount
			if it.Group == "debt" {
				out.TotalDebt += it.Amount
			} else {
				out.TotalAsset += it.Amount
			}
		}
		irows.Close()
	}
	// 지갑 잔액은 파일에 없다 — 바로 쓸 수 있는 돈으로 더한다.
	wallets, err := s.FinanceWallets(ctx, owner, now)
	if err != nil {
		return out, err
	}
	out.Wallets = wallets
	for _, w := range wallets {
		out.Items = append(out.Items, FinItemView{OwnerID: w.OwnerID, FinItem: FinItem{Group: "liquid", Category: "지역화폐", Name: w.Name + " (직접 관리)", Amount: w.Balance}})
		out.Groups["liquid"] += w.Balance
		out.TotalAsset += w.Balance
	}
	out.NetWorth = out.TotalAsset - out.TotalDebt

	// 순자산 추이: 달마다 그 달 말까지의 사람별 최신 스냅샷을 더한다.
	totals := map[int64]int64{}
	trows, err := s.db.QueryContext(ctx, `SELECT id, total_asset - total_debt FROM fin_snapshots`)
	if err != nil {
		return out, err
	}
	for trows.Next() {
		var id, v int64
		if err := trows.Scan(&id, &v); err != nil {
			trows.Close()
			return out, err
		}
		totals[id] = v
	}
	trows.Close()
	months := map[string]bool{}
	for _, x := range snaps {
		months[x.at[:7]] = true
	}
	var ms []string
	for m := range months {
		ms = append(ms, m)
	}
	sort.Strings(ms)
	for _, m := range ms {
		cur := map[int64]snap{}
		for _, x := range snaps {
			if x.at[:7] <= m {
				cur[x.owner] = x
			}
		}
		var sum int64
		for _, x := range cur {
			sum += totals[x.id]
		}
		out.History = append(out.History, struct {
			Month    string `json:"month"`
			NetWorth int64  `json:"net_worth"`
		}{m, sum})
	}

	// 2) 거래: 최근 13개월.
	from := now.AddDate(0, -13, 0).Format("2006-01") + "-01T00:00"
	q := `SELECT id, owner_id, at, type, cat1, cat2, content, amount, method, memo FROM fin_tx WHERE at >= ?`
	args := []any{from}
	if owner != 0 {
		q += ` AND owner_id = ?`
		args = append(args, owner)
	}
	xrows, err := s.db.QueryContext(ctx, q+` ORDER BY at`, args...)
	if err != nil {
		return out, err
	}
	var all []finRow
	for xrows.Next() {
		var r finRow
		var id int64
		if err := xrows.Scan(&id, &r.owner, &r.At, &r.Type, &r.Cat1, &r.Cat2, &r.Content, &r.Amount, &r.Method, &r.Memo); err != nil {
			xrows.Close()
			return out, err
		}
		r.ref = "t" + strconv.FormatInt(id, 10)
		all = append(all, r)
	}
	xrows.Close()
	// 지갑에서 쓴 건 직접 적은 기록이 실제 지출이다(충전 이체는 아래에서 지갑으로 옮긴 돈으로 뺀다).
	wspends, err := s.finWalletSpendRows(ctx, owner, from)
	if err != nil {
		return out, err
	}
	all = append(all, wspends...)
	sort.SliceStable(all, func(i, j int) bool { return all[i].At < all[j].At })
	out.TxCount = len(all)
	for i := range all {
		r := &all[i]
		r.key, r.label, r.orig = finKey(r.Content), r.Content, r.Cat1
		if l, ok := labels[r.key]; ok {
			if l.Label != "" {
				r.label = l.Label
			}
			if l.Cat1 != "" {
				r.Cat1 = l.Cat1
			}
		}
	}
	// '수정됨'은 이름·분류를 고친 것만. 메모는 설명이라 표시하지 않는다.
	edited := func(k string) bool { l := labels[k]; return l.Label != "" || l.Cat1 != "" }
	plain := make([]FinTx, len(all))
	for i := range all {
		plain[i] = all[i].FinTx
	}
	partner := finPairs(plain)
	paired := map[int]bool{}
	for i := range partner {
		paired[i] = true
	}
	// 짝이 맞는 이체라도 받는 쪽이 적금·청약 계좌면 떼어둔 돈이다(결제수단 칸에 상품 이름이 온다).
	toSavingAcct := func(i int) bool {
		j, ok := partner[i]
		return ok && finSaveAcct.MatchString(all[j].Method)
	}
	// 가족 이름이 든 이체('토스 김승환', '카뱅오픈김승환')는 연결 안 된 내 계좌로 옮긴 것이다.
	var family []string
	if us, err := s.ListUsers(ctx); err == nil {
		for _, u := range us {
			if len([]rune(u.Name)) >= 2 {
				family = append(family, u.Name)
			}
		}
	}
	toFamily := func(content string) bool {
		for _, n := range family {
			if strings.Contains(content, n) {
				return true
			}
		}
		return false
	}
	// 저축(save)은 쓴 돈이 아니지만 한 달 가용에서 빼는 돈이다: 적금·청약·펀드·연금·보험.
	// 파일의 분류는 믿을 수 없다(적금 이체가 '미분류'·'카드대금'으로 온다). 내용으로도 본다.
	saving := func(r *finRow) bool {
		return r.Cat1 == "저축" || r.Cat1 == "투자" || finSaveRe.MatchString(r.Content) || finSaveRe.MatchString(r.label)
	}
	// 파킹통장·MMF 로 옮긴 건 내 돈의 자리만 바뀐 것 — 쓴 돈도 저축도 수입도 아니다.
	parking := func(r *finRow) bool {
		return !finSaveRe.MatchString(r.Content) && finParkRe.MatchString(r.Content)
	}
	// 카드 결제가 결제수단으로 들어와 있으면 그 카드의 대금 이체는 같은 돈을 한 번 더 세는 것이다.
	var methods []string
	for _, r := range all {
		if r.Type == "지출" && r.Method != "" {
			methods = append(methods, strings.ToLower(r.Method))
		}
	}
	// 잔액을 맞춘 뒤의 충전만 지갑으로 옮긴 돈이다. 그 전엔 사용 기록이 없으니 충전을 쓴 돈으로 둔다
	// (안 그러면 지난 달들의 지역화폐 지출이 통째로 사라져 평소 생활비가 줄어든다).
	// 사용을 적어둔 달도 마찬가지다 — 그 달은 적은 사용이 쓴 돈이다(충전까지 세면 두 번이다).
	recorded := map[string]bool{}
	for _, r := range wspends {
		recorded[fmt.Sprintf("%d:%s", r.owner, r.At[:7])] = true
	}
	walletCharge := func(r *finRow) bool {
		for _, w := range wallets {
			if w.OwnerID == r.owner && strings.Contains(r.Content, w.Match) &&
				(r.At > w.BaseAt || recorded[fmt.Sprintf("%d:%s", r.owner, r.At[:7])]) {
				return true
			}
		}
		return false
	}
	payCharge := func(r *finRow) bool {
		m := finPayCharge.FindStringSubmatch(r.Content)
		if m == nil {
			return false
		}
		brand := strings.ToLower(m[1])
		for _, meth := range methods {
			if strings.Contains(meth, brand) {
				return true
			}
		}
		return false
	}
	cardBill := func(r *finRow) bool {
		m := finCardBill.FindStringSubmatch(r.Content)
		if m == nil {
			return false
		}
		brand := strings.ToLower(strings.TrimSpace(m[1]))
		names := []string{brand + "카드"}
		if en := finCardEn[brand]; en != "" {
			names = append(names, en)
		}
		for _, meth := range methods {
			for _, n := range names {
				if strings.Contains(meth, n) {
					return true
				}
			}
		}
		return false
	}
	for i := range all {
		r := &all[i]
		switch {
		case r.Type == "수입":
			r.kind = "in"
		case r.Type == "지출" && r.Amount < 0 && (finSaveRe.MatchString(r.Content) || finSaveRe.MatchString(r.label) || r.Cat1 == "보험"):
			// 보험료·청약 납입은 카드로 나가도 쓴 돈이 아니라 떼어두는 돈으로 본다.
			r.kind = "save"
		case r.Type == "지출":
			r.kind = "spend"
		case r.Type == "이체" && paired[i] && r.Amount < 0 && (saving(r) || toSavingAcct(i)):
			if toSavingAcct(i) && labels[r.key].Label == "" {
				r.label = all[partner[i]].Method // 계좌번호 대신 상품 이름
			}
			// 내 저축·투자 계좌로 옮긴 것도 '떼어둔 돈'이다. 매달 자동이체되는 적금을
			// 월 가용에서 빼려면 세야 한다(들어온 쪽 줄은 건너뛴다).
			r.kind = "save"
		case r.Type == "이체" && r.Amount < 0 && walletCharge(r):
			r.kind = "skip"
		case r.Type == "이체" && r.Amount < 0 && (cardBill(r) || payCharge(r)):
			r.kind = "skip"
		case r.Type == "이체" && (paired[i] || toFamily(r.Content) || parking(r)):
			r.kind = "skip"
		case r.Type == "이체" && r.Amount > 0:
			r.kind = "in_other"
		case r.Type == "이체" && saving(r):
			r.kind = "save"
		case r.Type == "이체":
			r.kind = "spend"
		default:
			r.kind = "skip"
		}
	}

	thisMonth := now.Format("2006-01")
	var complete []string // 끝난 달, 오래된 것부터 최대 12개
	for k := 12; k >= 1; k-- {
		complete = append(complete, now.AddDate(0, -k, 0).Format("2006-01"))
	}
	last6 := complete[len(complete)-6:]
	in6 := map[string]bool{}
	for _, m := range last6 {
		in6[m] = true
	}

	// '지출 아님'으로 뺀 것: 쓴 돈·저축에서 빼고 목록으로만 돌려준다(되돌릴 수 있게).
	notSpend := map[string]bool{}
	nrows, err := s.db.QueryContext(ctx, `SELECT key FROM fin_not_spend`)
	if err != nil {
		return out, err
	}
	for nrows.Next() {
		var k string
		if err := nrows.Scan(&k); err != nil {
			nrows.Close()
			return out, err
		}
		notSpend[k] = true
	}
	nrows.Close()
	nsByKey := map[string]*FinNotSpend{}
	for i := range all {
		r := &all[i]
		if (r.kind != "spend" && r.kind != "save") || !notSpend[r.key] {
			continue
		}
		r.kind = "skip"
		x := nsByKey[r.key]
		if x == nil {
			x = &FinNotSpend{Key: r.key, Label: r.label, Cat1: r.Cat1, Edited: edited(r.key)}
			nsByKey[r.key] = x
		}
		x.Total += -r.Amount
		x.Count++
	}
	for k := range notSpend {
		if x := nsByKey[k]; x != nil {
			out.NotSpend = append(out.NotSpend, *x)
		} else {
			out.NotSpend = append(out.NotSpend, FinNotSpend{Key: k, Label: k})
		}
	}
	sort.Slice(out.NotSpend, func(i, j int) bool { return out.NotSpend[i].Total > out.NotSpend[j].Total })

	// 고정 판정을 사람이 뒤집은 것(지출·수입 공통, key 는 finKey).
	overrides := map[string]bool{}
	regular := map[string]bool{}
	orows, err := s.db.QueryContext(ctx, `SELECT key, fixed FROM fin_fixed`)
	if err != nil {
		return out, err
	}
	for orows.Next() {
		var k string
		var f int
		if err := orows.Scan(&k, &f); err != nil {
			orows.Close()
			return out, err
		}
		// fixed: 1 고정 / 0 예상 외로 옮김 / -1 평소(예상 외로 판정하지 않음)
		overrides[k] = f == 1
		if f < 0 {
			regular[k] = true
		}
	}
	orows.Close()

	// 2-1) 수입원: 최근 끝난 12개월, 내용별로 묶는다(같은 이름으로 고친 건 하나로).
	//      수입인 것 중 최근 6개월에 3달 이상 들어온 건 고정수입, 나머지는 예상 외 수입.
	incOv := map[string]bool{}
	irows2, err := s.db.QueryContext(ctx, `SELECT key, income FROM fin_income`)
	if err != nil {
		return out, err
	}
	for irows2.Next() {
		var k string
		var v int
		if err := irows2.Scan(&k, &v); err != nil {
			irows2.Close()
			return out, err
		}
		incOv[k] = v != 0
	}
	irows2.Close()
	in12 := map[string]bool{}
	for _, m := range complete {
		in12[m] = true
	}
	type incAgg struct {
		label, cat1 string
		typed       bool // '수입' 타입
		byMonth     map[string]int64
		total       int64
		keys        []string
	}
	incs := map[string]*incAgg{}
	for _, r := range all {
		if (r.kind != "in" && r.kind != "in_other") || r.Amount <= 0 {
			continue
		}
		gk := r.key
		if l := labels[r.key]; l.Label != "" {
			gk = "=" + l.Label // 같은 이름으로 고친 것끼리 묶는다
		}
		g := incs[gk]
		if g == nil {
			g = &incAgg{label: r.label, cat1: r.Cat1, byMonth: map[string]int64{}}
			incs[gk] = g
		}
		if !slices.Contains(g.keys, r.key) {
			g.keys = append(g.keys, r.key)
		}
		g.typed = g.typed || r.kind == "in"
		if in12[r.At[:7]] {
			g.byMonth[r.At[:7]] += r.Amount
			g.total += r.Amount
		}
	}
	incomeKeys := map[string]bool{}
	keyGroup := map[string]string{}       // finKey → 묶음 key
	incByGroup := map[string]*FinIncome{} // 목록에서 빠진 작은 것도 여기엔 있다
	for gk, g := range incs {
		var vals []int64
		for _, v := range g.byMonth {
			vals = append(vals, v)
		}
		med := median(vals)
		recurring := len(vals) >= 3 && med >= 300_000 && !finOwnMoney.MatchString(g.label)
		auto := g.typed || recurring
		final := auto
		var ov, hasOv bool
		for _, k := range g.keys {
			if v, ok := incOv[k]; ok {
				ov, hasOv = v, true
				break
			}
		}
		if hasOv {
			final = ov
		}
		isEdited := false
		for _, k := range g.keys {
			if final {
				incomeKeys[k] = true
			}
			isEdited = isEdited || edited(k)
		}
		sort.Strings(g.keys)
		f := FinIncome{Key: gk, Keys: g.keys, Edited: isEdited, Label: g.label, Cat1: g.cat1, Monthly: med, Months: len(vals), Total: g.total, Auto: auto, Income: final}
		if hasOv {
			v := ov
			f.Override = &v
		}
		for m := range g.byMonth {
			if in6[m] {
				f.Months6++
			}
		}
		f.FixedAuto = final && f.Months6 >= 3
		f.Fixed = f.FixedAuto
		for _, k := range g.keys {
			keyGroup[k] = gk
			if v, ok := overrides[k]; ok && f.FixedOverride == nil {
				vv := v
				f.FixedOverride, f.Fixed = &vv, v && final
			}
		}
		incByGroup[gk] = &f
		// 목록엔 1년에 10만 원 넘게 들어온 것만(앱테크 몇십 원은 세되 보여주지 않는다).
		if !hasOv && g.total < 100_000 {
			continue
		}
		out.Income = append(out.Income, f)
	}
	sort.Slice(out.Income, func(i, j int) bool {
		if out.Income[i].Income != out.Income[j].Income {
			return out.Income[i].Income
		}
		return out.Income[i].Total > out.Income[j].Total
	})
	for i := range all {
		if all[i].kind == "in" || all[i].kind == "in_other" {
			if incomeKeys[all[i].key] {
				all[i].kind = "in"
			} else {
				all[i].kind = "in_other"
			}
		}
	}

	// 3) 고정지출: 최근 끝난 6개월 중 3달 이상, 달마다 금액이 비슷(중앙값 대비 ±25%).
	//    주거/통신·보험은 금액이 달라도 고정으로 본다.
	type agg struct {
		label, cat1, kind string
		byMonth           map[string]int64
		count             int
	}
	groups := map[string]*agg{}
	for _, r := range all {
		if (r.kind != "spend" && r.kind != "save") || r.Amount >= 0 || !in6[r.At[:7]] {
			continue
		}
		k := r.key
		g := groups[k]
		if g == nil {
			g = &agg{label: r.label, cat1: r.Cat1, kind: r.kind, byMonth: map[string]int64{}}
			groups[k] = g
		}
		g.byMonth[r.At[:7]] += -r.Amount
		g.count++
	}
	fixedKeys := map[string]bool{}
	for k, g := range groups {
		var vals []int64
		for _, v := range g.byMonth {
			vals = append(vals, v)
		}
		med := median(vals)
		stable := len(vals) >= 3
		for _, v := range vals {
			if med > 0 && math.Abs(float64(v-med))/float64(med) > 0.25 {
				stable = false
			}
		}
		// 적금은 금액이 들쭉날쭉해도(주·일 단위 자동이체) 매달 넣으면 고정이다.
		category := len(vals) >= 3 && (g.kind == "save" || g.cat1 == "주거/통신" || strings.Contains(g.label, "보험"))
		auto := stable || category
		ov, hasOv := overrides[k]
		final := auto
		if hasOv {
			final = ov
		}
		if !auto && !hasOv {
			continue
		}
		f := FinFixed{Key: k, Label: g.label, Cat1: g.cat1, Kind: g.kind, Monthly: med, Months: len(vals), Auto: auto, Fixed: final, Edited: edited(k)}
		if hasOv {
			v := ov
			f.Override = &v
		}
		out.Fixed = append(out.Fixed, f)
		if final {
			fixedKeys[k] = true
		}
	}
	sort.Slice(out.Fixed, func(i, j int) bool { return out.Fixed[i].Monthly > out.Fixed[j].Monthly })

	// 4) 달별 흐름.
	byMonth := map[string]*FinMonth{}
	for _, m := range append(append([]string{}, complete...), thisMonth) {
		byMonth[m] = &FinMonth{Month: m, Partial: m == thisMonth}
	}
	for _, r := range all {
		m := byMonth[r.At[:7]]
		if m == nil {
			continue
		}
		switch r.kind {
		case "in":
			m.In += r.Amount
			if g := incByGroup[keyGroup[r.key]]; g != nil && g.Fixed {
				m.FixedIn += r.Amount
			}
		case "in_other":
			m.OtherIn += r.Amount
		case "spend":
			m.Spend += -r.Amount
			if r.wallet {
				m.Wallet += -r.Amount
			}
			if fixedKeys[finKey(r.Content)] {
				m.Fixed += -r.Amount
			}
		case "save":
			m.Save += -r.Amount
			if fixedKeys[finKey(r.Content)] {
				m.FixedSave += -r.Amount
			}
		}
	}
	for _, m := range append(append([]string{}, complete...), thisMonth) {
		x := byMonth[m]
		x.Variable = x.Spend - x.Fixed
		out.Months = append(out.Months, *x)
	}

	// 5) 예상 외 지출: 고정이 아닌 쓴 돈 중, 한 건이 10만 원 이상이면서 그 분류의 평소 한 건(최근 6개월 중앙값)의
	//    3배 이상이거나, 그 분류의 달 합계가 평소(최근 6개월 중앙값)의 1.5배 이상으로 튄 달의 큰 건들.
	if month == "" {
		month = complete[len(complete)-1]
	}
	out.Month = month
	// detect 는 한 달의 예상 외 지출. 평소 값은 그 달을 뺀 최근 6개월로 잡는다.
	detect := func(month string) []FinUnexpected {
		var found []FinUnexpected
		perTx := map[string][]int64{}
		catMonth := map[string]map[string]int64{}
		for _, r := range all {
			if r.kind != "spend" || r.Amount >= 0 || fixedKeys[finKey(r.Content)] {
				continue
			}
			c := r.Cat1
			if in6[r.At[:7]] && r.At[:7] != month {
				perTx[c] = append(perTx[c], -r.Amount)
			}
			if catMonth[c] == nil {
				catMonth[c] = map[string]int64{}
			}
			catMonth[c][r.At[:7]] += -r.Amount
		}
		spike := map[string]string{}
		for c, bm := range catMonth {
			var vals []int64
			for _, m := range last6 {
				if m != month {
					vals = append(vals, bm[m])
				}
			}
			med := median(vals)
			if cur := bm[month]; med > 0 && cur >= med*3/2 && cur-med >= 50000 {
				spike[c] = "평소 " + c + " 한 달 " + won(med) + " → 이번 달 " + won(cur)
			}
		}
		for _, r := range all {
			// 카드값은 한 달치를 모은 금액이라 한 건으로 튀어 보여도 '예상 외'가 아니다. 분류가 아니라 이름으로
			// 가린다 — 파일은 사람에게 보낸 이체도 '카드대금'으로 분류하곤 한다.
			if r.kind != "spend" || r.Amount >= 0 || r.At[:7] != month || fixedKeys[finKey(r.Content)] || finCardBill.MatchString(r.Content) {
				continue
			}
			amt := -r.Amount
			// 사람이 '예상 외'로 옮긴 건 기준과 상관없이 넣는다.
			if regular[r.key] {
				continue
			}
			moved := false
			if v, ok := overrides[r.key]; ok && !v {
				moved = true
			}
			if amt < 100000 && !moved {
				continue
			}
			reason := ""
			if med := median(perTx[r.Cat1]); med > 0 && amt >= med*3 {
				reason = "평소 " + orDefault(r.Cat1, "미분류") + " 한 건(" + won(med) + ")의 " + strconv.FormatInt(amt/med, 10) + "배"
			} else if len(perTx[r.Cat1]) == 0 {
				reason = "최근 6개월에 없던 " + orDefault(r.Cat1, "미분류") + " 지출"
			} else if sp := spike[r.Cat1]; sp != "" {
				reason = sp
			}
			if reason == "" && moved {
				reason = "예상 외로 옮김"
			}
			if reason != "" {
				found = append(found, FinUnexpected{Key: r.key, Ref: r.ref, Edited: edited(r.key), At: r.At, Content: r.label, Cat1: r.Cat1, Amount: amt, Reason: reason})
			}
		}
		return found
	}
	scope := map[string]bool{month: true}
	if allMonths {
		for _, m := range out.Months {
			scope[m.Month] = true
		}
		for _, m := range out.Months {
			out.Unexpected = append(out.Unexpected, detect(m.Month)...)
		}
	} else {
		out.Unexpected = detect(month)
	}
	if out.Unexpected == nil {
		out.Unexpected = []FinUnexpected{}
	}
	// 달마다 예상 외 지출 합. '평소 생활비'는 이걸 뺀 것으로 잡는다(가전 같은 한 번의 큰 지출에 휘둘리지 않게).
	for i := range out.Months {
		for _, u := range detect(out.Months[i].Month) {
			out.Months[i].Unexpected += u.Amount
		}
	}
	sort.Slice(out.Unexpected, func(i, j int) bool { return out.Unexpected[i].Amount > out.Unexpected[j].Amount })

	// 평소 지출: 그 달에 쓴 돈 중 고정도 예상 외도 아닌 것(한 달에 쓸 수 있는 돈 안에서 쓰는 돈).
	unexp := map[string]bool{}
	for _, u := range out.Unexpected {
		unexp[fmt.Sprintf("%s|%s|%d", u.Key, u.At, u.Amount)] = true
	}
	for _, r := range all {
		if r.kind != "spend" || r.Amount >= 0 || !scope[r.At[:7]] || fixedKeys[r.key] {
			continue
		}
		if unexp[fmt.Sprintf("%s|%s|%d", r.key, r.At, -r.Amount)] {
			continue
		}
		out.Regular = append(out.Regular, FinRegular{Key: r.key, Ref: r.ref, Edited: edited(r.key), At: r.At, Content: r.label, Cat1: r.Cat1, Amount: -r.Amount, Wallet: r.wallet})
	}
	sort.Slice(out.Regular, func(i, j int) bool { return out.Regular[i].Amount > out.Regular[j].Amount })

	// 예상 외 수입: 그 달의 수입 중 고정수입이 아닌 것.
	for _, r := range all {
		if r.kind != "in" || !scope[r.At[:7]] {
			continue
		}
		g := incByGroup[keyGroup[r.key]]
		if g == nil || g.Fixed {
			continue
		}
		out.IncomeTx = append(out.IncomeTx, FinIncomeTx{Group: g.Key, Ref: r.ref, Key: r.key, Keys: g.Keys, At: r.At, Content: r.label, Cat1: r.Cat1, Amount: r.Amount, Edited: edited(r.key)})
	}
	sort.Slice(out.IncomeTx, func(i, j int) bool { return out.IncomeTx[i].Amount > out.IncomeTx[j].Amount })

	// 목록에 나온 거래의 태그, 그리고 같은 이름의 다른 거래에 썼던 태그(제안만 — 저절로 붙이지 않는다).
	shownKeys := map[string]bool{}
	addRef := func(ref, key string) {
		if ids := txTags[ref]; len(ids) > 0 {
			out.TxTags[ref] = ids
		}
		shownKeys[key] = true
	}
	for _, u := range out.Unexpected {
		addRef(u.Ref, u.Key)
	}
	for _, u := range out.Regular {
		addRef(u.Ref, u.Key)
	}
	for _, u := range out.IncomeTx {
		addRef(u.Ref, u.Key)
	}
	for _, r := range all {
		if shownKeys[r.key] && len(txTags[r.ref]) > 0 {
			out.KeyTxTags[r.key] = uniqIDs(append(out.KeyTxTags[r.key], txTags[r.ref]...))
		}
	}

	// 태그별: 그 달에 쓴 돈(고정 포함)과 저축.
	byTag := map[int64]*FinTagSpend{}
	for _, r := range all {
		if (r.kind != "spend" && r.kind != "save") || r.Amount >= 0 || r.At[:7] != month {
			continue
		}
		ids := txTags[r.ref]
		if fixedKeys[r.key] {
			ids = uniqIDs(append(append([]int64{}, ids...), links[r.key]...))
		}
		if len(ids) == 0 {
			if r.kind == "spend" {
				out.Untagged += -r.Amount
			}
			continue
		}
		for _, id := range ids {
			t := byTag[id]
			if t == nil {
				t = &FinTagSpend{TagID: id}
				byTag[id] = t
			}
			t.Amount += -r.Amount
			t.Count++
		}
	}
	for _, t := range byTag {
		out.TagSpend = append(out.TagSpend, *t)
	}
	sort.Slice(out.TagSpend, func(i, j int) bool { return out.TagSpend[i].Amount > out.TagSpend[j].Amount })

	// 6) 목표와 월 가용.
	goals, err := s.FinanceGoals(ctx)
	if err != nil {
		return out, err
	}
	for i := range goals {
		want := map[string]bool{}
		for _, n := range goals[i].Items {
			want[n] = true
		}
		for _, it := range out.Items {
			if want[it.Name] && it.Group != "debt" {
				goals[i].Current += it.Amount
			}
		}
		out.Plan.GoalsMonthly += goals[i].Monthly
	}
	out.Goals = goals

	income, buffer := s.finSettings(ctx)
	var spend6, n6 int64
	var ins []int64
	for _, m := range out.Months {
		if m.Partial {
			continue
		}
		ins = append(ins, m.In)
		if in6[m.Month] {
			spend6 += m.Spend - m.Unexpected
			n6++
		}
	}
	// 고정수입은 항목별 금액을 더하지 않는다 — 같은 급여가 달마다 이름이 달라 여러 항목이 되기도 한다.
	// 달마다 들어온 고정수입을 더하고 최근 6개월의 중앙값을 쓴다.
	fixedByMonth := map[string]int64{}
	for _, r := range all {
		if r.kind != "in" || !in6[r.At[:7]] {
			continue
		}
		if g := incByGroup[keyGroup[r.key]]; g != nil && g.Fixed {
			fixedByMonth[r.At[:7]] += r.Amount
		}
	}
	var fixedVals []int64
	for _, m := range last6 {
		fixedVals = append(fixedVals, fixedByMonth[m])
	}
	out.Plan.FixedIncome = median(fixedVals)
	out.Plan.IncomeHint = out.Plan.FixedIncome
	if out.Plan.IncomeHint == 0 {
		out.Plan.IncomeHint = median(ins)
	}
	if n6 > 0 {
		out.Plan.SpendAvg = spend6 / n6
	}
	for _, f := range out.Fixed {
		if !f.Fixed {
			continue
		}
		if f.Kind == "save" {
			out.Plan.FixedSave += f.Monthly
		} else {
			out.Plan.FixedSpend += f.Monthly
		}
	}
	var v3 int64
	cnt := 0
	for i := len(out.Months) - 2; i >= 0 && cnt < 3; i-- {
		v3 += out.Months[i].Variable - out.Months[i].Unexpected
		cnt++
	}
	if cnt > 0 {
		out.Plan.VariableAvg = v3 / int64(cnt)
	}
	if n := len(out.Months); n > 0 {
		cur := out.Months[n-1] // 이번 달(아직 안 끝남)
		// 예상 외(가전 등)는 뺀다 — 평소 생활비와 같은 기준이어야 한도와 비교할 수 있다.
		out.Plan.MonthSpent, out.Plan.MonthWallet, out.Plan.MonthUnexpected = cur.Variable-cur.Unexpected, cur.Wallet, cur.Unexpected
	}
	out.Plan.DaysPassed = now.Day()
	out.Plan.DaysInMonth = time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Day()
	out.Plan.IncomeBase = income
	base := income
	if base == 0 {
		base = out.Plan.IncomeHint
	}
	out.Plan.Spendable = base - out.Plan.FixedSpend - out.Plan.FixedSave - out.Plan.GoalsMonthly
	out.Plan.BufferMonths = buffer
	// 지역화폐는 쓸 곳이 정해진 돈이라 여유자금엔 넣지 않는다(자산·순자산엔 들어간다).
	out.Plan.Liquid = out.Groups["liquid"]
	for _, w := range out.Wallets {
		out.Plan.Liquid -= w.Balance
	}
	for _, it := range out.Items {
		if it.Group != "liquid" && it.Group != "debt" && finParkRe.MatchString(it.Name+" "+it.Category) {
			out.Plan.Emergency += it.Amount
		}
	}
	out.Plan.Free = out.Plan.Liquid + out.Plan.Emergency - out.Plan.SpendAvg*int64(buffer)
	return out, nil
}

func won(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	var s string
	switch {
	case n >= 100000000:
		s = strconv.FormatFloat(float64(n)/1e8, 'f', 1, 64) + "억"
	case n >= 10000:
		s = strconv.FormatInt(int64(math.Round(float64(n)/1e4)), 10) + "만 원"
	default:
		s = strconv.FormatInt(n, 10) + "원"
	}
	if neg {
		return "-" + s
	}
	return s
}

// --- 목표 ---

func (s *Store) FinanceGoals(ctx context.Context) ([]FinGoal, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, target, due, monthly, items FROM fin_goals ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FinGoal{}
	for rows.Next() {
		var g FinGoal
		var items string
		if err := rows.Scan(&g.ID, &g.Name, &g.Target, &g.Due, &g.Monthly, &items); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(items), &g.Items)
		if g.Items == nil {
			g.Items = []string{}
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) FinanceSaveGoal(ctx context.Context, g FinGoal, by int64) (FinGoal, error) {
	g.Name = strings.TrimSpace(g.Name)
	if g.Name == "" || len([]rune(g.Name)) > 60 {
		return FinGoal{}, invalid("목표 이름을 적어주세요")
	}
	if g.Target <= 0 {
		return FinGoal{}, invalid("목표 금액을 적어주세요")
	}
	if g.Monthly < 0 {
		return FinGoal{}, invalid("월 적립은 0 이상이어야 해요")
	}
	if g.Due != nil && *g.Due != "" && !validDate(*g.Due) {
		return FinGoal{}, invalid("날짜가 올바르지 않아요")
	}
	if g.Due != nil && *g.Due == "" {
		g.Due = nil
	}
	if g.Items == nil {
		g.Items = []string{}
	}
	items, _ := json.Marshal(g.Items)
	if g.ID == 0 {
		r, err := s.db.ExecContext(ctx, `INSERT INTO fin_goals (name, target, due, monthly, items, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			g.Name, g.Target, g.Due, g.Monthly, string(items), by, time.Now().Unix())
		if err != nil {
			return FinGoal{}, err
		}
		g.ID, _ = r.LastInsertId()
		return g, nil
	}
	r, err := s.db.ExecContext(ctx, `UPDATE fin_goals SET name=?, target=?, due=?, monthly=?, items=? WHERE id=?`, g.Name, g.Target, g.Due, g.Monthly, string(items), g.ID)
	if err != nil {
		return FinGoal{}, err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return FinGoal{}, ErrNotFound
	}
	return g, nil
}

func (s *Store) FinanceDeleteGoal(ctx context.Context, id int64) error {
	r, err := s.db.ExecContext(ctx, `DELETE FROM fin_goals WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
