package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// CareKind 는 기록 종류 하나의 성질이다.
type CareKind struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Duration bool   `json:"duration"` // 얼마나 걸렸는지 적는가 (진행 중 상태가 있다)
	Amount   bool   `json:"amount"`   // 양(ml)을 적는가
	// Details 는 고를 수 있는 상세. 비어 있으면 자유 입력이거나 없음.
	Details []string `json:"details"`
	Free    bool     `json:"free"` // detail 을 자유 입력으로 받는가
}

// CareKinds 는 화면 버튼 순서 그대로다.
var CareKinds = []CareKind{
	{Key: "breast", Label: "모유", Duration: true, Details: []string{"왼쪽", "오른쪽", "양쪽"}},
	{Key: "formula", Label: "분유", Amount: true},
	{Key: "solids", Label: "이유식", Amount: true, Free: true},
	{Key: "diaper", Label: "기저귀", Details: []string{"소변", "대변", "둘 다"}},
	{Key: "sleep", Label: "수면", Duration: true, Details: []string{"낮잠", "밤잠"}},
	{Key: "pump_feed", Label: "유축 수유", Amount: true},
	{Key: "pump", Label: "유축", Duration: true, Amount: true},
	{Key: "bath", Label: "목욕", Duration: true},
	{Key: "hospital", Label: "병원", Free: true},
	{Key: "medicine", Label: "투약", Free: true},
	{Key: "snack", Label: "간식", Free: true},
	{Key: "milk", Label: "우유", Amount: true},
	{Key: "water", Label: "물", Amount: true},
	{Key: "play", Label: "놀이", Duration: true, Free: true},
	{Key: "tummy", Label: "터미타임", Duration: true},
	// 체온은 detail 에 '37.2°C' 로 적는다.
	{Key: "temp", Label: "체온", Free: true},
	{Key: "etc", Label: "기타", Duration: true, Free: true},
}

func careKind(key string) (CareKind, bool) {
	for _, k := range CareKinds {
		if k.Key == key {
			return k, true
		}
	}
	return CareKind{}, false
}

// CareLog 는 기록 한 줄이다.
type CareLog struct {
	ID        int64   `json:"id"`
	ChildID   int64   `json:"child_id"`
	Kind      string  `json:"kind"`
	At        string  `json:"at"` // 'YYYY-MM-DDTHH:MM'
	Minutes   *int    `json:"minutes"`
	AmountML  *int    `json:"amount_ml"`
	Detail    *string `json:"detail"`
	Note      string  `json:"note"`
	CreatedBy *int64  `json:"created_by"`
	CreatedAt int64   `json:"created_at"`
	// 이유식: 먹인 재료와 식단 끼니.
	Items  []CareItem `json:"items"`
	MealID *int64     `json:"meal_id"`
}

// CareItem 은 이유식 기록에 걸린 재료 하나다.
type CareItem struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// CareInput 은 생성·수정 값. nil 은 안 건드림, 음수 분/양은 지움(NULL).
type CareInput struct {
	Kind     *string
	At       *string
	Minutes  *int
	AmountML *int
	Detail   *string // "" 로 지움
	Note     *string
	// 이유식 재료 이름들. 재료 목록에서 찾아 id 로 건다(없으면 만든다).
	Ingredients *[]string
	// 식단 끼니. 0 이면 연결을 푼다.
	MealID *int64
}

const careMaxNote = 500

