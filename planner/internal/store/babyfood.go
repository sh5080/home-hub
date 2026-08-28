package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 이유식 식단·재고.
//
// 식단표는 D+n(생후 일수)으로 저장한다. 날짜는 profile.birth_date 로 그때그때
// 환산한다 — 생일을 고치면 194일치가 통째로 따라 움직여야 하기 때문이다.
//
// 재고는 '마지막 실사 − 그 뒤 소모 + 그 뒤 제조'로 계산한다. 숫자 하나를
// 직접 고치는 방식은 안 고치면 바로 틀어지고, 왜 틀어졌는지도 알 수 없다.

const (
	bfDefaultHorizon = 21 // 3주
	bfMaxHorizon     = 180
	bfMaxToppings    = 12
	bfMaxNameLen     = 40
)

// BFProfile은 아이 정보와 계산 기준이다. 생일은 개인정보라 코드에 없다.
type BFProfile struct {
	Name        string  `json:"name"`
	BirthDate   *string `json:"birth_date"`
	HorizonDays int     `json:"horizon_days"`
	// 파생값 — 생일이 있을 때만 채운다.
	TodayDDay *int   `json:"today_dday"`
	Today     string `json:"today"`
	// 식단이 들어 있는 범위.
	FromDDay *int `json:"from_dday"`
	ToDDay   *int `json:"to_dday"`
}

// BFProfileInput은 부분 수정이다. nil이면 안 건드린다.
type BFProfileInput struct {
	Name        *string
	BirthDate   *string // "" 로 지움
	HorizonDays *int
}

// BFMealSrc는 시드 당시의 끼니 구성이다. 수정 표시("원래 …")에만 쓴다.
type BFMealSrc struct {
	Base     string   `json:"base"`
	Toppings []string `json:"toppings"`
	Snack    *string  `json:"snack"`
}

// BFMeal은 한 끼다.
type BFMeal struct {
	ID       int64     `json:"id"`
	Slot     string    `json:"slot"`
	Base     string    `json:"base"`
	Toppings []string  `json:"toppings"`
	Snack    *string   `json:"snack"`
	Src      BFMealSrc `json:"src"`
	// Edited는 지금 값이 시드 원본과 다른지. 화면에서 "수정됨" 배지를 띄운다.
	Edited   bool   `json:"edited"`
	EatenG   *int   `json:"eaten_g"`
	ServedG  *int   `json:"served_g"`
	Skipped  bool   `json:"skipped"`
	EditedBy *int64 `json:"edited_by"`
	EditedAt *int64 `json:"edited_at"`
}

// BFDay는 하루치 식단이다.
type BFDay struct {
	DDay       int      `json:"dday"`
	Date       string   `json:"date"` // 생일이 없으면 ""
	Stage      string   `json:"stage"`
	Label      string   `json:"label"`
	Kind       string   `json:"kind"`
	NewItem    *string  `json:"new_item"`
	NewItemSrc *string  `json:"new_item_src"`
	Note       string   `json:"note"`
	Meals      []BFMeal `json:"meals"`
}

// BFStock은 재료 한 줄이다. 재고가 어떻게 그 숫자가 됐는지 구성요소를 전부
// 같이 준다 — 조용히 틀어지지 않게 하는 게 이 설계의 요점이다.
type BFStock struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Need int    `json:"need"`  // 기간 내 필요 개수
	Stock *int  `json:"stock"` // nil이면 아직 실사 안 함
	Make  int   `json:"make"`  // 제조 필요 = max(0, need - stock)

	CountQty  *int   `json:"count_qty"`
	CountDDay *int   `json:"count_dday"`
	CountAt   *int64 `json:"count_at"`
	countBatchID int64
	Used      int    `json:"used"` // 실사 다음날 ~ 어제 소모
	Made      int    `json:"made"` // 실사 이후 제조
}

// BFStockView는 재고 화면 전체다.
type BFStockView struct {
	From        string    `json:"from"`
	To          string    `json:"to"`
	FromDDay    int       `json:"from_dday"`
	ToDDay      int       `json:"to_dday"`
	HorizonDays int       `json:"horizon_days"`
	Items       []BFStock `json:"items"`
}

