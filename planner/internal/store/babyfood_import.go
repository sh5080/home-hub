package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// 식단 데이터 파일(JSON)을 읽는다. 내용은 저장소에 없다.

// BFPlanFile은 식단 데이터 파일의 최상위 구조다.
type BFPlanFile struct {
	Schema       int             `json:"schema"`
	DefaultTrack []string        `json:"default_track"`
	Plans        []BFPlanSection `json:"plans"`
	Ingredients  []BFPlanIngr    `json:"ingredients"`
}

type BFPlanSection struct {
	ID    string      `json:"id"`
	Label string      `json:"label"`
	Kind  string      `json:"kind"` // 'topping' | 'menu'
	From  int         `json:"from"`
	To    int         `json:"to"`
	Days  []BFPlanDay `json:"days"`
}

// BFPlanDay 는 하루. D 는 생후 일수.
type BFPlanDay struct {
	D     int          `json:"d"`
	New   string       `json:"new"`
	Meals []BFPlanMeal `json:"meals"`
}

type BFPlanMeal struct {
	Slot     string   `json:"slot"`
	Base     string   `json:"base"`
	Toppings []string `json:"toppings"`
	Snack    *string  `json:"snack"`
}

// BFPlanIngr 는 재료 분류. base/cube 는 큐브로 세고 dish 는 따로 묶는다.
type BFPlanIngr struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// BFImportPlan 은 가져오기 요약(dry-run 도 같은 값).
type BFImportPlan struct {
	Sections []BFImportSection `json:"sections"`
	// Existing 은 이미 있어서 건너뛴 날 수(손으로 고친 걸 덮지 않는다).
	Existing    int `json:"existing"`
	NewDays     int `json:"new_days"`
	NewMeals    int `json:"new_meals"`
	Ingredients int `json:"ingredients"`
}

type BFImportSection struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	From  int    `json:"from"`
	To    int    `json:"to"`
	Days  int    `json:"days"`
	Skip  int    `json:"skip"`
}

// ParseBFPlan은 파일 내용을 구조로 바꾸고 최소한의 검증을 한다.
func ParseBFPlan(raw []byte) (*BFPlanFile, error) {
	var f BFPlanFile
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("식단 파일을 읽지 못했어요: %w", err)
	}
	if f.Schema != 1 {
		return nil, invalid(fmt.Sprintf("모르는 schema %d", f.Schema))
	}
	if len(f.Plans) == 0 {
		return nil, invalid("구간이 하나도 없어요")
	}
	for _, p := range f.Plans {
		if p.ID == "" {
			return nil, invalid("구간에 id가 없어요")
		}
		if p.Kind != "topping" && p.Kind != "menu" {
			return nil, invalid(fmt.Sprintf("구간 %s: kind는 topping 또는 menu 예요", p.ID))
		}
		seen := map[int]bool{}
		for _, d := range p.Days {
			if seen[d.D] {
				return nil, invalid(fmt.Sprintf("구간 %s: D+%d 가 두 번 나와요", p.ID, d.D))
			}
			seen[d.D] = true
			if len(d.Meals) == 0 {
				return nil, invalid(fmt.Sprintf("구간 %s: D+%d 에 끼니가 없어요", p.ID, d.D))
			}
			for _, m := range d.Meals {
				if m.Slot == "" {
					return nil, invalid(fmt.Sprintf("구간 %s: D+%d 끼니에 slot이 없어요", p.ID, d.D))
				}
			}
		}
	}
	return &f, nil
}

// BFSelectTrack 은 고른 구간을 D+n 순으로 편다. 같은 D+n 은 먼저 고른 쪽이 이긴다.
func BFSelectTrack(f *BFPlanFile, track []string) ([]BFPlanSection, error) {
	if len(track) == 0 {
		track = f.DefaultTrack
	}
	if len(track) == 0 {
		return nil, invalid("어떤 구간을 쓸지 정해주세요")
	}
	by := map[string]BFPlanSection{}
	for _, p := range f.Plans {
		by[p.ID] = p
	}
	out := make([]BFPlanSection, 0, len(track))
	taken := map[int]string{}
	for _, id := range track {
		p, ok := by[id]
		if !ok {
			return nil, invalid("모르는 구간: " + id)
		}
		for _, d := range p.Days {
			if prev, dup := taken[d.D]; dup {
				return nil, invalid(fmt.Sprintf("D+%d 를 %s 와 %s 가 겹쳐서 채워요", d.D, prev, id))
			}
			taken[d.D] = id
		}
		out = append(out, p)
	}
	return out, nil
}

