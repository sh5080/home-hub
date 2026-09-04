package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 식단은 D+n(생후 일수)으로 저장하고 날짜는 생일로 그때그때 환산한다.
// 재고 = 마지막 실사 − 그 뒤 소모 + 그 뒤 제조.

const (
	bfDefaultHorizon = 21 // 3주
	bfMaxHorizon     = 180
	bfMaxToppings    = 12
	bfMaxNameLen     = 40
)

// BFChild 는 이유식 대상 아이. 생년월일은 users 에 있다.
type BFChild struct {
	UserID      int64  `json:"user_id"`
	Name        string `json:"name"`
	BirthDate   string `json:"birth_date"`
	HorizonDays int    `json:"horizon_days"`
	// 파생값
	TodayDDay int    `json:"today_dday"`
	Today     string `json:"today"`
	FromDDay  *int   `json:"from_dday"`
	ToDDay    *int   `json:"to_dday"`
	Days      int    `json:"days"` // 들어 있는 식단 일수
	// 하루 몇 끼일 때 몇 시에 먹이는지. {1:["12:00"], 2:[...], ...}
	MealTimes map[int][]string `json:"meal_times"`
}

// 끼니 이름은 그날 끼니 수로 정한다(1끼=점심). 저장된 slot 은 이름표일 뿐이다.
var bfSlotNames = map[int][]string{
	1: {"점심"},
	2: {"점심", "저녁"},
	3: {"아침", "점심", "저녁"},
}

// bf_meal_times 가 아직 없을 때의 기본 시각.
var bfDefaultTimes = map[int][]string{
	1: {"12:00"},
	2: {"12:00", "18:00"},
	3: {"09:00", "12:00", "18:00"},
}

// BFMealSrc 는 시드 당시의 끼니 구성(수정 표시용).
type BFMealSrc struct {
	Base     string   `json:"base"`
	Toppings []string `json:"toppings"`
	Snack    *string  `json:"snack"`
}

// BFMeal은 한 끼다.
type BFMeal struct {
	ID   int64  `json:"id"`
	Slot string `json:"slot"`
	// Title 은 그날 끼니 수로 정한 이름. 화면은 Slot 대신 이걸 쓴다.
	Title string `json:"title"`
	// At 은 'HH:MM'. 직접 넣은 값이 없으면 설정의 기본값.
	At    string `json:"at"`
	AtSet bool   `json:"at_set"`
	// Eaten 은 이 끼니에 걸린 이유식 기록. nil 이면 아직 안 먹였다.
	Eaten    *BFEaten  `json:"eaten"`
	Base     string    `json:"base"`
	Toppings []string  `json:"toppings"`
	Snack    *string   `json:"snack"`
	Src      BFMealSrc `json:"src"`
	Edited   bool      `json:"edited"`
	EatenG   *int      `json:"eaten_g"`
	ServedG  *int      `json:"served_g"`
	Skipped  bool      `json:"skipped"`
	EditedBy *int64    `json:"edited_by"`
	EditedAt *int64    `json:"edited_at"`
}

// BFEaten 은 끼니에 걸린 이유식 기록의 요약이다.
type BFEaten struct {
	LogID    int64  `json:"log_id"`
	At       string `json:"at"`
	AmountML *int   `json:"amount_ml"`
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
	// 그날 남긴 반응·좋아함 기록. 표시가 있는 재료만 들어온다.
	Logs []BFLog `json:"logs"`
}

// BFLog 는 (아이, 날짜, 재료) 하나의 기록.
// 좋아함·싫어함은 서로를 끄고, 셋 다 꺼지면 행을 지운다.
type BFLog struct {
	DDay     int    `json:"dday"`
	Date     string `json:"date"`
	Name     string `json:"name"`
	Reaction bool   `json:"reaction"`
	Liked    bool   `json:"liked"`
	Disliked bool   `json:"disliked"`
	At       int64  `json:"at"`
	By       *int64 `json:"by"`
}

// BFStock 은 재료 한 줄. 재고가 어떻게 나왔는지 구성요소를 같이 준다.
type BFStock struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Need  int    `json:"need"`  // 기간 내 필요 개수
	Stock *int   `json:"stock"` // nil이면 아직 실사 안 함
	Make  int    `json:"make"`  // 제조 필요 = max(0, need - stock)

	CountQty     *int   `json:"count_qty"`
	CountDDay    *int   `json:"count_dday"`
	CountAt      *int64 `json:"count_at"`
	countBatchID int64
	Used         int `json:"used"` // 실사 다음날 ~ 어제 소모
	Made         int `json:"made"` // 실사 이후 제조
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