// --- 날짜 ↔ D+n ---

func parseDay(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, invalid("날짜는 YYYY-MM-DD 형식이어야 해요")
	}
	return t, nil
}

// bfClock은 테스트가 '오늘'을 고정할 수 있게 둔 갈고리다.
var bfClock = func() time.Time { return time.Now() }

func bfToday() string { return bfClock().Format("2006-01-02") }

// --- profile ---

func (s *Store) bfProfileRow(ctx context.Context) (string, *string, int, error) {
	var name string
	var birth *string
	var horizon int
	err := s.db.QueryRowContext(ctx,
		`SELECT name, birth_date, horizon_days FROM bf_profile WHERE id=1`).
		Scan(&name, &birth, &horizon)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, bfDefaultHorizon, nil
	}
	return name, birth, horizon, err
}

// BFProfileGet은 프로필과 함께 오늘의 D+n, 식단이 들어 있는 범위를 준다.
func (s *Store) BFProfileGet(ctx context.Context) (BFProfile, error) {
	name, birth, horizon, err := s.bfProfileRow(ctx)
	if err != nil {
		return BFProfile{}, err
	}
	p := BFProfile{Name: name, BirthDate: birth, HorizonDays: horizon, Today: bfToday()}

	var lo, hi sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT min(dday), max(dday) FROM bf_days`).Scan(&lo, &hi); err != nil {
		return BFProfile{}, err
	}
	if lo.Valid {
		a, b := int(lo.Int64), int(hi.Int64)
		p.FromDDay, p.ToDDay = &a, &b
	}
	if birth != nil {
		d, err := bfDDay(*birth, p.Today)
		if err != nil {
			return BFProfile{}, err
		}
		p.TodayDDay = &d
	}
	return p, nil
}

// bfDDay는 생일과 날짜로 생후 일수를 구한다.
func bfDDay(birth, date string) (int, error) {
	b, err := parseDay(birth)
	if err != nil {
		return 0, err
	}
	d, err := parseDay(date)
	if err != nil {
		return 0, err
	}
	return int(d.Sub(b).Hours() / 24), nil
}

// bfDate는 생후 일수를 날짜로 되돌린다.
func bfDate(birth string, dday int) (string, error) {
	b, err := parseDay(birth)
	if err != nil {
		return "", err
	}
	return b.AddDate(0, 0, dday).Format("2006-01-02"), nil
}

// BFProfileSet은 프로필을 부분 수정한다.
func (s *Store) BFProfileSet(ctx context.Context, in BFProfileInput) (BFProfile, error) {
	name, birth, horizon, err := s.bfProfileRow(ctx)
	if err != nil {
		return BFProfile{}, err
	}
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if len([]rune(name)) > bfMaxNameLen {
			return BFProfile{}, invalid("이름이 너무 길어요")
		}
	}
	if in.BirthDate != nil {
		v := strings.TrimSpace(*in.BirthDate)
		if v == "" {
			birth = nil
		} else {
			if _, err := parseDay(v); err != nil {
				return BFProfile{}, err
			}
			birth = &v
		}
	}
	if in.HorizonDays != nil {
		horizon = *in.HorizonDays
		if horizon < 1 || horizon > bfMaxHorizon {
			return BFProfile{}, invalid(fmt.Sprintf("기간은 1~%d일 사이여야 해요", bfMaxHorizon))
		}
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO bf_profile (id, name, birth_date, horizon_days, updated_at)
		 VALUES (1, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   name=excluded.name, birth_date=excluded.birth_date,
		   horizon_days=excluded.horizon_days, updated_at=excluded.updated_at`,
		name, birth, horizon, time.Now().Unix()); err != nil {
		return BFProfile{}, err
	}
	return s.BFProfileGet(ctx)
}

// --- 조회 ---

