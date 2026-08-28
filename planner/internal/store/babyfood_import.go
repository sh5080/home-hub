package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 식단 데이터 파일을 읽어 들인다.
//
// 내용은 저장소에 없다 — 파일로 받아서 여기서만 채운다. 파일을 갈아끼우면
// 새 구간을 붙일 수 있고, 코드를 고칠 일은 없다.

// BFPlanFile은 식단 데이터 파일의 최상위 구조다.
type BFPlanFile struct {
	Schema       int             `json:"schema"`
	DefaultTrack []string        `json:"default_track"`
	Plans        []BFPlanSection `json:"plans"`
	Ingredients  []BFPlanIngr    `json:"ingredients"`
}

// BFPlanSection은 한 구간(초기 / 중기 … )이다.
type BFPlanSection struct {
	ID    string      `json:"id"`
	Label string      `json:"label"`
	Kind  string      `json:"kind"` // 'topping' | 'menu'
	From  int         `json:"from"`
	To    int         `json:"to"`
	Days  []BFPlanDay `json:"days"`
}

// BFPlanDay는 하루다. D는 생후 일수.
type BFPlanDay struct {
	D     int          `json:"d"`
	New   string       `json:"new"`
	Meals []BFPlanMeal `json:"meals"`
}

// BFPlanMeal은 한 끼다.
type BFPlanMeal struct {
	Slot     string   `json:"slot"`
	Base     string   `json:"base"`
	Toppings []string `json:"toppings"`
	Snack    *string  `json:"snack"`
}

// BFPlanIngr는 재료 분류다. 'base'/'cube'는 큐브로 세고 'dish'는 그날 만드는
// 요리라 재고 화면에서 따로 묶는다.
type BFPlanIngr struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// BFImportPlan은 가져오기 결과 요약이다. dry-run에서도 같은 값을 만든다.
type BFImportPlan struct {
	Sections []BFImportSection `json:"sections"`
	// Existing은 이미 들어 있어서 건드리지 않는 날 수다. 손으로 고친 내용을
	// 덮어쓰지 않기 위해 기본은 '건너뛴다'.
	Existing    int `json:"existing"`
	NewDays     int `json:"new_days"`
	NewMeals    int `json:"new_meals"`
	Ingredients int `json:"ingredients"`
}

// BFImportSection은 구간별 요약이다.
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

// BFSelectTrack은 쓸 구간들을 골라 D+n 순서로 편다. 같은 D+n을 여러 구간이
// 채우면(중기 두 끼 / 세 끼) 먼저 고른 쪽이 이긴다.
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

// BFImport는 고른 구간을 넣는다. apply가 false면 아무것도 쓰지 않고 요약만
// 돌려준다 — 가져오기는 되돌리기 어려우니 기본을 dry-run으로 둔다.
//
// replace가 false면 **이미 있는 날은 건너뛴다**. 손으로 고쳐둔 식단을
// 덮어쓰지 않기 위해서다.
//
// fromDDay가 0보다 크면 그 날 이후만 다룬다. 중기 두 끼 → 세 끼처럼 도중에
// 구성을 바꿀 때, 이미 지나간 날의 기록까지 건드리지 않기 위한 것이다.
func (s *Store) BFImport(ctx context.Context, f *BFPlanFile, track []string, apply, replace bool, fromDDay int) (BFImportPlan, error) {
	secs, err := BFSelectTrack(f, track)
	if err != nil {
		return BFImportPlan{}, err
	}

	existing := map[int]bool{}
	rows, err := s.db.QueryContext(ctx, `SELECT dday FROM bf_days`)
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

	// 재료 분류를 먼저 넣는다. 이미 있으면 kind만 맞춘다(재고 수치는 건드리지
	// 않는다 — 실사값을 날리면 안 된다).
	for _, ing := range f.Ingredients {
		kind := ing.Kind
		if kind == "" {
			kind = "cube"
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bf_ingredients (name, kind) VALUES (?, ?)
			 ON CONFLICT(name) DO UPDATE SET kind=excluded.kind`, ing.Name, kind); err != nil {
			return BFImportPlan{}, err
		}
	}

	for _, w := range writes {
		if replace {
			// bf_meals / bf_meal_items 는 ON DELETE CASCADE 로 따라 지워진다.
			if _, err := tx.ExecContext(ctx, `DELETE FROM bf_days WHERE dday=?`, w.day.D); err != nil {
				return BFImportPlan{}, err
			}
		}
		var newItem *string
		if w.day.New != "" {
			v := w.day.New
			newItem = &v
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bf_days (dday, stage, label, kind, new_item, new_item_src)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			w.day.D, w.sec.ID, w.sec.Label, w.sec.Kind, newItem, newItem); err != nil {
			return BFImportPlan{}, err
		}
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
				`INSERT INTO bf_meals (dday, slot, pos, src) VALUES (?, ?, ?, ?)`,
				w.day.D, m.Slot, i, string(src))
			if err != nil {
				return BFImportPlan{}, err
			}
			id, _ := res.LastInsertId()
			if m.Base != "" {
				if err := bfPutItem(ctx, tx, id, "base", 0, m.Base); err != nil {
					return BFImportPlan{}, err
				}
			}
			for j, t := range tops {
				if err := bfPutItem(ctx, tx, id, "topping", j, t); err != nil {
					return BFImportPlan{}, err
				}
			}
			if m.Snack != nil && *m.Snack != "" {
				if err := bfPutItem(ctx, tx, id, "snack", 0, *m.Snack); err != nil {
					return BFImportPlan{}, err
				}
			}
		}
	}

	// 프로필 행이 없으면 만들어 둔다. 생일은 넣지 않는다 — 화면에서 받는다.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO bf_profile (id, horizon_days, updated_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO NOTHING`, bfDefaultHorizon, time.Now().Unix()); err != nil {
		return BFImportPlan{}, err
	}
	if err := tx.Commit(); err != nil {
		return BFImportPlan{}, err
	}
	return plan, nil
}

// BFApplyEdit은 가져온 직후 "손으로 고쳐둔 것"을 그대로 반영할 때 쓴다.
// 시드 원본은 그대로 두고 현재 값만 바꾸므로 화면에 "수정됨 · 원래 …"로 뜬다.
func (s *Store) BFApplyEdit(ctx context.Context, dday int, slot string, in BFMealInput, userID int64) error {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM bf_meals WHERE dday=? AND slot=?`, dday, slot).Scan(&id)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = s.BFUpdateMeal(ctx, id, in, userID)
	return err
}