// bfClock 은 테스트가 '오늘'을 고정하는 갈고리.
var bfClock = func() time.Time { return time.Now() }

func bfToday() string { return bfClock().Format("2006-01-02") }

// bfDDay는 생년월일과 날짜로 생후 일수를 구한다.
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

// --- 아이 ---

// BFChildren 은 이유식 대상 아이들이다.
func (s *Store) BFChildren(ctx context.Context) ([]BFChild, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.user_id, u.name, COALESCE(u.birth_date, ''), c.horizon_days,
		       (SELECT count(*) FROM bf_days d WHERE d.child_id = c.user_id),
		       (SELECT min(dday) FROM bf_days d WHERE d.child_id = c.user_id),
		       (SELECT max(dday) FROM bf_days d WHERE d.child_id = c.user_id)
		  FROM bf_children c JOIN users u ON u.id = c.user_id
		 ORDER BY u.birth_date, u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	today := bfToday()
	out := []BFChild{}
	for rows.Next() {
		var c BFChild
		var lo, hi sql.NullInt64
		if err := rows.Scan(&c.UserID, &c.Name, &c.BirthDate, &c.HorizonDays, &c.Days, &lo, &hi); err != nil {
			return nil, err
		}
		c.Today = today
		if c.BirthDate != "" {
			if c.TodayDDay, err = bfDDay(c.BirthDate, today); err != nil {
				return nil, err
			}
		}
		if lo.Valid {
			a, b := int(lo.Int64), int(hi.Int64)
			c.FromDDay, c.ToDDay = &a, &b
		}
		out = append(out, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// 커넥션이 하나라 rows 를 닫은 뒤에 다음 질의를 한다.
	times, err := s.bfAllMealTimes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].MealTimes = times[out[i].UserID]
		if out[i].MealTimes == nil {
			out[i].MealTimes = bfCopyTimes(bfDefaultTimes)
		}
	}
	return out, nil
}

// bfAllMealTimes는 아이별 시간표를 한 번에 읽는다.
func (s *Store) bfAllMealTimes(ctx context.Context) (map[int64]map[int][]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT child_id, n, pos, at FROM bf_meal_times ORDER BY child_id, n, pos`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]map[int][]string{}
	for rows.Next() {
		var child int64
		var n, pos int
		var at string
		if err := rows.Scan(&child, &n, &pos, &at); err != nil {
			return nil, err
		}
		if out[child] == nil {
			out[child] = map[int][]string{}
		}
		for len(out[child][n]) < pos {
			out[child][n] = append(out[child][n], "")
		}
		out[child][n] = append(out[child][n], at)
	}
	return out, rows.Err()
}

func bfCopyTimes(src map[int][]string) map[int][]string {
	out := make(map[int][]string, len(src))
	for k, v := range src {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// bfChild 는 한 아이를 찾는다.
func (s *Store) bfChild(ctx context.Context, childID int64) (BFChild, error) {
	all, err := s.BFChildren(ctx)
	if err != nil {
		return BFChild{}, err
	}
	for _, c := range all {
		if c.UserID == childID {
			if c.BirthDate == "" {
				return BFChild{}, invalid("아이의 생년월일이 없어요")
			}
			return c, nil
		}
	}
	return BFChild{}, ErrNotFound
}

// BFAddChild는 가족 한 명을 이유식 대상으로 삼는다.
func (s *Store) BFAddChild(ctx context.Context, userID int64, horizon int) error {
	if horizon <= 0 {
		horizon = bfDefaultHorizon
	}
	if horizon < 1 || horizon > bfMaxHorizon {
		return invalid(fmt.Sprintf("기간은 1~%d일 사이여야 해요", bfMaxHorizon))
	}
	var birth *string
	err := s.db.QueryRowContext(ctx, `SELECT birth_date FROM users WHERE id=?`, userID).Scan(&birth)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if birth == nil || *birth == "" {
		return invalid("먼저 가족 설정에서 생년월일을 넣어주세요")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO bf_children (user_id, horizon_days, created_at) VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET horizon_days=excluded.horizon_days`,
		userID, horizon, time.Now().Unix()); err != nil {
		return err
	}
	if err := bfSeedMealTimes(ctx, tx, userID); err != nil {
		return err
	}
	return tx.Commit()
}