// BFRange는 [from, to] 구간(D+n 기준)의 식단을 돌려준다.
func (s *Store) BFRange(ctx context.Context, from, to int) ([]BFDay, error) {
	if to < from {
		return nil, invalid("범위가 거꾸로예요")
	}
	if to-from > 400 {
		return nil, invalid("한 번에 400일까지만 볼 수 있어요")
	}
	_, birth, _, err := s.bfProfileRow(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT dday, stage, label, kind, new_item, new_item_src, note
		   FROM bf_days WHERE dday BETWEEN ? AND ? ORDER BY dday`, from, to)
	if err != nil {
		return nil, err
	}
	days := map[int]*BFDay{}
	var order []int
	for rows.Next() {
		var d BFDay
		if err := rows.Scan(&d.DDay, &d.Stage, &d.Label, &d.Kind,
			&d.NewItem, &d.NewItemSrc, &d.Note); err != nil {
			rows.Close()
			return nil, err
		}
		if birth != nil {
			if d.Date, err = bfDate(*birth, d.DDay); err != nil {
				rows.Close()
				return nil, err
			}
		}
		d.Meals = []BFMeal{}
		days[d.DDay] = &d
		order = append(order, d.DDay)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(order) == 0 {
		return []BFDay{}, nil
	}

	// 끼니 + 구성품을 각각 한 번씩 읽어 Go에서 붙인다. 끼니마다 질의하면
	// 커넥션이 하나뿐이라 그대로 왕복 비용이 된다.
	meals := map[int64]*BFMeal{}
	mrows, err := s.db.QueryContext(ctx,
		`SELECT id, dday, slot, src, eaten_g, served_g, skipped, edited_by, edited_at
		   FROM bf_meals WHERE dday BETWEEN ? AND ? ORDER BY dday, pos`, from, to)
	if err != nil {
		return nil, err
	}
	type slot struct {
		dday int
		meal *BFMeal
	}
	var flat []slot
	for mrows.Next() {
		var m BFMeal
		var dday int
		var src string
		var skipped int
		if err := mrows.Scan(&m.ID, &dday, &m.Slot, &src, &m.EatenG, &m.ServedG,
			&skipped, &m.EditedBy, &m.EditedAt); err != nil {
			mrows.Close()
			return nil, err
		}
		m.Skipped = skipped != 0
		if err := json.Unmarshal([]byte(src), &m.Src); err != nil {
			mrows.Close()
			return nil, fmt.Errorf("meal %d src: %w", m.ID, err)
		}
		if m.Src.Toppings == nil {
			m.Src.Toppings = []string{}
		}
		m.Toppings = []string{}
		flat = append(flat, slot{dday, &m})
	}
	mrows.Close()
	if err := mrows.Err(); err != nil {
		return nil, err
	}
	for i := range flat {
		meals[flat[i].meal.ID] = flat[i].meal
	}

	irows, err := s.db.QueryContext(ctx,
		`SELECT i.meal_id, i.role, i.name
		   FROM bf_meal_items i JOIN bf_meals m ON m.id = i.meal_id
		  WHERE m.dday BETWEEN ? AND ? ORDER BY i.meal_id, i.role, i.pos`, from, to)
	if err != nil {
		return nil, err
	}
	for irows.Next() {
		var id int64
		var role, name string
		if err := irows.Scan(&id, &role, &name); err != nil {
			irows.Close()
			return nil, err
		}
		m := meals[id]
		if m == nil {
			continue
		}
		switch role {
		case "base":
			m.Base = name
		case "topping":
			m.Toppings = append(m.Toppings, name)
		case "snack":
			v := name
			m.Snack = &v
		}
	}
	irows.Close()
	if err := irows.Err(); err != nil {
		return nil, err
	}

	for _, f := range flat {
		f.meal.Edited = mealDiffers(*f.meal)
		if d := days[f.dday]; d != nil {
			d.Meals = append(d.Meals, *f.meal)
		}
	}
	out := make([]BFDay, 0, len(order))
	for _, k := range order {
		out = append(out, *days[k])
	}
	return out, nil
}

func mealDiffers(m BFMeal) bool {
	if m.Base != m.Src.Base || len(m.Toppings) != len(m.Src.Toppings) {
		return true
	}
	for i := range m.Toppings {
		if m.Toppings[i] != m.Src.Toppings[i] {
			return true
		}
	}
	a, b := "", ""
	if m.Snack != nil {
		a = *m.Snack
	}
	if m.Src.Snack != nil {
		b = *m.Src.Snack
	}
	return a != b
}

// --- 수정 ---

// BFMealInput은 끼니 부분 수정이다. nil이면 안 건드린다.
type BFMealInput struct {
	Base     *string
	Toppings *[]string
	Snack    *string // "" 로 지움
	EatenG   *int    // 음수면 지움
	ServedG  *int
	Skipped  *bool
	// Reset이 true면 나머지를 무시하고 시드 원본으로 되돌린다.
	Reset bool
}

func bfCleanName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if len([]rune(s)) > bfMaxNameLen {
		return "", invalid("재료 이름이 너무 길어요")
	}
	return s, nil
}

// BFUpdateMeal은 한 끼를 고친다. 구성품은 통째로 다시 쓴다.
func (s *Store) BFUpdateMeal(ctx context.Context, id int64, in BFMealInput, userID int64) (BFDay, error) {
	var dday int
	var srcRaw string
	var base sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT m.dday, m.src,
		        (SELECT name FROM bf_meal_items WHERE meal_id=m.id AND role='base' LIMIT 1)
		   FROM bf_meals m WHERE m.id=?`, id).Scan(&dday, &srcRaw, &base)
	if errors.Is(err, sql.ErrNoRows) {
		return BFDay{}, ErrNotFound
	}
	if err != nil {
		return BFDay{}, err
	}
	var src BFMealSrc
	if err := json.Unmarshal([]byte(srcRaw), &src); err != nil {
		return BFDay{}, err
	}

	// 현재 구성품을 읽어 변경분만 덮어쓴다. tx를 잡기 **전에** 읽는다 —
	// 커넥션이 하나라 tx 안에서 Query 하면 그대로 멈춘다.
	cur := BFMealSrc{Base: base.String, Toppings: []string{}}
	trows, err := s.db.QueryContext(ctx,
		`SELECT role, name FROM bf_meal_items WHERE meal_id=? ORDER BY role, pos`, id)
	if err != nil {
		return BFDay{}, err
	}
	for trows.Next() {
		var role, name string
		if err := trows.Scan(&role, &name); err != nil {
			trows.Close()
			return BFDay{}, err
		}
		if role == "topping" {
			cur.Toppings = append(cur.Toppings, name)
		} else if role == "snack" {
			v := name
			cur.Snack = &v
		}
	}
	trows.Close()
	if err := trows.Err(); err != nil {
		return BFDay{}, err
	}

	next := cur
	if in.Reset {
		next = src
		if next.Toppings == nil {
			next.Toppings = []string{}
		}
	} else {
		if in.Base != nil {
			v, err := bfCleanName(*in.Base)
			if err != nil {
				return BFDay{}, err
			}
			next.Base = v
		}
		if in.Toppings != nil {
			if len(*in.Toppings) > bfMaxToppings {
				return BFDay{}, invalid(fmt.Sprintf("토핑은 %d개까지예요", bfMaxToppings))
			}
			out := make([]string, 0, len(*in.Toppings))
			for _, t := range *in.Toppings {
				v, err := bfCleanName(t)
				if err != nil {
					return BFDay{}, err
				}
				if v != "" {
					out = append(out, v)
				}
			}
			next.Toppings = out
		}
		if in.Snack != nil {
			v, err := bfCleanName(*in.Snack)
			if err != nil {
				return BFDay{}, err
			}
			if v == "" {
				next.Snack = nil
			} else {
				next.Snack = &v
			}
		}
	}

	now := time.Now().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BFDay{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM bf_meal_items WHERE meal_id=?`, id); err != nil {
		return BFDay{}, err
	}
	if next.Base != "" {
		if err := bfPutItem(ctx, tx, id, "base", 0, next.Base); err != nil {
			return BFDay{}, err
		}
	}
	for i, t := range next.Toppings {
		if err := bfPutItem(ctx, tx, id, "topping", i, t); err != nil {
			return BFDay{}, err
		}
	}
	if next.Snack != nil {
		if err := bfPutItem(ctx, tx, id, "snack", 0, *next.Snack); err != nil {
			return BFDay{}, err
		}
	}

	if in.Reset {
		if _, err := tx.ExecContext(ctx,
			`UPDATE bf_meals SET edited_by=NULL, edited_at=NULL WHERE id=?`, id); err != nil {
			return BFDay{}, err
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			`UPDATE bf_meals SET edited_by=?, edited_at=? WHERE id=?`, userID, now, id); err != nil {
			return BFDay{}, err
		}
	}
	if in.EatenG != nil {
		if err := bfSetGrams(ctx, tx, id, "eaten_g", *in.EatenG); err != nil {
			return BFDay{}, err
		}
	}
	if in.ServedG != nil {
		if err := bfSetGrams(ctx, tx, id, "served_g", *in.ServedG); err != nil {
			return BFDay{}, err
		}
	}
	if in.Skipped != nil {
		v := 0
		if *in.Skipped {
			v = 1
		}
		if _, err := tx.ExecContext(ctx, `UPDATE bf_meals SET skipped=? WHERE id=?`, v, id); err != nil {
			return BFDay{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return BFDay{}, err
	}

	days, err := s.BFRange(ctx, dday, dday)
	if err != nil || len(days) == 0 {
		return BFDay{}, err
	}
	return days[0], nil
}

func bfSetGrams(ctx context.Context, tx *sql.Tx, id int64, col string, v int) error {
	if v > 5000 {
		return invalid("양이 너무 커요")
	}
	// col은 호출부에서 고정 문자열로만 준다 — 사용자 입력이 아니다.
	q := "UPDATE bf_meals SET " + col + "=? WHERE id=?"
	if v < 0 {
		_, err := tx.ExecContext(ctx, "UPDATE bf_meals SET "+col+"=NULL WHERE id=?", id)
		return err
	}
	_, err := tx.ExecContext(ctx, q, v, id)
	return err
}

// bfPutItem은 구성품 한 줄을 쓰고, 처음 보는 재료면 재료 목록에도 넣는다.
func bfPutItem(ctx context.Context, tx *sql.Tx, mealID int64, role string, pos int, name string) error {
	kind := "cube"
	if role == "base" {
		kind = "base"
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO bf_ingredients (name, kind) VALUES (?, ?) ON CONFLICT(name) DO NOTHING`,
		name, kind); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO bf_meal_items (meal_id, role, pos, name) VALUES (?, ?, ?, ?)`,
		mealID, role, pos, name)
	return err
}

// BFDayInput은 하루 단위 정보(메모, NEW 재료) 수정이다. 알레르기·좋아함은
// 날짜가 아니라 재료에 달린다 — BFTagFood 를 쓴다.
type BFDayInput struct {
	Note    *string
	NewItem *string // "" 로 지움
}

// BFUpdateDay는 그날의 메모와 NEW 재료를 고친다.
func (s *Store) BFUpdateDay(ctx context.Context, dday int, in BFDayInput) (BFDay, error) {
	sets := []string{}
	args := []any{}
	if in.Note != nil {
		v := strings.TrimSpace(*in.Note)
		if len([]rune(v)) > 500 {
			return BFDay{}, invalid("메모가 너무 길어요")
		}
		sets, args = append(sets, "note=?"), append(args, v)
	}
	if in.NewItem != nil {
		v, err := bfCleanName(*in.NewItem)
		if err != nil {
			return BFDay{}, err
		}
		if v == "" {
			sets = append(sets, "new_item=NULL")
		} else {
			sets, args = append(sets, "new_item=?"), append(args, v)
		}
	}
	if len(sets) == 0 {
		return BFDay{}, invalid("바꿀 내용이 없어요")
	}
	args = append(args, dday)
	// sets의 원소는 전부 위에서 만든 상수 문자열이고 값은 ? 로만 들어간다.
	res, err := s.db.ExecContext(ctx,
		"UPDATE bf_days SET "+strings.Join(sets, ", ")+" WHERE dday=?", args...)
	if err != nil {
		return BFDay{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return BFDay{}, ErrNotFound
	}
	days, err := s.BFRange(ctx, dday, dday)
	if err != nil || len(days) == 0 {
		return BFDay{}, err
	}
	return days[0], nil
}

// --- 재고 ---

// BFStockList는 오늘부터 horizon일치 필요량과 현재 재고를 계산한다.
//
// 구간이 겹치지 않게 나눈다:
//   - 소모(used): 실사 다음날 ~ **어제**
//   - 필요(need): **오늘** ~ 오늘+horizon-1
//
// 오늘 먹일 몫은 '이미 쓴 것'이 아니라 '앞으로 필요한 것'으로 센다. 겹치면
// 오늘치가 두 번 빠진다.
func (s *Store) BFStockList(ctx context.Context, horizon int) (BFStockView, error) {
	p, err := s.BFProfileGet(ctx)
	if err != nil {
		return BFStockView{}, err
	}
	if p.BirthDate == nil {
		return BFStockView{}, invalid("먼저 아이 생일을 설정해주세요")
	}
	if p.TodayDDay == nil {
		return BFStockView{}, invalid("생일이 올바르지 않아요")
	}
	if horizon <= 0 {
		horizon = p.HorizonDays
	}
	if horizon < 1 || horizon > bfMaxHorizon {
		return BFStockView{}, invalid(fmt.Sprintf("기간은 1~%d일 사이여야 해요", bfMaxHorizon))
	}
	from := *p.TodayDDay
	to := from + horizon - 1

	need, err := s.bfCountItems(ctx, from, to)
	if err != nil {
		return BFStockView{}, err
	}
	// 소모는 재료마다 실사 시점이 달라서 (재료, 날짜)별로 받아 Go에서 자른다.
	past, err := s.bfCountItemsByDay(ctx, from)
	if err != nil {
		return BFStockView{}, err
	}
	batches, err := s.bfBatchTotals(ctx)
	if err != nil {
		return BFStockView{}, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT name, kind, count_qty, count_dday, count_at, count_batch_id
		   FROM bf_ingredients ORDER BY name`)
	if err != nil {
		return BFStockView{}, err
	}
	defer rows.Close()

	out := []BFStock{}
	for rows.Next() {
		var it BFStock
		if err := rows.Scan(&it.Name, &it.Kind, &it.CountQty, &it.CountDDay,
			&it.CountAt, &it.countBatchID); err != nil {
			return BFStockView{}, err
		}
		it.Need = need[it.Name]
		if it.CountQty != nil && it.CountDDay != nil {
			for _, u := range past[it.Name] {
				if u.dday > *it.CountDDay {
					it.Used += u.n
				}
			}
			for _, b := range batches[it.Name] {
				if b.id > it.countBatchID {
					it.Made += b.qty
				}
			}
			v := *it.CountQty - it.Used + it.Made
			it.Stock = &v
			if d := it.Need - v; d > 0 {
				it.Make = d
			}
		} else {
			// 실사한 적 없으면 재고를 '모름'으로 둔다. 0으로 단정하면
			// 냉동실에 있는 걸 또 만들게 된다.
			it.Make = it.Need
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return BFStockView{}, err
	}

	fromDate, _ := bfDate(*p.BirthDate, from)
	toDate, _ := bfDate(*p.BirthDate, to)
	return BFStockView{
		From: fromDate, To: toDate, FromDDay: from, ToDDay: to,
		HorizonDays: horizon, Items: out,
	}, nil
}