func careCheck(in CareInput, kind CareKind) error {
	if in.At != nil && !validDue(*in.At) {
		return invalid("시각이 올바르지 않아요")
	}
	// 앞으로의 시각은 받지 않는다(자정 넘겨 어젯밤 기록을 오늘 날짜로 적는 실수).
	// 폰 시계 차이만 5분 봐준다.
	if in.At != nil && len(*in.At) == 16 {
		if t, err := time.ParseInLocation("2006-01-02T15:04", *in.At, time.Local); err == nil && t.After(time.Now().Add(5*time.Minute)) {
			return invalid("아직 오지 않은 시각이에요. 어제 일이면 날짜를 어제로 바꿔주세요")
		}
	}
	if in.Minutes != nil && *in.Minutes > 24*60 {
		return invalid("하루를 넘길 수는 없어요")
	}
	if in.AmountML != nil && *in.AmountML > 5000 {
		return invalid("양이 너무 커요")
	}
	if in.Detail != nil {
		d := strings.TrimSpace(*in.Detail)
		if d != "" && !kind.Free && len(kind.Details) > 0 {
			ok := false
			for _, v := range kind.Details {
				ok = ok || v == d
			}
			if !ok {
				return invalid("고를 수 없는 값이에요: " + d)
			}
		}
		if len([]rune(d)) > 100 {
			return invalid("상세가 너무 길어요")
		}
	}
	if in.Note != nil && len([]rune(*in.Note)) > careMaxNote {
		return invalid("메모가 너무 길어요")
	}
	return nil
}