// bfCheckTime 은 'HH:MM'(초 없음)인지 본다.
func bfCheckTime(v string) error {
	if len(v) != 5 || v[2] != ':' {
		return invalid("시각은 09:00 처럼 적어주세요")
	}
	h := (int(v[0]-'0'))*10 + int(v[1]-'0')
	m := (int(v[3]-'0'))*10 + int(v[4]-'0')
	for _, c := range []byte{v[0], v[1], v[3], v[4]} {
		if c < '0' || c > '9' {
			return invalid("시각은 09:00 처럼 적어주세요")
		}
	}
	if h > 23 || m > 59 {
		return invalid("시각은 09:00 처럼 적어주세요")
	}
	return nil
}

// BFSetMealTimes 는 '하루 n끼일 때'의 기본 시각을 정한다(끼니 수마다 따로).
func (s *Store) BFSetMealTimes(ctx context.Context, childID int64, n int, times []string) error {
	if n < 1 || n > len(bfSlotNames) {
		return invalid(fmt.Sprintf("하루 1~%d끼까지예요", len(bfSlotNames)))
	}
	if len(times) != n {
		return invalid(fmt.Sprintf("%d끼면 시각도 %d개예요", n, n))
	}
	prev := ""
	for i, v := range times {
		v = strings.TrimSpace(v)
		if err := bfCheckTime(v); err != nil {
			return err
		}
		if i > 0 && v <= prev {
			return invalid("시각은 앞에서 뒤로 가야 해요")
		}
		times[i], prev = v, v
	}
	if _, err := s.bfChild(ctx, childID); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM bf_meal_times WHERE child_id=? AND n=?`, childID, n); err != nil {
		return err
	}
	for i, v := range times {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bf_meal_times (child_id, n, pos, at) VALUES (?, ?, ?, ?)`,
			childID, n, i, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// bfSeedMealTimes는 새로 들어온 아이에게 기본 시간표를 깔아준다.
func bfSeedMealTimes(ctx context.Context, tx *sql.Tx, childID int64) error {
	for n, times := range bfDefaultTimes {
		for i, v := range times {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO bf_meal_times (child_id, n, pos, at) VALUES (?, ?, ?, ?)
				 ON CONFLICT(child_id, n, pos) DO NOTHING`, childID, n, i, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// BFRemoveChild 는 대상에서만 뺀다. 식단·재고 기록은 남긴다.
func (s *Store) BFRemoveChild(ctx context.Context, userID int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM bf_children WHERE user_id=?`, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// BFAdoptOrphans 는 child_id 가 없는 이유식 자료를 한 아이에게 붙인다.
func (s *Store) BFAdoptOrphans(ctx context.Context, childID int64) (int64, error) {
	if _, err := s.bfChild(ctx, childID); err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE bf_days SET child_id=? WHERE child_id IS NULL`, childID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	for _, q := range []string{
		`UPDATE bf_ingredients SET child_id=? WHERE child_id IS NULL`,
		`UPDATE bf_batches SET child_id=? WHERE child_id IS NULL`,
	} {
		if _, err := tx.ExecContext(ctx, q, childID); err != nil {
			return 0, err
		}
	}
	return n, tx.Commit()
}

// --- 조회 ---

// BFRange는 [from, to] 구간(D+n 기준)의 식단을 돌려준다.
func (s *Store) BFRange(ctx context.Context, childID int64, from, to int) ([]BFDay, error) {
	if to < from {
		return nil, invalid("범위가 거꾸로예요")
	}
	if to-from > 400 {
		return nil, invalid("한 번에 400일까지만 볼 수 있어요")
	}
	child, err := s.bfChild(ctx, childID)
	if err != nil {
		return nil, err
	}
	birth := &child.BirthDate

	rows, err := s.db.QueryContext(ctx,
		`SELECT dday, stage, label, kind, new_item, new_item_src, note
		   FROM bf_days WHERE child_id=? AND dday BETWEEN ? AND ? ORDER BY dday`, childID, from, to)
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
		d.Meals, d.Logs = []BFMeal{}, []BFLog{}
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

	// 끼니·구성품을 각각 한 번에 읽는다(커넥션 하나 — 끼니마다 묻지 않는다).
	meals := map[int64]*BFMeal{}
	mrows, err := s.db.QueryContext(ctx,
		`SELECT m.id, d.dday, m.slot, m.at, m.src, m.eaten_g, m.served_g, m.skipped, m.edited_by, m.edited_at
		   FROM bf_meals m JOIN bf_days d ON d.id = m.day_id
		  WHERE d.child_id=? AND d.dday BETWEEN ? AND ? ORDER BY d.dday, m.pos`, childID, from, to)
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
		var at sql.NullString
		if err := mrows.Scan(&m.ID, &dday, &m.Slot, &at, &src, &m.EatenG, &m.ServedG,
			&skipped, &m.EditedBy, &m.EditedAt); err != nil {
			mrows.Close()
			return nil, err
		}
		m.Skipped = skipped != 0
		m.At, m.AtSet = at.String, at.Valid && at.String != ""
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
		   FROM bf_meal_items i
		   JOIN bf_meals m ON m.id = i.meal_id
		   JOIN bf_days  d ON d.id = m.day_id
		  WHERE d.child_id=? AND d.dday BETWEEN ? AND ? ORDER BY i.meal_id, i.role, i.pos`, childID, from, to)
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

	lrows, err := s.db.QueryContext(ctx,
		`SELECT dday, name, reaction, liked, disliked, logged_at, logged_by
		   FROM bf_logs WHERE child_id=? AND dday BETWEEN ? AND ? ORDER BY dday, name`,
		childID, from, to)
	if err != nil {
		return nil, err
	}
	for lrows.Next() {
		var l BFLog
		var reaction, liked, disliked int
		if err := lrows.Scan(&l.DDay, &l.Name, &reaction, &liked, &disliked, &l.At, &l.By); err != nil {
			lrows.Close()
			return nil, err
		}
		l.Reaction, l.Liked, l.Disliked = reaction != 0, liked != 0, disliked != 0
		if birth != nil {
			if l.Date, err = bfDate(*birth, l.DDay); err != nil {
				lrows.Close()
				return nil, err
			}
		}
		if d := days[l.DDay]; d != nil {
			d.Logs = append(d.Logs, l)
		}
	}
	lrows.Close()
	if err := lrows.Err(); err != nil {
		return nil, err
	}

	// 먹인 기록은 아래 루프가 끼니를 값으로 복사하기 전에 붙여야 한다.
	if len(flat) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(flat)), ",")
		ids := make([]any, len(flat))
		for i, f := range flat {
			ids[i] = f.meal.ID
		}
		erows, err := s.db.QueryContext(ctx,
			`SELECT meal_id, id, at, amount_ml FROM care_logs WHERE meal_id IN (`+ph+`) ORDER BY at`, ids...)
		if err != nil {
			return nil, err
		}
		for erows.Next() {
			var mid int64
			var e BFEaten
			if err := erows.Scan(&mid, &e.LogID, &e.At, &e.AmountML); err != nil {
				erows.Close()
				return nil, err
			}
			if m := meals[mid]; m != nil {
				v := e
				m.Eaten = &v
			}
		}
		erows.Close()
		if err := erows.Err(); err != nil {
			return nil, err
		}
	}

	for _, f := range flat {
		f.meal.Edited = mealDiffers(*f.meal)
		if d := days[f.dday]; d != nil {
			d.Meals = append(d.Meals, *f.meal)
		}
	}
	for _, d := range days {
		bfNameMeals(d, child.MealTimes)
	}

	out := make([]BFDay, 0, len(order))
	for _, k := range order {
		out = append(out, *days[k])
	}
	return out, nil
}