// bfCountItems는 [from,to]에서 재료별 사용 횟수를 센다. 'menu' 구간은 큐브가
// 아니므로, 건너뛴 끼니는 먹지 않았으므로 제외한다.
func (s *Store) bfCountItems(ctx context.Context, from, to int) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT i.name, count(*) FROM bf_meal_items i
		   JOIN bf_meals m ON m.id = i.meal_id
		   JOIN bf_days  d ON d.dday = m.dday
		  WHERE d.kind='topping' AND m.skipped=0 AND m.dday BETWEEN ? AND ?
		  GROUP BY i.name`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var name string
		var n int
		if err := rows.Scan(&name, &n); err != nil {
			return nil, err
		}
		out[name] = n
	}
	return out, rows.Err()
}

type bfDayCount struct {
	dday int
	n    int
}

// bfCountItemsByDay는 today 이전의 소모를 (재료, 날짜)별로 준다. 재료마다
// 실사 시점이 달라서 한 덩어리로 합칠 수 없다.
func (s *Store) bfCountItemsByDay(ctx context.Context, today int) (map[string][]bfDayCount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT i.name, m.dday, count(*) FROM bf_meal_items i
		   JOIN bf_meals m ON m.id = i.meal_id
		   JOIN bf_days  d ON d.dday = m.dday
		  WHERE d.kind='topping' AND m.skipped=0 AND m.dday < ?
		  GROUP BY i.name, m.dday`, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]bfDayCount{}
	for rows.Next() {
		var name string
		var c bfDayCount
		if err := rows.Scan(&name, &c.dday, &c.n); err != nil {
			return nil, err
		}
		out[name] = append(out[name], c)
	}
	return out, rows.Err()
}