// CareAdd 는 기록을 남긴다. 시각이 비면 지금.
func (s *Store) CareAdd(ctx context.Context, childID int64, in CareInput, userID int64) (CareLog, error) {
	if in.Kind == nil {
		return CareLog{}, invalid("종류를 골라주세요")
	}
	kind, ok := careKind(*in.Kind)
	if !ok {
		return CareLog{}, invalid("모르는 종류예요: " + *in.Kind)
	}
	if _, err := s.bfChild(ctx, childID); err != nil {
		return CareLog{}, err
	}
	at := time.Now().Format("2006-01-02T15:04")
	if in.At != nil && *in.At != "" {
		at = *in.At
	} else {
		in.At = &at
	}
	if err := careCheck(in, kind); err != nil {
		return CareLog{}, err
	}
	var minutes, amount any
	if in.Minutes != nil && *in.Minutes >= 0 {
		minutes = *in.Minutes
	}
	if in.AmountML != nil && *in.AmountML >= 0 {
		amount = *in.AmountML
	}
	var detail any
	if in.Detail != nil && strings.TrimSpace(*in.Detail) != "" {
		detail = strings.TrimSpace(*in.Detail)
	}
	note := ""
	if in.Note != nil {
		note = strings.TrimSpace(*in.Note)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO care_logs (child_id, kind, at, minutes, amount_ml, detail, note, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		childID, kind.Key, at, minutes, amount, detail, note, userID, time.Now().Unix())
	if err != nil {
		return CareLog{}, err
	}
	id, _ := res.LastInsertId()
	l, err := s.CareGet(ctx, id)
	if err != nil {
		return CareLog{}, err
	}
	if err := s.careApplySolids(ctx, l, in); err != nil {
		return CareLog{}, err
	}
	return s.CareGet(ctx, id)
}

// CareGet 은 한 줄을 읽는다.
func (s *Store) CareGet(ctx context.Context, id int64) (CareLog, error) {
	var l CareLog
	err := s.db.QueryRowContext(ctx,
		`SELECT id, child_id, kind, at, minutes, amount_ml, detail, note, created_by, created_at, meal_id
		   FROM care_logs WHERE id=?`, id).
		Scan(&l.ID, &l.ChildID, &l.Kind, &l.At, &l.Minutes, &l.AmountML, &l.Detail, &l.Note, &l.CreatedBy, &l.CreatedAt, &l.MealID)
	if errors.Is(err, sql.ErrNoRows) {
		return CareLog{}, ErrNotFound
	}
	if err != nil {
		return CareLog{}, err
	}
	one := []CareLog{l}
	if err := s.careAttachItems(ctx, one); err != nil {
		return CareLog{}, err
	}
	return one[0], nil
}

// careAttachItems 는 이유식 기록들에 재료를 한 번의 질의로 붙인다.
func (s *Store) careAttachItems(ctx context.Context, logs []CareLog) error {
	idx := map[int64]int{}
	var ids []any
	for i := range logs {
		logs[i].Items = []CareItem{}
		if logs[i].Kind == "solids" {
			idx[logs[i].ID] = i
			ids = append(ids, logs[i].ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	// 자리표시자 개수만 이어 붙인다. 값은 전부 ?.
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.log_id, i.id, i.name FROM care_log_items c JOIN bf_ingredients i ON i.id = c.ingredient_id
		  WHERE c.log_id IN (`+ph+`) ORDER BY c.log_id, c.pos`, ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var logID int64
		var it CareItem
		if err := rows.Scan(&logID, &it.ID, &it.Name); err != nil {
			return err
		}
		if i, ok := idx[logID]; ok {
			logs[i].Items = append(logs[i].Items, it)
		}
	}
	return rows.Err()
}

// careSetItems 는 재료 이름을 id 로 건다(없으면 만든다). detail 은 이름을 이은 사본.
// tx 안에서 부르므로 조회도 tx 로 한다.
func careSetItems(ctx context.Context, tx *sql.Tx, childID, logID int64, names []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM care_log_items WHERE log_id=?`, logID); err != nil {
		return err
	}
	var clean []string
	seen := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		if len([]rune(n)) > 60 {
			return invalid("재료 이름이 너무 길어요")
		}
		seen[n] = true
		clean = append(clean, n)
	}
	for pos, n := range clean {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bf_ingredients (child_id, name, kind) VALUES (?, ?, 'cube') ON CONFLICT(child_id, name) DO NOTHING`,
			childID, n); err != nil {
			return err
		}
		var ingID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM bf_ingredients WHERE child_id=? AND name=?`, childID, n).Scan(&ingID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO care_log_items (log_id, pos, ingredient_id) VALUES (?, ?, ?)`, logID, pos, ingID); err != nil {
			return err
		}
	}
	var detail any
	if len(clean) > 0 {
		detail = strings.Join(clean, ", ")
	}
	_, err := tx.ExecContext(ctx, `UPDATE care_logs SET detail=? WHERE id=?`, detail, logID)
	return err
}

// careCheckMeal 은 끼니가 그 아이의 것인지 본다.
func careCheckMeal(ctx context.Context, tx *sql.Tx, childID, mealID int64) error {
	var n int
	if err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM bf_meals m JOIN bf_days d ON d.id = m.day_id WHERE m.id=? AND d.child_id=?`,
		mealID, childID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return invalid("그 아이의 식단 끼니가 아니에요")
	}
	return nil
}

// careApplySolids 는 재료·끼니를 반영한다(이유식만).
func (s *Store) careApplySolids(ctx context.Context, l CareLog, in CareInput) error {
	if l.Kind != "solids" || (in.Ingredients == nil && in.MealID == nil) {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if in.Ingredients != nil {
		if err := careSetItems(ctx, tx, l.ChildID, l.ID, *in.Ingredients); err != nil {
			return err
		}
	}
	if in.MealID != nil {
		if *in.MealID == 0 {
			if _, err := tx.ExecContext(ctx, `UPDATE care_logs SET meal_id=NULL WHERE id=?`, l.ID); err != nil {
				return err
			}
		} else {
			if err := careCheckMeal(ctx, tx, l.ChildID, *in.MealID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE care_logs SET meal_id=? WHERE id=?`, *in.MealID, l.ID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// CareUpdate 는 부분 수정. 종류는 바꾸지 않는다.
func (s *Store) CareUpdate(ctx context.Context, id int64, in CareInput) (CareLog, error) {
	cur, err := s.CareGet(ctx, id)
	if err != nil {
		return CareLog{}, err
	}
	kind, _ := careKind(cur.Kind)
	if err := careCheck(in, kind); err != nil {
		return CareLog{}, err
	}
	sets, args := []string{}, []any{}
	if in.At != nil {
		sets, args = append(sets, "at=?"), append(args, *in.At)
	}
	if in.Minutes != nil {
		if *in.Minutes < 0 {
			sets = append(sets, "minutes=NULL")
		} else {
			sets, args = append(sets, "minutes=?"), append(args, *in.Minutes)
		}
	}
	if in.AmountML != nil {
		if *in.AmountML < 0 {
			sets = append(sets, "amount_ml=NULL")
		} else {
			sets, args = append(sets, "amount_ml=?"), append(args, *in.AmountML)
		}
	}
	if in.Detail != nil {
		d := strings.TrimSpace(*in.Detail)
		if d == "" {
			sets = append(sets, "detail=NULL")
		} else {
			sets, args = append(sets, "detail=?"), append(args, d)
		}
	}
	if in.Note != nil {
		sets, args = append(sets, "note=?"), append(args, strings.TrimSpace(*in.Note))
	}
	if len(sets) > 0 {
		args = append(args, id)
		// sets 는 상수 문자열뿐, 값은 ?.
		if _, err := s.db.ExecContext(ctx, "UPDATE care_logs SET "+strings.Join(sets, ", ")+" WHERE id=?", args...); err != nil {
			return CareLog{}, err
		}
	}
	// 재료를 주면 detail 은 재료에서 다시 만든다.
	if err := s.careApplySolids(ctx, cur, in); err != nil {
		return CareLog{}, err
	}
	return s.CareGet(ctx, id)
}

// CareDelete 는 한 줄을 지운다.
func (s *Store) CareDelete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM care_logs WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CareDay 는 하루치 기록과 그날의 합계다.
type CareDay struct {
	Date      string    `json:"date"`
	Logs      []CareLog `json:"logs"`       // 늦은 것부터
	FormulaML int       `json:"formula_ml"` // 분유 + 유축 수유 + 우유
	SolidsML  int       `json:"solids_ml"`  // 이유식
	SleepMin  int       `json:"sleep_min"`  // 수면 합계(끝난 것만)
	NightMin  int       `json:"night_min"`  // 그중 밤잠
	NapMin    int       `json:"nap_min"`    // 그중 낮잠 (밤잠이라 적지 않은 건 전부)
	Diapers   int       `json:"diapers"`
	BreastMin int       `json:"breast_min"`
	Feedings  int       `json:"feedings"` // 모유·분유·유축 수유 횟수
}

// CareList 는 그날의 기록을 늦은 것부터 준다.
func (s *Store) CareList(ctx context.Context, childID int64, date string) (CareDay, error) {
	if !validDate(date) {
		return CareDay{}, invalid("날짜가 올바르지 않아요")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, child_id, kind, at, minutes, amount_ml, detail, note, created_by, created_at, meal_id
		   FROM care_logs WHERE child_id=? AND at >= ? AND at < ?
		  ORDER BY at DESC, id DESC`, childID, date+"T00:00", date+"T24:00")
	if err != nil {
		return CareDay{}, err
	}
	defer rows.Close()
	out := CareDay{Date: date, Logs: []CareLog{}}
	for rows.Next() {
		var l CareLog
		if err := rows.Scan(&l.ID, &l.ChildID, &l.Kind, &l.At, &l.Minutes, &l.AmountML, &l.Detail, &l.Note, &l.CreatedBy, &l.CreatedAt, &l.MealID); err != nil {
			return CareDay{}, err
		}
		out.Logs = append(out.Logs, l)
		ml, min := 0, 0
		if l.AmountML != nil {
			ml = *l.AmountML
		}
		if l.Minutes != nil {
			min = *l.Minutes
		}
		switch l.Kind {
		case "formula", "pump_feed", "milk":
			out.FormulaML += ml
			out.Feedings++
		case "breast":
			out.BreastMin += min
			out.Feedings++
		case "solids":
			out.SolidsML += ml
		case "sleep":
			out.SleepMin += min
			if l.Detail != nil && *l.Detail == "밤잠" {
				out.NightMin += min
			} else {
				out.NapMin += min
			}
		case "diaper":
			out.Diapers++
		}
	}
	if err := rows.Err(); err != nil {
		return CareDay{}, err
	}
	rows.Close()
	if err := s.careAttachItems(ctx, out.Logs); err != nil {
		return CareDay{}, err
	}
	return out, nil
}

// CareLast 는 종류별 마지막 기록(오늘로 한정하지 않는다).
func (s *Store) CareLast(ctx context.Context, childID int64) (map[string]CareLog, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, child_id, kind, at, minutes, amount_ml, detail, note, created_by, created_at, meal_id
		   FROM care_logs WHERE child_id=? AND id IN (
		     SELECT (SELECT id FROM care_logs c2 WHERE c2.child_id = c1.child_id AND c2.kind = c1.kind ORDER BY at DESC, id DESC LIMIT 1)
		       FROM care_logs c1 WHERE c1.child_id=? GROUP BY c1.kind)`, childID, childID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]CareLog{}
	for rows.Next() {
		var l CareLog
		if err := rows.Scan(&l.ID, &l.ChildID, &l.Kind, &l.At, &l.Minutes, &l.AmountML, &l.Detail, &l.Note, &l.CreatedBy, &l.CreatedAt, &l.MealID); err != nil {
			return nil, err
		}
		l.Items = []CareItem{}
		out[l.Kind] = l
	}
	return out, rows.Err()
}

// CareImportEntry 는 다른 앱에서 가져올 기록 한 줄이다.
type CareImportEntry struct {
	Source   string
	Kind     string
	At       string
	Minutes  *int
	AmountML *int
	Detail   string
	Note     string
}

// CareImport 는 출처가 같은 줄과 손기록(같은 종류·시각)을 건너뛴다.
// (넣음, 출처 중복, 손기록과 겹침).
func (s *Store) CareImport(ctx context.Context, childID int64, es []CareImportEntry, userID int64) (added, dupSource, dupManual int, err error) {
	// 손기록은 tx 전에 읽는다(커넥션 하나).
	manual := map[string]bool{}
	rows, err := s.db.QueryContext(ctx, `SELECT kind, at FROM care_logs WHERE child_id=? AND source IS NULL`, childID)
	if err != nil {
		return 0, 0, 0, err
	}
	for rows.Next() {
		var k, at string
		if err := rows.Scan(&k, &at); err != nil {
			rows.Close()
			return 0, 0, 0, err
		}
		manual[k+"|"+at] = true
	}
	rows.Close()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, 0, err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	for _, e := range es {
		if _, ok := careKind(e.Kind); !ok {
			return 0, 0, 0, invalid("모르는 종류: " + e.Kind)
		}
		if !validDateTime(e.At) {
			return 0, 0, 0, invalid("시각이 이상해요: " + e.At)
		}
		if manual[e.Kind+"|"+e.At] {
			dupManual++
			continue
		}
		var detail any
		if e.Detail != "" {
			detail = e.Detail
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO care_logs (child_id, kind, at, minutes, amount_ml, detail, note, created_by, created_at, source)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(source) WHERE source IS NOT NULL DO NOTHING`,
			childID, e.Kind, e.At, e.Minutes, e.AmountML, detail, e.Note, userID, now, e.Source)
		if err != nil {
			return 0, 0, 0, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			dupSource++
		} else {
			added++
		}
	}
	return added, dupSource, dupManual, tx.Commit()
}

// CareSolidsDates 는 이유식 기록이 있는 날짜들이다.
func (s *Store) CareSolidsDates(ctx context.Context, childID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT substr(at, 1, 10) FROM care_logs WHERE child_id=? AND kind='solids' ORDER BY 1`, childID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