// bfNameMeals 는 끼니 수로 이름을, 기본 시간표로 시각을 채운다.
// 4끼 이상이면 저장된 이름표를 그대로 둔다.
func bfNameMeals(d *BFDay, times map[int][]string) {
	n := len(d.Meals)
	names, at := bfSlotNames[n], times[n]
	if at == nil {
		at = bfDefaultTimes[n]
	}
	for i := range d.Meals {
		m := &d.Meals[i]
		m.Title = m.Slot
		if i < len(names) {
			m.Title = names[i]
		}
		if !m.AtSet && i < len(at) {
			m.At = at[i]
		}
	}
}

func mealDiffers(m BFMeal) bool {
	if m.AtSet {
		return true
	}
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
	// At은 'HH:MM'. ""를 주면 설정의 기본 시각으로 되돌린다.
	At *string
	// Reset 이면 나머지를 무시하고 시드 원본으로(시각도 기본값으로).
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
	var childID int64
	var srcRaw string
	var base sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT d.dday, d.child_id, m.src,
		        (SELECT name FROM bf_meal_items WHERE meal_id=m.id AND role='base' LIMIT 1)
		   FROM bf_meals m JOIN bf_days d ON d.id = m.day_id
		  WHERE m.id=?`, id).Scan(&dday, &childID, &srcRaw, &base)
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

	// tx 를 잡기 전에 읽는다 — 커넥션이 하나라 tx 안에서 Query 하면 멈춘다.
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
		if err := bfPutItem(ctx, tx, childID, id, "base", 0, next.Base); err != nil {
			return BFDay{}, err
		}
	}
	for i, t := range next.Toppings {
		if err := bfPutItem(ctx, tx, childID, id, "topping", i, t); err != nil {
			return BFDay{}, err
		}
	}
	if next.Snack != nil {
		if err := bfPutItem(ctx, tx, childID, id, "snack", 0, *next.Snack); err != nil {
			return BFDay{}, err
		}
	}

	if in.Reset {
		if _, err := tx.ExecContext(ctx,
			`UPDATE bf_meals SET edited_by=NULL, edited_at=NULL, at=NULL WHERE id=?`, id); err != nil {
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
	if in.At != nil && !in.Reset {
		v := strings.TrimSpace(*in.At)
		if v == "" {
			if _, err := tx.ExecContext(ctx, `UPDATE bf_meals SET at=NULL WHERE id=?`, id); err != nil {
				return BFDay{}, err
			}
		} else {
			if err := bfCheckTime(v); err != nil {
				return BFDay{}, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE bf_meals SET at=? WHERE id=?`, v, id); err != nil {
				return BFDay{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return BFDay{}, err
	}

	days, err := s.BFRange(ctx, childID, dday, dday)
	if err != nil || len(days) == 0 {
		return BFDay{}, err
	}
	return days[0], nil
}