type bfBatch struct {
	id  int64
	qty int
}

func (s *Store) bfBatchTotals(ctx context.Context) (map[string][]bfBatch, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, id, qty FROM bf_batches ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]bfBatch{}
	for rows.Next() {
		var name string
		var b bfBatch
		if err := rows.Scan(&name, &b.id, &b.qty); err != nil {
			return nil, err
		}
		out[name] = append(out[name], b)
	}
	return out, rows.Err()
}

// BFCount는 실사다. 지금 냉동실에 있는 개수를 그대로 적는다. 이 시점 이후의
// 소모·제조만 다시 반영되므로, 어긋났을 때 되돌릴 수 있는 유일한 손잡이다.
func (s *Store) BFCount(ctx context.Context, name string, qty int, userID int64) error {
	name, err := bfCleanName(name)
	if err != nil {
		return err
	}
	if name == "" {
		return invalid("재료를 골라주세요")
	}
	if qty < 0 || qty > 9999 {
		return invalid("수량은 0 이상이어야 해요")
	}
	dday, err := s.bfTodayDDay(ctx)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE bf_ingredients
		    SET count_qty=?, count_dday=?, count_at=?, count_by=?,
		        count_batch_id=(SELECT COALESCE(max(id), 0) FROM bf_batches)
		  WHERE name=?`,
		qty, dday, time.Now().Unix(), userID, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// BFAddBatch는 큐브를 만든 기록이다(+). 실사 이후의 것만 재고에 더해진다.
func (s *Store) BFAddBatch(ctx context.Context, name string, qty int, note string, userID int64) error {
	name, err := bfCleanName(name)
	if err != nil {
		return err
	}
	if name == "" {
		return invalid("재료를 골라주세요")
	}
	if qty <= 0 || qty > 9999 {
		return invalid("만든 개수를 적어주세요")
	}
	note = strings.TrimSpace(note)
	if len([]rune(note)) > 200 {
		return invalid("메모가 너무 길어요")
	}
	dday, err := s.bfTodayDDay(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO bf_ingredients (name, kind) VALUES (?, 'cube') ON CONFLICT(name) DO NOTHING`,
		name); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO bf_batches (name, qty, made_dday, made_at, made_by, note)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		name, qty, dday, time.Now().Unix(), userID, note); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) bfTodayDDay(ctx context.Context) (int, error) {
	_, birth, _, err := s.bfProfileRow(ctx)
	if err != nil {
		return 0, err
	}
	if birth == nil {
		return 0, invalid("먼저 아이 생일을 설정해주세요")
	}
	return bfDDay(*birth, bfToday())
}