// BFImport 는 고른 구간을 넣는다. apply=false 면 요약만(기본 dry-run).
// replace=false 면 이미 있는 날은 건너뛴다. fromDDay>0 이면 그 날 이후만.
func (s *Store) BFImport(ctx context.Context, childID int64, f *BFPlanFile, track []string, apply, replace bool, fromDDay int) (BFImportPlan, error) {
	secs, err := BFSelectTrack(f, track)
	if err != nil {
		return BFImportPlan{}, err
	}

	existing := map[int]bool{}
	rows, err := s.db.QueryContext(ctx, `SELECT dday FROM bf_days WHERE child_id=?`, childID)
	if err != nil {
		return BFImportPlan{}, err
	}
	for rows.Next() {
		var d int
		if err := rows.Scan(&d); err != nil {
			rows.Close()
			return BFImportPlan{}, err
		}
		existing[d] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return BFImportPlan{}, err
	}

	var plan BFImportPlan
	plan.Ingredients = len(f.Ingredients)
	type write struct {
		sec BFPlanSection
		day BFPlanDay
	}
	var writes []write
	for _, p := range secs {
		sum := BFImportSection{ID: p.ID, Label: p.Label, From: p.From, To: p.To}
		days := append([]BFPlanDay(nil), p.Days...)
		sort.Slice(days, func(i, j int) bool { return days[i].D < days[j].D })
		for _, d := range days {
			if fromDDay > 0 && d.D < fromDDay {
				continue
			}
			if existing[d.D] && !replace {
				sum.Skip++
				plan.Existing++
				continue
			}
			sum.Days++
			plan.NewDays++
			plan.NewMeals += len(d.Meals)
			writes = append(writes, write{p, d})
		}
		plan.Sections = append(plan.Sections, sum)
	}
	if !apply {
		return plan, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BFImportPlan{}, err
	}
	defer tx.Rollback()

	// 재료는 kind 만 맞춘다 — 실사값은 건드리지 않는다.
	for _, ing := range f.Ingredients {
		kind := ing.Kind
		if kind == "" {
			kind = "cube"
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bf_ingredients (child_id, name, kind) VALUES (?, ?, ?)
			 ON CONFLICT(child_id, name) DO UPDATE SET kind=excluded.kind`, childID, ing.Name, kind); err != nil {
			return BFImportPlan{}, err
		}
	}

	for _, w := range writes {
		if replace {
			// bf_meals / bf_meal_items 는 ON DELETE CASCADE 로 따라 지워진다.
			if _, err := tx.ExecContext(ctx, `DELETE FROM bf_days WHERE child_id=? AND dday=?`, childID, w.day.D); err != nil {
				return BFImportPlan{}, err
			}
		}
		var newItem *string
		if w.day.New != "" {
			v := w.day.New
			newItem = &v
		}
		dayRes, err := tx.ExecContext(ctx,
			`INSERT INTO bf_days (child_id, dday, stage, label, kind, new_item, new_item_src)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			childID, w.day.D, w.sec.ID, w.sec.Label, w.sec.Kind, newItem, newItem)
		if err != nil {
			return BFImportPlan{}, err
		}
		dayID, _ := dayRes.LastInsertId()
		for i, m := range w.day.Meals {
			tops := m.Toppings
			if tops == nil {
				tops = []string{}
			}
			src, err := json.Marshal(BFMealSrc{Base: m.Base, Toppings: tops, Snack: m.Snack})
			if err != nil {
				return BFImportPlan{}, err
			}
			res, err := tx.ExecContext(ctx,
				`INSERT INTO bf_meals (day_id, slot, pos, src) VALUES (?, ?, ?, ?)`,
				dayID, m.Slot, i, string(src))
			if err != nil {
				return BFImportPlan{}, err
			}
			id, _ := res.LastInsertId()
			if m.Base != "" {
				if err := bfPutItem(ctx, tx, childID, id, "base", 0, m.Base); err != nil {
					return BFImportPlan{}, err
				}
			}
			for j, t := range tops {
				if err := bfPutItem(ctx, tx, childID, id, "topping", j, t); err != nil {
					return BFImportPlan{}, err
				}
			}
			if m.Snack != nil && *m.Snack != "" {
				if err := bfPutItem(ctx, tx, childID, id, "snack", 0, *m.Snack); err != nil {
					return BFImportPlan{}, err
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return BFImportPlan{}, err
	}
	return plan, nil
}

// BFApplyEdit 은 가져온 직후 손으로 고친 것을 반영한다(시드 원본은 그대로).
func (s *Store) BFApplyEdit(ctx context.Context, childID int64, dday int, slot string, in BFMealInput, userID int64) error {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT m.id FROM bf_meals m JOIN bf_days d ON d.id = m.day_id
		  WHERE d.child_id=? AND d.dday=? AND m.slot=?`, childID, dday, slot).Scan(&id)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = s.BFUpdateMeal(ctx, id, in, userID)
	return err
}