func bfSetGrams(ctx context.Context, tx *sql.Tx, id int64, col string, v int) error {
	if v > 5000 {
		return invalid("양이 너무 커요")
	}
	// col 은 호출부의 고정 문자열뿐이다.
	q := "UPDATE bf_meals SET " + col + "=? WHERE id=?"
	if v < 0 {
		_, err := tx.ExecContext(ctx, "UPDATE bf_meals SET "+col+"=NULL WHERE id=?", id)
		return err
	}
	_, err := tx.ExecContext(ctx, q, v, id)
	return err
}

// bfPutItem은 구성품 한 줄을 쓰고, 처음 보는 재료면 재료 목록에도 넣는다.
func bfPutItem(ctx context.Context, tx *sql.Tx, childID, mealID int64, role string, pos int, name string) error {
	kind := "cube"
	if role == "base" {
		kind = "base"
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO bf_ingredients (child_id, name, kind) VALUES (?, ?, ?)
		 ON CONFLICT(child_id, name) DO NOTHING`, childID, name, kind); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO bf_meal_items (meal_id, role, pos, name) VALUES (?, ?, ?, ?)`,
		mealID, role, pos, name)
	return err
}

// BFDayInput 은 메모·NEW 재료 수정. 반응·좋아함은 BFSetLog.
type BFDayInput struct {
	Note    *string
	NewItem *string // "" 로 지움
}