// BFDDayOf는 생일과 날짜로 생후 일수를 구한다. 핸들러가 ?from=YYYY-MM-DD 를
// 저장 형식으로 바꿀 때 쓴다.
func BFDDayOf(birth, date string) (int, error) { return bfDDay(birth, date) }

// --- 음식별 표시 (알레르기 반응 / 좋아함) ---

// BFFood는 재료 한 종류와 거기 달린 표시다.
type BFFood struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // 'base' | 'cube' | 'dish'
	// 둘은 서로 독립이다. 반응이 있으면서 잘 먹을 수도 있다.
	Reaction bool   `json:"reaction"`
	Liked    bool   `json:"liked"`
	TagAt    *int64 `json:"tag_at"`
	TagBy    *int64 `json:"tag_by"`
	// 처음 나오는 날. "언제부터 먹었나"를 짚을 수 있게 같이 준다.
	FirstDDay *int   `json:"first_dday"`
	FirstDate string `json:"first_date"`
	Uses      int    `json:"uses"`
}

// BFFoods는 식단에 나오는 모든 재료를 표시와 함께 돌려준다. 목록이 100종
// 남짓이라 한 번에 주고 화면에서 나눈다.
func (s *Store) BFFoods(ctx context.Context) ([]BFFood, error) {
	_, birth, _, err := s.bfProfileRow(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT i.name, i.kind, i.reaction, i.liked, i.tag_at, i.tag_by,
		        (SELECT min(m.dday) FROM bf_meal_items x JOIN bf_meals m ON m.id = x.meal_id
		          WHERE x.name = i.name),
		        (SELECT count(*) FROM bf_meal_items x WHERE x.name = i.name)
		   FROM bf_ingredients i ORDER BY i.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BFFood{}
	for rows.Next() {
		var f BFFood
		var reaction, liked int
		var first sql.NullInt64
		if err := rows.Scan(&f.Name, &f.Kind, &reaction, &liked, &f.TagAt, &f.TagBy, &first, &f.Uses); err != nil {
			return nil, err
		}
		f.Reaction, f.Liked = reaction != 0, liked != 0
		if first.Valid {
			d := int(first.Int64)
			f.FirstDDay = &d
			if birth != nil {
				if f.FirstDate, err = bfDate(*birth, d); err != nil {
					return nil, err
				}
			}
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// BFFoodTag는 재료에 달 표시다. nil이면 안 건드린다.
type BFFoodTag struct {
	Reaction *bool
	Liked    *bool
}

// BFTagFood는 알레르기 반응·좋아함을 켜고 끈다.
func (s *Store) BFTagFood(ctx context.Context, name string, in BFFoodTag, userID int64) (BFFood, error) {
	name, err := bfCleanName(name)
	if err != nil {
		return BFFood{}, err
	}
	if name == "" {
		return BFFood{}, invalid("재료를 골라주세요")
	}
	if in.Reaction == nil && in.Liked == nil {
		return BFFood{}, invalid("바꿀 내용이 없어요")
	}
	sets := []string{"tag_at=?", "tag_by=?"}
	args := []any{time.Now().Unix(), userID}
	if in.Reaction != nil {
		sets = append(sets, "reaction=?")
		args = append(args, boolInt(*in.Reaction))
	}
	if in.Liked != nil {
		sets = append(sets, "liked=?")
		args = append(args, boolInt(*in.Liked))
	}
	args = append(args, name)
	// sets 의 원소는 전부 위에서 만든 상수 문자열이고 값은 ? 로만 들어간다.
	res, err := s.db.ExecContext(ctx,
		"UPDATE bf_ingredients SET "+strings.Join(sets, ", ")+" WHERE name=?", args...)
	if err != nil {
		return BFFood{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return BFFood{}, ErrNotFound
	}
	foods, err := s.BFFoods(ctx)
	if err != nil {
		return BFFood{}, err
	}
	for _, f := range foods {
		if f.Name == name {
			return f, nil
		}
	}
	return BFFood{}, ErrNotFound
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