// BFUpdateDay는 그날의 메모와 NEW 재료를 고친다.
func (s *Store) BFUpdateDay(ctx context.Context, childID int64, dday int, in BFDayInput) (BFDay, error) {
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
	args = append(args, childID, dday)
	// sets 는 상수 문자열뿐, 값은 전부 ? 로 들어간다.
	res, err := s.db.ExecContext(ctx,
		"UPDATE bf_days SET "+strings.Join(sets, ", ")+" WHERE child_id=? AND dday=?", args...)
	if err != nil {
		return BFDay{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return BFDay{}, ErrNotFound
	}
	days, err := s.BFRange(ctx, childID, dday, dday)
	if err != nil || len(days) == 0 {
		return BFDay{}, err
	}
	return days[0], nil
}

// --- 재고 ---

// BFStockList 는 오늘부터 horizon 일치 필요량과 현재 재고를 계산한다.
// 소모 = 실사 다음날 ~ 어제, 필요 = 오늘 ~ 오늘+horizon-1 (겹치면 오늘치가 두 번 빠진다).
func (s *Store) BFStockList(ctx context.Context, childID int64, horizon int) (BFStockView, error) {
	child, err := s.bfChild(ctx, childID)
	if err != nil {
		return BFStockView{}, err
	}
	if horizon <= 0 {
		horizon = child.HorizonDays
	}
	if horizon < 1 || horizon > bfMaxHorizon {
		return BFStockView{}, invalid(fmt.Sprintf("기간은 1~%d일 사이여야 해요", bfMaxHorizon))
	}
	from := child.TodayDDay
	to := from + horizon - 1

	need, err := s.bfCountItems(ctx, childID, from, to)
	if err != nil {
		return BFStockView{}, err
	}
	past, err := s.bfCountItemsByDay(ctx, childID, from)
	if err != nil {
		return BFStockView{}, err
	}
	batches, err := s.bfBatchTotals(ctx, childID)
	if err != nil {
		return BFStockView{}, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT name, kind, count_qty, count_dday, count_at, count_batch_id
		   FROM bf_ingredients WHERE child_id=? ORDER BY name`, childID)
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
			// 실사한 적 없으면 재고는 '모름'이다 — 0 으로 두면 있는 걸 또 만든다.
			it.Make = it.Need
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return BFStockView{}, err
	}

	fromDate, _ := bfDate(child.BirthDate, from)
	toDate, _ := bfDate(child.BirthDate, to)
	return BFStockView{
		From: fromDate, To: toDate, FromDDay: from, ToDDay: to,
		HorizonDays: horizon, Items: out,
	}, nil
}

// bfCountItems 는 [from,to] 재료별 사용 횟수. menu 구간과 건너뛴 끼니는 뺀다.
func (s *Store) bfCountItems(ctx context.Context, childID int64, from, to int) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT i.name, count(*) FROM bf_meal_items i
		   JOIN bf_meals m ON m.id = i.meal_id
		   JOIN bf_days  d ON d.id = m.day_id
		  WHERE d.child_id=? AND d.kind='topping' AND m.skipped=0 AND d.dday BETWEEN ? AND ?
		  GROUP BY i.name`, childID, from, to)
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

// bfCountItemsByDay 는 today 이전 소모를 (재료, 날짜)별로 준다 — 재료마다 실사 시점이 다르다.
func (s *Store) bfCountItemsByDay(ctx context.Context, childID int64, today int) (map[string][]bfDayCount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT i.name, d.dday, count(*) FROM bf_meal_items i
		   JOIN bf_meals m ON m.id = i.meal_id
		   JOIN bf_days  d ON d.id = m.day_id
		  WHERE d.child_id=? AND d.kind='topping' AND m.skipped=0 AND d.dday < ?
		  GROUP BY i.name, d.dday`, childID, today)
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

func (s *Store) bfBatchTotals(ctx context.Context, childID int64) (map[string][]bfBatch, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, id, qty FROM bf_batches WHERE child_id=? ORDER BY name, id`, childID)
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

// BFCount 는 실사다. 이후의 소모·제조만 다시 반영된다.
func (s *Store) BFCount(ctx context.Context, childID int64, name string, qty int, userID int64) error {
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
	dday, err := s.bfTodayDDay(ctx, childID)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE bf_ingredients
		    SET count_qty=?, count_dday=?, count_at=?, count_by=?,
		        count_batch_id=(SELECT COALESCE(max(id), 0) FROM bf_batches WHERE child_id=?)
		  WHERE child_id=? AND name=?`,
		qty, dday, time.Now().Unix(), userID, childID, childID, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// BFAddBatch 는 큐브 제조 기록. 실사 이후 것만 재고에 더해진다.
func (s *Store) BFAddBatch(ctx context.Context, childID int64, name string, qty int, note string, userID int64) error {
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
	dday, err := s.bfTodayDDay(ctx, childID)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO bf_ingredients (child_id, name, kind) VALUES (?, ?, 'cube')
		 ON CONFLICT(child_id, name) DO NOTHING`, childID, name); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO bf_batches (child_id, name, qty, made_dday, made_at, made_by, note)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		childID, name, qty, dday, time.Now().Unix(), userID, note); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) bfTodayDDay(ctx context.Context, childID int64) (int, error) {
	c, err := s.bfChild(ctx, childID)
	if err != nil {
		return 0, err
	}
	return c.TodayDDay, nil
}

// BFDDayOf 는 생일과 날짜로 생후 일수(D+n, 태어난 날=0)를 구한다.
func BFDDayOf(birth, date string) (int, error) { return bfDDay(birth, date) }

// --- 음식별 표시 ---

// BFFood 는 재료 한 종류와 기록 요약이다.
type BFFood struct {
	Name string `json:"name"`
	// 검색처럼 여러 아이를 섞어 보여줄 때만 채운다.
	ChildID   int64  `json:"child_id,omitempty"`
	ChildName string `json:"child_name,omitempty"`
	Kind      string `json:"kind"` // 'base' | 'cube' | 'dish'
	// 한 번이라도 있었으면 true.
	Reaction      bool     `json:"reaction"`
	Liked         bool     `json:"liked"`
	Disliked      bool     `json:"disliked"`
	ReactionDates []string `json:"reaction_dates"`
	LikedDates    []string `json:"liked_dates"`
	DislikedDates []string `json:"disliked_dates"`
	LastAt        *int64   `json:"last_at"`
	LastBy        *int64   `json:"last_by"`
	FirstDDay     *int     `json:"first_dday"`
	FirstDate     string   `json:"first_date"`
	Uses          int      `json:"uses"`
}

// BFFoods 는 재료 전부를 요약과 함께 준다.
func (s *Store) BFFoods(ctx context.Context, childID int64) ([]BFFood, error) {
	child, err := s.bfChild(ctx, childID)
	if err != nil {
		return nil, err
	}
	birth := child.BirthDate

	// 기록을 먼저 통째로 읽어 둔다(재료마다 묻지 않는다).
	logs, err := s.bfLogsByName(ctx, childID, birth)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT i.name, i.kind,
		        (SELECT min(d.dday) FROM bf_meal_items x
		           JOIN bf_meals m ON m.id = x.meal_id
		           JOIN bf_days  d ON d.id = m.day_id
		          WHERE x.name = i.name AND d.child_id = i.child_id),
		        (SELECT count(*) FROM bf_meal_items x
		           JOIN bf_meals m ON m.id = x.meal_id
		           JOIN bf_days  d ON d.id = m.day_id
		          WHERE x.name = i.name AND d.child_id = i.child_id)
		   FROM bf_ingredients i WHERE i.child_id=? ORDER BY i.name`, childID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BFFood{}
	for rows.Next() {
		var f BFFood
		var first sql.NullInt64
		if err := rows.Scan(&f.Name, &f.Kind, &first, &f.Uses); err != nil {
			return nil, err
		}
		f.ReactionDates, f.LikedDates, f.DislikedDates = []string{}, []string{}, []string{}
		for _, l := range logs[f.Name] {
			when := l.Date
			if when == "" {
				when = fmt.Sprintf("D+%d", l.DDay)
			}
			if l.Reaction {
				f.Reaction = true
				f.ReactionDates = append(f.ReactionDates, when)
			}
			if l.Liked {
				f.Liked = true
				f.LikedDates = append(f.LikedDates, when)
			}
			if l.Disliked {
				f.Disliked = true
				f.DislikedDates = append(f.DislikedDates, when)
			}
			if f.LastAt == nil || l.At > *f.LastAt {
				at, by := l.At, l.By
				f.LastAt, f.LastBy = &at, by
			}
		}
		if first.Valid {
			d := int(first.Int64)
			f.FirstDDay = &d
			if f.FirstDate, err = bfDate(birth, d); err != nil {
				return nil, err
			}
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// bfLogsByName은 한 아이의 기록 전부를 재료 이름으로 묶어 돌려준다.
func (s *Store) bfLogsByName(ctx context.Context, childID int64, birth string) (map[string][]BFLog, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT dday, name, reaction, liked, disliked, logged_at, logged_by
		   FROM bf_logs WHERE child_id=? ORDER BY dday`, childID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]BFLog{}
	for rows.Next() {
		var l BFLog
		var reaction, liked, disliked int
		if err := rows.Scan(&l.DDay, &l.Name, &reaction, &liked, &disliked, &l.At, &l.By); err != nil {
			return nil, err
		}
		l.Reaction, l.Liked, l.Disliked = reaction != 0, liked != 0, disliked != 0
		if l.Date, err = bfDate(birth, l.DDay); err != nil {
			return nil, err
		}
		out[l.Name] = append(out[l.Name], l)
	}
	return out, rows.Err()
}

// BFFoodTag는 그날 그 재료에 남길 표시다. nil이면 안 건드린다.
type BFFoodTag struct {
	Reaction *bool
	Liked    *bool
	Disliked *bool
}

// BFSetLog 는 하루치 기록을 남긴다. 표시가 모두 꺼지면 행을 지운다.
func (s *Store) BFSetLog(ctx context.Context, childID int64, dday int, name string, in BFFoodTag, userID int64) (BFDay, error) {
	name, err := bfCleanName(name)
	if err != nil {
		return BFDay{}, err
	}
	if name == "" {
		return BFDay{}, invalid("재료를 골라주세요")
	}
	if in.Reaction == nil && in.Liked == nil && in.Disliked == nil {
		return BFDay{}, invalid("바꿀 내용이 없어요")
	}
	if _, err := s.bfChild(ctx, childID); err != nil {
		return BFDay{}, err
	}
	var cur BFLog
	var reaction, liked, disliked int
	had := true
	err = s.db.QueryRowContext(ctx,
		`SELECT reaction, liked, disliked FROM bf_logs WHERE child_id=? AND dday=? AND name=?`,
		childID, dday, name).Scan(&reaction, &liked, &disliked)
	switch {
	case err == sql.ErrNoRows:
		had = false
	case err != nil:
		return BFDay{}, err
	}
	cur.Reaction, cur.Liked, cur.Disliked = reaction != 0, liked != 0, disliked != 0

	// 새 기록은 그날 식단에 있는 재료에만 남긴다. 이미 있는 행은 고칠 수 있다.
	if !had {
		var onMenu int
		if err := s.db.QueryRowContext(ctx,
			`SELECT count(*) FROM bf_days d
			  WHERE d.child_id=? AND d.dday=?
			    AND (d.new_item = ?
			         OR EXISTS (SELECT 1 FROM bf_meals m
			                      JOIN bf_meal_items x ON x.meal_id = m.id
			                     WHERE m.day_id = d.id AND x.name = ?))`,
			childID, dday, name, name).Scan(&onMenu); err != nil {
			return BFDay{}, err
		}
		if onMenu == 0 {
			return BFDay{}, ErrNotFound
		}
	}
	if in.Reaction != nil {
		cur.Reaction = *in.Reaction
	}
	if in.Liked != nil {
		cur.Liked = *in.Liked
		// 좋아함과 싫어함은 서로를 끈다. 켠 쪽이 이긴다.
		if cur.Liked {
			cur.Disliked = false
		}
	}
	if in.Disliked != nil {
		cur.Disliked = *in.Disliked
		if cur.Disliked {
			cur.Liked = false
		}
	}

	if !cur.Reaction && !cur.Liked && !cur.Disliked {
		if _, err := s.db.ExecContext(ctx,
			`DELETE FROM bf_logs WHERE child_id=? AND dday=? AND name=?`, childID, dday, name); err != nil {
			return BFDay{}, err
		}
	} else if _, err := s.db.ExecContext(ctx,
		`INSERT INTO bf_logs (child_id, dday, name, reaction, liked, disliked, logged_at, logged_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(child_id, dday, name) DO UPDATE SET
		   reaction=excluded.reaction, liked=excluded.liked, disliked=excluded.disliked,
		   logged_at=excluded.logged_at, logged_by=excluded.logged_by`,
		childID, dday, name, boolInt(cur.Reaction), boolInt(cur.Liked), boolInt(cur.Disliked),
		time.Now().Unix(), userID); err != nil {
		return BFDay{}, err
	}

	days, err := s.BFRange(ctx, childID, dday, dday)
	if err != nil || len(days) == 0 {
		return BFDay{}, err
	}
	return days[0], nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// BFShoppingCard 는 기간 내 '제조 필요'를 장보기 카드 한 장으로 만든다.
func (s *Store) BFShoppingCard(ctx context.Context, childID int64, horizon int, userID int64) (Card, error) {
	view, err := s.BFStockList(ctx, childID, horizon)
	if err != nil {
		return Card{}, err
	}
	need := make([]BFStock, 0, len(view.Items))
	for _, it := range view.Items {
		if it.Make > 0 {
			need = append(need, it)
		}
	}
	if len(need) == 0 {
		return Card{}, invalid("지금은 만들어야 할 게 없어요")
	}
	sort.Slice(need, func(i, j int) bool {
		if need[i].Make != need[j].Make {
			return need[i].Make > need[j].Make
		}
		return need[i].Name < need[j].Name
	})
	items := make([]string, 0, len(need))
	for _, it := range need {
		items = append(items, fmt.Sprintf("%s %d개", it.Name, it.Make))
	}

	col, err := s.firstColumn(ctx)
	if err != nil {
		return Card{}, err
	}
	title := "이유식 장보기"
	note := fmt.Sprintf("%s ~ %s (%d일치) 기준이에요. 냉동실을 다시 세면 숫자가 바뀝니다.",
		view.From, view.To, view.HorizonDays)
	content := ChecklistContent(note, items)
	due := bfToday()
	return s.CreateCard(ctx, col, CardInput{Title: &title, Content: &content, DueAt: &due}, userID)
}

// firstColumn 은 첫 보드의 첫 칸(= 할 일). 칸의 뜻은 이름이 아니라 위치다.
func (s *Store) firstColumn(ctx context.Context) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `
		SELECT c.id FROM columns c
		  JOIN boards b ON b.id = c.board_id
		 ORDER BY b.id, c.position LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, invalid("보드가 없어요")
	}
	return id, err
}
