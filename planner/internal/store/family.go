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

// '함께': 여러 기능의 기록을 가로질러 연속 기록·가족 퀘스트·저금통·성장 마일스톤을 계산한다.
// 경쟁이 아니라 같이 채우는 것이라 점수·순위는 없다. 저장하는 건 주간 보상 문구와 확인한 마일스톤뿐.

type FamilyStreak struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Current   int    `json:"current"`    // 오늘까지 이어진 날(오늘 아직이면 어제까지)
	TodayDone bool   `json:"today_done"` //
	MonthDone int    `json:"month_done"` // 이번 달 해낸 날
	MonthDays int    `json:"month_days"` // 이번 달 해당하는 날(오늘까지)
	URL       string `json:"url"`
}

type FamilyQuest struct {
	Key    string           `json:"key"`
	Label  string           `json:"label"`
	Help   string           `json:"help"`
	Value  int64            `json:"value"`
	Target int64            `json:"target"`
	Done   bool             `json:"done"`
	Unit   string           `json:"unit"`
	By     map[string]int64 `json:"by"` // 누가 채웠는지(이름 → 몇). 순위가 아니라 고마움 표시용
	URL    string           `json:"url"`
}

type FamilyJarMonth struct {
	Month string `json:"month"`
	Saved int64  `json:"saved"` // 쓸 수 있는 돈 - 평소 지출(음수면 넘김)
}

type FamilyJar struct {
	Spendable  int64            `json:"spendable"`
	MonthLeft  int64            `json:"month_left"` // 이번 달 남은 돈
	Pace       int64            `json:"pace"`       // 이 속도면 월말까지 쓸 돈
	Incentive  int64            `json:"incentive"`  // 이번 달 지역화폐 적립(보너스)
	Total      int64            `json:"total"`      // 저금통 시작 달부터 끝난 달들의 합
	From       string           `json:"from"`
	Months     []FamilyJarMonth `json:"months"`
	GoalName   string           `json:"goal_name"`
	GoalTarget int64            `json:"goal_target"`
}

type FamilyMilestone struct {
	Key    string `json:"key"`
	Group  string `json:"group"`
	Label  string `json:"label"`
	Value  int64  `json:"value"`  // 지금 값
	Target int64  `json:"target"` // 이 마일스톤의 기준
	Done   bool   `json:"done"`
	At     string `json:"at"`  // 달성한 날(YYYY-MM-DD)
	New    bool   `json:"new"` // 아직 확인 안 함
	URL    string `json:"url"`
}

// 식물 단계별 필요한 물방울(누적). 5단계가 끝.
var FamilyStages = []struct {
	Name  string `json:"name"`
	Drops int64  `json:"drops"`
}{{"씨앗", 0}, {"새싹", 30}, {"잎새", 80}, {"꽃봉오리", 150}, {"활짝", 250}}

type FamilyDrop struct {
	Day    string `json:"day"`
	Label  string `json:"label"`
	Amount int64  `json:"amount"`
}

type FamilyPlant struct {
	ID      int64        `json:"id"`
	Name    string       `json:"name"`
	Started string       `json:"started"`
	Drops   int64        `json:"drops"`
	Stage   int          `json:"stage"` // 1~5
	Next    int64        `json:"next"`  // 다음 단계까지 필요한 누적(5단계면 0)
	Today   int64        `json:"today"` // 오늘 얻은 물방울
	Recent  []FamilyDrop `json:"recent"`
	Ready   bool         `json:"ready"`   // 5단계 — 다 키웠다. '다음 식물'을 누르면 구매권이 된다
	Count   int          `json:"count"`   // 지금까지 다 키운 식물 수
	Credits int          `json:"credits"` // 아직 안 쓴 구매권(다 키운 식물 중 위시를 안 산 것)
}

type FamilyWish struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Price     int64  `json:"price"`
	Note      string `json:"note"`
	CreatedBy *int64 `json:"created_by"`
	BoughtAt  *int64 `json:"bought_at"`
}

type FamilyBoard struct {
	Plant      FamilyPlant       `json:"plant"`
	Stages     any               `json:"stages"`
	Wishes     []FamilyWish      `json:"wishes"`
	Week       string            `json:"week"` // 이번 주 월요일
	WeekEnd    string            `json:"week_end"`
	Reward     string            `json:"reward"`
	Quests     []FamilyQuest     `json:"quests"`
	Streaks    []FamilyStreak    `json:"streaks"`
	Jar        *FamilyJar        `json:"jar"`
	Milestones []FamilyMilestone `json:"milestones"` // 달성한 것 + 그룹마다 다음 하나
	Together   map[string]int64  `json:"together"`   // 이번 주 함께한 기록 수(이름 → 수)
}

func mondayOf(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

// daySet 은 'YYYY-MM-DD' → true.
func (s *Store) daySet(ctx context.Context, q string, args ...any) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		if len(d) >= 10 {
			out[d[:10]] = true
		}
	}
	return out, rows.Err()
}

// streakOf: applies 가 false 인 날은 건너뛴다(끊지 않는다). 오늘을 아직 못 했으면 어제까지로 센다.
func streakOf(name, label, url string, today time.Time, done func(string) bool, applies func(time.Time) bool) FamilyStreak {
	st := FamilyStreak{Key: name, Label: label, URL: url}
	ds := func(t time.Time) string { return t.Format("2006-01-02") }
	st.TodayDone = applies(today) && done(ds(today))
	d := today
	if !st.TodayDone {
		d = d.AddDate(0, 0, -1)
	}
	for i := 0; i < 400; i++ {
		if applies(d) {
			if !done(ds(d)) {
				break
			}
			st.Current++
		}
		d = d.AddDate(0, 0, -1)
	}
	for d := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location()); !d.After(today); d = d.AddDate(0, 0, 1) {
		if applies(d) {
			st.MonthDays++
			if done(ds(d)) {
				st.MonthDone++
			}
		}
	}
	return st
}

type familyRoutine struct {
	id      int64
	mask    int
	created string
}

func (s *Store) familyRoutines(ctx context.Context) ([]familyRoutine, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, weekdays_mask, created_at FROM routines WHERE active = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []familyRoutine
	for rows.Next() {
		var r familyRoutine
		var c int64
		if err := rows.Scan(&r.id, &r.mask, &c); err != nil {
			return nil, err
		}
		r.created = time.Unix(c, 0).Format("2006-01-02")
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Family(ctx context.Context, now time.Time) (FamilyBoard, error) {
	// 목록은 비어도 null 이 아니라 [] 로 준다(화면이 바로 .map 한다).
	b := FamilyBoard{Quests: []FamilyQuest{}, Streaks: []FamilyStreak{}, Milestones: []FamilyMilestone{}, Wishes: []FamilyWish{}, Together: map[string]int64{}}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	mon := mondayOf(now)
	b.Week, b.WeekEnd = mon.Format("2006-01-02"), mon.AddDate(0, 0, 6).Format("2006-01-02")
	weekFrom, weekTo := b.Week, mon.AddDate(0, 0, 7).Format("2006-01-02") // to 는 배타
	since := today.AddDate(0, 0, -400).Format("2006-01-02")

	users, err := s.ListUsers(ctx)
	if err != nil {
		return b, err
	}
	name := map[int64]string{}
	for _, u := range users {
		name[u.ID] = u.Name
	}
	children, err := s.BFChildren(ctx)
	if err != nil {
		return b, err
	}

	// --- 기록 모으기(커넥션 하나라 읽기를 먼저 다 한다) ---
	routines, err := s.familyRoutines(ctx)
	if err != nil {
		return b, err
	}
	checks := map[string]map[int64]bool{} // date → routine
	checkBy := map[string]int64{}
	{
		rows, err := s.db.QueryContext(ctx, `SELECT routine_id, date, checked_by FROM routine_checks WHERE date >= ?`, since)
		if err != nil {
			return b, err
		}
		for rows.Next() {
			var id, by int64
			var d string
			if err := rows.Scan(&id, &d, &by); err != nil {
				rows.Close()
				return b, err
			}
			if checks[d] == nil {
				checks[d] = map[int64]bool{}
			}
			checks[d][id] = true
			if d >= weekFrom && d < weekTo {
				checkBy[name[by]]++
			}
		}
		rows.Close()
	}
	diaryDays, err := s.daySet(ctx, `SELECT date FROM diary WHERE date >= ?`, since)
	if err != nil {
		return b, err
	}
	careDays, err := s.daySet(ctx, `SELECT at FROM care_logs WHERE at >= ?`, since)
	if err != nil {
		return b, err
	}
	together := map[string]int64{}
	countBy := func(q string, args ...any) (map[string]int64, int64, error) {
		rows, err := s.db.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, 0, err
		}
		defer rows.Close()
		out, total := map[string]int64{}, int64(0)
		for rows.Next() {
			var by sql.NullInt64
			var n int64
			if err := rows.Scan(&by, &n); err != nil {
				return nil, 0, err
			}
			if by.Valid && name[by.Int64] != "" {
				out[name[by.Int64]] += n
			}
			total += n
		}
		return out, total, rows.Err()
	}
	diaryBy, diaryWeek, err := countBy(`SELECT created_by, count(*) FROM diary WHERE date >= ? AND date < ? GROUP BY created_by`, weekFrom, weekTo)
	if err != nil {
		return b, err
	}
	careBy, _, err := countBy(`SELECT created_by, count(*) FROM care_logs WHERE at >= ? AND at < ? GROUP BY created_by`, weekFrom, weekTo)
	if err != nil {
		return b, err
	}
	monUnix, nextUnix := mon.Unix(), mon.AddDate(0, 0, 7).Unix()
	taskBy, taskWeek, err := countBy(`SELECT coalesce(assignee_id, created_by), count(*) FROM cards WHERE done_at >= ? AND done_at < ? GROUP BY 1`, monUnix, nextUnix)
	if err != nil {
		return b, err
	}
	// 이번 주 처음 먹어본 이유식 재료.
	newFoods := map[string]bool{}
	{
		rows, err := s.db.QueryContext(ctx, `SELECT i.name, min(l.at) FROM care_log_items x JOIN care_logs l ON l.id = x.log_id
			JOIN bf_ingredients i ON i.id = x.ingredient_id GROUP BY x.ingredient_id`)
		if err != nil {
			return b, err
		}
		for rows.Next() {
			var n, first string
			if err := rows.Scan(&n, &first); err != nil {
				rows.Close()
				return b, err
			}
			if first >= weekFrom && first < weekTo {
				newFoods[n] = true
			}
		}
		rows.Close()
	}
	for _, m := range []map[string]int64{checkBy, diaryBy, careBy, taskBy} {
		for k, v := range m {
			together[k] += v
		}
	}
	b.Together = together
	if err := s.db.QueryRowContext(ctx, `SELECT reward FROM family_weeks WHERE week = ?`, b.Week).Scan(&b.Reward); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return b, err
	}

	// 재정(올린 파일이 있을 때만). 이번 달 기준.
	var fin *FinOverview
	thisMonth := now.Format("2006-01")
	if ov, err := s.financeOverview(ctx, 0, thisMonth, now, true); err == nil && len(ov.Owners) > 0 {
		fin = &ov
	} else if err != nil {
		return b, err
	}

	// --- 연속 기록 ---
	type streakDef struct {
		key, label string
		done       func(string) bool
		applies    func(time.Time) bool
	}
	var defs []streakDef
	mk := func(key, label, url string, done func(string) bool, applies func(time.Time) bool) FamilyStreak {
		defs = append(defs, streakDef{key, label, done, applies})
		return streakOf(key, label, url, today, done, applies)
	}
	scheduled := func(d time.Time) []familyRoutine {
		bit := 1 << ((int(d.Weekday()) + 6) % 7)
		ds := d.Format("2006-01-02")
		var out []familyRoutine
		for _, r := range routines {
			if r.mask&bit != 0 && r.created <= ds {
				out = append(out, r)
			}
		}
		return out
	}
	if len(routines) > 0 {
		b.Streaks = append(b.Streaks, mk("routines", "루틴 다 하기", "/routines",
			func(ds string) bool {
				d, _ := time.ParseInLocation("2006-01-02", ds, today.Location())
				for _, r := range scheduled(d) {
					if !checks[ds][r.id] {
						return false
					}
				}
				return true
			},
			func(d time.Time) bool { return len(scheduled(d)) > 0 }))
	}
	always := func(time.Time) bool { return true }
	b.Streaks = append(b.Streaks, mk("diary", "일기 쓰기", "/diary", func(ds string) bool { return diaryDays[ds] }, always))
	if len(children) > 0 {
		b.Streaks = append(b.Streaks, mk("care", "육아 기록", "/care", func(ds string) bool { return careDays[ds] }, always))
	}
	if fin != nil && fin.Plan.Spendable > 0 {
		// 그날까지 평소 지출 누계가 '쓸 수 있는 돈'의 그날까지 몫 안이면 지킨 날. 이번 달 안에서만 센다.
		dim := fin.Plan.DaysInMonth
		cum := map[int]int64{}
		for _, r := range fin.Regular {
			if r.At[:7] != thisMonth {
				continue
			}
			if d := atoiDefault2(r.At[8:10]); d > 0 {
				cum[d] += r.Amount
			}
		}
		running, ok := int64(0), map[string]bool{}
		for d := 1; d <= today.Day(); d++ {
			running += cum[d]
			if running <= fin.Plan.Spendable*int64(d)/int64(dim) {
				ok[time.Date(today.Year(), today.Month(), d, 0, 0, 0, 0, today.Location()).Format("2006-01-02")] = true
			}
		}
		monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
		st := mk("budget", "생활비 페이스 지키기", "/finance", func(ds string) bool { return ok[ds] },
			func(d time.Time) bool { return !d.Before(monthStart) })
		b.Streaks = append(b.Streaks, st)
	}

	// --- 가족 퀘스트(주마다 자동, 다 같이 채운다) ---
	var pool []FamilyQuest
	weekDays := 0 // 이번 주 지난 날(오늘 포함)
	for d := mon; !d.After(today); d = d.AddDate(0, 0, 1) {
		weekDays++
	}
	if len(routines) > 0 {
		var sched, done int64
		for d := mon; d.Before(mon.AddDate(0, 0, 7)); d = d.AddDate(0, 0, 1) {
			ds := d.Format("2006-01-02")
			for _, r := range scheduled(d) {
				sched++
				if checks[ds][r.id] {
					done++
				}
			}
		}
		target := (sched*8 + 9) / 10
		pool = append(pool, FamilyQuest{Key: "routines", Label: "이번 주 루틴 80% 채우기", Help: fmt.Sprintf("이번 주 해야 할 루틴 %d번 중 %d번", sched, target),
			Value: done, Target: max(target, 1), Unit: "번", By: checkBy, URL: "/routines"})
	}
	pool = append(pool, FamilyQuest{Key: "diary", Label: "일기 3편 남기기", Help: "누가 쓰든 이번 주 일기 3편",
		Value: diaryWeek, Target: 3, Unit: "편", By: diaryBy, URL: "/diary"})
	pool = append(pool, FamilyQuest{Key: "tasks", Label: "할 일 5개 끝내기", Help: "이번 주 완료 칸으로 옮긴 카드",
		Value: taskWeek, Target: 5, Unit: "개", By: taskBy, URL: "/boards"})
	if len(children) > 0 {
		var names []string
		for n := range newFoods {
			names = append(names, n)
		}
		sort.Strings(names)
		help := children[0].Name + "가 이번 주 처음 먹어본 재료"
		if len(names) > 0 {
			help += ": " + strings.Join(names, ", ")
		}
		pool = append(pool, FamilyQuest{Key: "food", Label: "새 이유식 재료 1개 먹이기", Help: help,
			Value: int64(len(newFoods)), Target: 1, Unit: "개", URL: "/babyfood"})
	}
	if fin != nil && fin.Plan.Spendable > 0 {
		var spent int64
		for _, r := range fin.Regular {
			if r.At[:10] >= weekFrom && r.At[:10] < weekTo {
				spent += r.Amount
			}
		}
		share := fin.Plan.Spendable * 7 / int64(fin.Plan.DaysInMonth)
		q := FamilyQuest{Key: "budget", Label: "이번 주 생활비 " + won(share) + " 안에서", Help: "평소 지출(지역화폐 포함, 예상 외 제외) — 주말까지 넘지 않으면 성공",
			Value: spent, Target: share, Unit: "원", URL: "/finance"}
		// 이번 주 끝까지의 파일이 올라와 있어야 판정한다(안 올렸으면 0원이라 저절로 성공이 된다).
		covered := true
		for _, at := range fin.AsOf {
			if at < b.WeekEnd {
				covered = false
			}
		}
		q.Done = spent <= share && weekDays == 7 && covered
		if !covered {
			q.Help = "이번 주까지의 자산 파일을 올리면 확인돼요 — 평소 지출(지역화폐 포함, 예상 외 제외)"
		}
		pool = append(pool, q)
		var untagged int64
		for _, r := range fin.Regular {
			if r.At[:7] == thisMonth && len(fin.TxTags[r.Ref]) == 0 && r.Amount >= finUntaggedMin {
				untagged++
			}
		}
		pool = append(pool, FamilyQuest{Key: "tags", Label: "이번 달 지출 분류 끝내기", Help: fmt.Sprintf("평소 지출 중 태그 없는 %d건(1,000원 이상)을 0건으로", untagged),
			Value: untagged, Target: 0, Unit: "건", URL: "/finance"})
	}
	for i := range pool {
		q := &pool[i]
		switch q.Key {
		case "budget":
		case "tags":
			q.Done = q.Value == 0
		default:
			q.Done = q.Value >= q.Target
		}
	}
	// 주마다 3개를 돌려가며 고른다. 같은 주엔 늘 같은 셋.
	_, wk := mon.ISOWeek()
	n := min(3, len(pool))
	for i := 0; i < n; i++ {
		b.Quests = append(b.Quests, pool[(wk+i*2)%len(pool)])
	}
	b.Quests = dedupQuests(b.Quests, pool, n)

	// --- 저금통 ---
	if fin != nil && fin.Plan.Spendable > 0 {
		from, err := s.familyJarFrom(ctx, now)
		if err != nil {
			return b, err
		}
		jar := &FamilyJar{Months: []FamilyJarMonth{}, Spendable: fin.Plan.Spendable, MonthLeft: fin.Plan.Spendable - fin.Plan.MonthSpent, From: from}
		if fin.Plan.DaysPassed > 0 {
			jar.Pace = fin.Plan.MonthSpent * int64(fin.Plan.DaysInMonth) / int64(fin.Plan.DaysPassed)
		}
		for _, w := range fin.Wallets {
			jar.Incentive += w.Earned
		}
		for _, m := range fin.Months {
			if m.Partial || m.Month < from {
				continue
			}
			saved := fin.Plan.Spendable - (m.Variable - m.Unexpected)
			jar.Months = append(jar.Months, FamilyJarMonth{Month: m.Month, Saved: saved})
			jar.Total += saved
		}
		if gid, ok, _ := s.KVGet(ctx, "family.jar_goal"); ok {
			for _, g := range fin.Goals {
				if fmt.Sprint(g.ID) == gid {
					jar.GoalName, jar.GoalTarget = g.Name, g.Target
				}
			}
		}
		b.Jar = jar
	}

	// --- 성장 마일스톤 ---
	ms, err := s.familyMilestones(ctx, children, today)
	if err != nil {
		return b, err
	}
	b.Milestones = ms

	// --- 식물: 물방울 주기(같은 key 는 한 번만) → 지금 식물의 단계 ---
	plant, err := s.familyPlant(ctx, today)
	if err != nil {
		return b, err
	}
	type award struct {
		key, day, label string
		amount          int64
	}
	var awards []award
	start, _ := time.ParseInLocation("2006-01-02", plant.Started, today.Location())
	for _, d := range defs {
		for day := start; !day.After(today); day = day.AddDate(0, 0, 1) {
			ds := day.Format("2006-01-02")
			if d.applies(day) && d.done(ds) {
				awards = append(awards, award{"streak:" + d.key + ":" + ds, ds, d.label, 1})
			}
		}
	}
	allDone := len(b.Quests) > 0
	for _, q := range b.Quests {
		if q.Done {
			awards = append(awards, award{"quest:" + b.Week + ":" + q.Key, today.Format("2006-01-02"), "퀘스트 · " + q.Label, 3})
		} else {
			allDone = false
		}
	}
	if allDone {
		awards = append(awards, award{"quests:" + b.Week, today.Format("2006-01-02"), "이번 주 퀘스트 모두 완료!", 5})
	}
	for _, m := range ms {
		if m.Done && m.At >= plant.Started {
			awards = append(awards, award{"ms:" + m.Key, m.At, "마일스톤 · " + m.Label, 5})
		}
	}
	for _, a := range awards {
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO family_drops (key, amount, day, label) VALUES (?, ?, ?, ?)`, a.key, a.amount, a.day, a.label); err != nil {
			return b, err
		}
	}
	if err := s.fillPlant(ctx, &plant, today); err != nil {
		return b, err
	}
	b.Plant, b.Stages = plant, FamilyStages
	if b.Wishes, err = s.FamilyWishes(ctx); err != nil {
		return b, err
	}
	return b, nil
}

// familyPlant 는 지금 키우는 식물(없으면 오늘 심는다).
func (s *Store) familyPlant(ctx context.Context, today time.Time) (FamilyPlant, error) {
	var p FamilyPlant
	err := s.db.QueryRowContext(ctx, `SELECT id, name, started FROM family_plants WHERE finished_at IS NULL ORDER BY id DESC LIMIT 1`).Scan(&p.ID, &p.Name, &p.Started)
	if errors.Is(err, sql.ErrNoRows) {
		p.Started = today.Format("2006-01-02")
		res, err := s.db.ExecContext(ctx, `INSERT INTO family_plants (started) VALUES (?)`, p.Started)
		if err != nil {
			return p, err
		}
		p.ID, _ = res.LastInsertId()
		return p, nil
	}
	return p, err
}

func (s *Store) fillPlant(ctx context.Context, p *FamilyPlant, today time.Time) error {
	ds := today.Format("2006-01-02")
	if err := s.db.QueryRowContext(ctx, `SELECT coalesce(sum(amount), 0), coalesce(sum(CASE WHEN day = ? THEN amount END), 0) FROM family_drops WHERE day >= ?`,
		ds, p.Started).Scan(&p.Drops, &p.Today); err != nil {
		return err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*), count(*) - (SELECT count(*) FROM family_wishes WHERE plant_id IS NOT NULL) FROM family_plants WHERE finished_at IS NOT NULL`).Scan(&p.Count, &p.Credits); err != nil {
		return err
	}
	p.Stage = 1
	for i, st := range FamilyStages {
		if p.Drops >= st.Drops {
			p.Stage = i + 1
		}
	}
	if p.Stage < len(FamilyStages) {
		p.Next = FamilyStages[p.Stage].Drops
	}
	p.Ready = p.Stage == len(FamilyStages)
	rows, err := s.db.QueryContext(ctx, `SELECT day, label, amount FROM family_drops WHERE day >= ? ORDER BY day DESC, rowid DESC LIMIT 20`, p.Started)
	if err != nil {
		return err
	}
	defer rows.Close()
	p.Recent = []FamilyDrop{}
	for rows.Next() {
		var d FamilyDrop
		if err := rows.Scan(&d.Day, &d.Label, &d.Amount); err != nil {
			return err
		}
		p.Recent = append(p.Recent, d)
	}
	return rows.Err()
}

// FamilyGrowNext 는 다 자란 식물을 마치고(구매권 하나) 새 씨앗을 심는다. 이름은 이어받는다.
func (s *Store) FamilyGrowNext(ctx context.Context, now time.Time) error {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	p, err := s.familyPlant(ctx, today)
	if err != nil {
		return err
	}
	if err := s.fillPlant(ctx, &p, today); err != nil {
		return err
	}
	if !p.Ready {
		return invalid("아직 다 자라지 않았어요")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE family_plants SET finished_at=? WHERE id=?`, now.Unix(), p.ID); err != nil {
		return err
	}
	// 새 씨앗은 내일부터 센다 — 오늘 이미 받은 물방울로 바로 자라지 않게.
	if _, err := tx.ExecContext(ctx, `INSERT INTO family_plants (started, name) VALUES (?, ?)`, today.AddDate(0, 0, 1).Format("2006-01-02"), p.Name); err != nil {
		return err
	}
	return tx.Commit()
}

// FamilyBuyWish 는 구매권(다 키운 식물) 하나로 위시를 산 것으로 표시한다.
func (s *Store) FamilyBuyWish(ctx context.Context, wishID int64, now time.Time) error {
	var bought sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT bought_at FROM family_wishes WHERE id=?`, wishID).Scan(&bought); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if bought.Valid {
		return invalid("이미 산 거예요")
	}
	var plant int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM family_plants WHERE finished_at IS NOT NULL
		AND id NOT IN (SELECT plant_id FROM family_wishes WHERE plant_id IS NOT NULL) ORDER BY id LIMIT 1`).Scan(&plant)
	if errors.Is(err, sql.ErrNoRows) {
		return invalid("구매권이 없어요 — 식물을 다 키우면 생겨요")
	}
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE family_wishes SET bought_at=?, plant_id=? WHERE id=?`, now.Unix(), plant, wishID)
	return err
}

// FamilyRenamePlant 는 지금 키우는 식물의 이름을 바꾼다.
func (s *Store) FamilyRenamePlant(ctx context.Context, name string, now time.Time) error {
	name = strings.TrimSpace(name)
	if len([]rune(name)) > 12 {
		return invalid("이름은 12자까지예요")
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	p, err := s.familyPlant(ctx, today)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE family_plants SET name=? WHERE id=?`, name, p.ID)
	return err
}

func (s *Store) FamilyWishes(ctx context.Context) ([]FamilyWish, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, title, price, note, created_by, bought_at FROM family_wishes ORDER BY bought_at IS NOT NULL, bought_at DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FamilyWish{}
	for rows.Next() {
		var w FamilyWish
		if err := rows.Scan(&w.ID, &w.Title, &w.Price, &w.Note, &w.CreatedBy, &w.BoughtAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func familyWishInput(title, note string, price int64) (string, string, error) {
	title, note = strings.TrimSpace(title), strings.TrimSpace(note)
	if title == "" || len([]rune(title)) > 60 {
		return "", "", invalid("이름은 1~60자예요")
	}
	if len([]rune(note)) > 200 || price < 0 {
		return "", "", invalid("메모는 200자, 가격은 0 이상이에요")
	}
	return title, note, nil
}

func (s *Store) FamilySaveWish(ctx context.Context, id int64, title, note string, price, by int64) (int64, error) {
	title, note, err := familyWishInput(title, note, price)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO family_wishes (title, price, note, created_by, created_at) VALUES (?, ?, ?, ?, ?)`, title, price, note, by, time.Now().Unix())
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	res, err := s.db.ExecContext(ctx, `UPDATE family_wishes SET title=?, price=?, note=? WHERE id=?`, title, price, note, id)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrNotFound
	}
	return id, nil
}

func (s *Store) FamilyDeleteWish(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM family_wishes WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func dedupQuests(qs, pool []FamilyQuest, n int) []FamilyQuest {
	seen := map[string]bool{}
	var out []FamilyQuest
	for _, q := range qs {
		if !seen[q.Key] {
			seen[q.Key] = true
			out = append(out, q)
		}
	}
	for _, q := range pool {
		if len(out) >= n {
			break
		}
		if !seen[q.Key] {
			seen[q.Key] = true
			out = append(out, q)
		}
	}
	return out
}

func atoiDefault2(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// 저금통은 처음 본 달부터 센다(그 전 달들은 기준 없이 쓴 돈이라 넣지 않는다).
func (s *Store) familyJarFrom(ctx context.Context, now time.Time) (string, error) {
	if v, ok, err := s.KVGet(ctx, "family.jar_from"); err != nil || ok {
		return v, err
	}
	m := now.Format("2006-01")
	return m, s.KVSet(ctx, "family.jar_from", m)
}

func (s *Store) FamilySetReward(ctx context.Context, week, reward string, by int64) error {
	if _, err := time.Parse("2006-01-02", week); err != nil {
		return invalid("주가 올바르지 않아요")
	}
	reward = strings.TrimSpace(reward)
	if len([]rune(reward)) > 60 {
		return invalid("보상은 60자까지예요")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO family_weeks (week, reward, updated_by, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(week) DO UPDATE SET reward=excluded.reward, updated_by=excluded.updated_by, updated_at=excluded.updated_at`,
		week, reward, by, time.Now().Unix())
	return err
}

func (s *Store) FamilySetJarGoal(ctx context.Context, goalID int64) error {
	if goalID == 0 {
		_, err := s.db.ExecContext(ctx, `DELETE FROM kv WHERE key='family.jar_goal'`)
		return err
	}
	return s.KVSet(ctx, "family.jar_goal", fmt.Sprint(goalID))
}

// --- 마일스톤 ---

type msSpec struct {
	group, unit, url string
	label            func(n int64) string
	steps            []int64
	// nth 는 n번째 기록이 생긴 날(없으면 "")과 지금 개수.
	count func(ctx context.Context) (int64, error)
	nth   func(ctx context.Context, n int64) (string, error)
}

func (s *Store) nthDay(q string) func(ctx context.Context, n int64) (string, error) {
	return func(ctx context.Context, n int64) (string, error) {
		var d sql.NullString
		err := s.db.QueryRowContext(ctx, q+` LIMIT 1 OFFSET ?`, n-1).Scan(&d)
		if errors.Is(err, sql.ErrNoRows) || !d.Valid {
			return "", nil
		}
		if len(d.String) >= 10 {
			return d.String[:10], err
		}
		return d.String, err
	}
}

func (s *Store) countQ(q string) func(ctx context.Context) (int64, error) {
	return func(ctx context.Context) (int64, error) {
		var n int64
		return n, s.db.QueryRowContext(ctx, q).Scan(&n)
	}
}

func (s *Store) familyMilestones(ctx context.Context, children []BFChild, today time.Time) ([]FamilyMilestone, error) {
	seen := map[string]bool{}
	if v, ok, err := s.KVGet(ctx, "family.seen"); err != nil {
		return nil, err
	} else if ok {
		var keys []string
		_ = json.Unmarshal([]byte(v), &keys)
		for _, k := range keys {
			seen[k] = true
		}
	}
	var out []FamilyMilestone
	add := func(key, group, label, url string, value, target int64, at string) {
		m := FamilyMilestone{Key: key, Group: group, Label: label, Value: value, Target: target, Done: value >= target, At: at, URL: url}
		m.New = m.Done && !seen[key]
		out = append(out, m)
	}
	// 단아 생후 날짜: 100일, 200일, 300일, 1년, 500일, 2년…
	for _, c := range children {
		birth, err := time.ParseInLocation("2006-01-02", c.BirthDate, today.Location())
		if err != nil {
			continue
		}
		age := int64(today.Sub(birth).Hours()/24) + 1 // 태어난 날이 1일
		days := []int64{50, 100, 200, 300, 365, 500, 730, 1000, 1095}
		next := false
		for _, d := range days {
			label := fmt.Sprintf("%s 생후 %d일", c.Name, d)
			switch d {
			case 365:
				label = c.Name + " 첫 돌"
			case 730:
				label = c.Name + " 두 돌"
			case 1095:
				label = c.Name + " 세 돌"
			}
			if age >= d {
				add(fmt.Sprintf("age:%d:%d", c.UserID, d), "성장", label, "/diary", age, d, birth.AddDate(0, 0, int(d-1)).Format("2006-01-02"))
			} else if !next {
				next = true
				add(fmt.Sprintf("age:%d:%d", c.UserID, d), "성장", label, "/diary", age, d, birth.AddDate(0, 0, int(d-1)).Format("2006-01-02"))
			}
		}
	}
	specs := []msSpec{
		{group: "이유식", url: "/babyfood", steps: []int64{10, 20, 30, 50, 70, 100},
			label: func(n int64) string { return fmt.Sprintf("이유식 재료 %d가지", n) },
			count: s.countQ(`SELECT count(DISTINCT ingredient_id) FROM care_log_items`),
			nth:   s.nthDay(`SELECT min(l.at) AS first FROM care_log_items x JOIN care_logs l ON l.id = x.log_id GROUP BY x.ingredient_id ORDER BY first`)},
		{group: "육아 기록", url: "/care", steps: []int64{500, 1000, 2000, 3000, 5000, 10000},
			label: func(n int64) string { return fmt.Sprintf("육아 기록 %d번", n) },
			count: s.countQ(`SELECT count(*) FROM care_logs`),
			nth:   s.nthDay(`SELECT at FROM care_logs ORDER BY at`)},
		{group: "다이어리", url: "/diary", steps: []int64{10, 50, 100, 200, 365, 500},
			label: func(n int64) string { return fmt.Sprintf("일기 %d편", n) },
			count: s.countQ(`SELECT count(*) FROM diary`),
			nth:   s.nthDay(`SELECT date FROM diary ORDER BY date`)},
		{group: "할 일", url: "/boards", steps: []int64{10, 50, 100, 300, 500, 1000},
			label: func(n int64) string { return fmt.Sprintf("할 일 %d개 끝냄", n) },
			count: s.countQ(`SELECT count(*) FROM cards WHERE done_at IS NOT NULL`),
			nth:   s.nthDay(`SELECT date(done_at, 'unixepoch', 'localtime') FROM cards WHERE done_at IS NOT NULL ORDER BY done_at`)},
		{group: "루틴", url: "/routines", steps: []int64{50, 100, 300, 500, 1000, 2000},
			label: func(n int64) string { return fmt.Sprintf("루틴 %d번 체크", n) },
			count: s.countQ(`SELECT count(*) FROM routine_checks`),
			nth:   s.nthDay(`SELECT date FROM routine_checks ORDER BY date`)},
	}
	if len(children) == 0 {
		specs = specs[2:]
	}
	for _, sp := range specs {
		n, err := sp.count(ctx)
		if err != nil {
			return nil, err
		}
		for _, step := range sp.steps {
			key := fmt.Sprintf("%s:%d", sp.group, step)
			if n >= step {
				at, err := sp.nth(ctx, step)
				if err != nil {
					return nil, err
				}
				add(key, sp.group, sp.label(step), sp.url, n, step, at)
				continue
			}
			add(key, sp.group, sp.label(step), sp.url, n, step, "")
			break
		}
	}
	// 처음 켤 때 이미 이룬 것들이 한꺼번에 '새로'로 뜨지 않게, 확인 기록이 없으면 다 본 걸로 둔다.
	if _, ok, _ := s.KVGet(ctx, "family.seen"); !ok {
		var keys []string
		for i := range out {
			if out[i].Done {
				keys = append(keys, out[i].Key)
				out[i].New = false
			}
		}
		raw, _ := json.Marshal(keys)
		if err := s.KVSet(ctx, "family.seen", string(raw)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// FamilySeen 은 마일스톤들을 확인했다고 적는다.
func (s *Store) FamilySeen(ctx context.Context, keys []string) error {
	cur := map[string]bool{}
	if v, ok, err := s.KVGet(ctx, "family.seen"); err != nil {
		return err
	} else if ok {
		var ks []string
		_ = json.Unmarshal([]byte(v), &ks)
		for _, k := range ks {
			cur[k] = true
		}
	}
	for _, k := range keys {
		cur[k] = true
	}
	var all []string
	for k := range cur {
		all = append(all, k)
	}
	sort.Strings(all)
	raw, _ := json.Marshal(all)
	return s.KVSet(ctx, "family.seen", string(raw))
}

// FamilyMonthReport 는 한 달('YYYY-MM') 동안 함께 쌓은 것을 한 줄로 정리한다(월간 리포트 알림).
func (s *Store) FamilyMonthReport(ctx context.Context, month string) (string, error) {
	from := month + "-01"
	t, err := time.Parse("2006-01-02", from)
	if err != nil {
		return "", invalid("달이 올바르지 않아요")
	}
	to := t.AddDate(0, 1, 0).Format("2006-01-02")
	var diary, care, checks, tasks int64
	for _, q := range []struct {
		dst  *int64
		sql  string
		args []any
	}{
		{&diary, `SELECT count(*) FROM diary WHERE date >= ? AND date < ?`, []any{from, to}},
		{&care, `SELECT count(*) FROM care_logs WHERE at >= ? AND at < ?`, []any{from, to}},
		{&checks, `SELECT count(*) FROM routine_checks WHERE date >= ? AND date < ?`, []any{from, to}},
		{&tasks, `SELECT count(*) FROM cards WHERE done_at >= ? AND done_at < ?`, []any{t.Unix(), t.AddDate(0, 1, 0).Unix()}},
	} {
		if err := s.db.QueryRowContext(ctx, q.sql, q.args...).Scan(q.dst); err != nil {
			return "", err
		}
	}
	parts := []string{}
	if diary > 0 {
		parts = append(parts, fmt.Sprintf("일기 %d편", diary))
	}
	if care > 0 {
		parts = append(parts, fmt.Sprintf("육아 기록 %d번", care))
	}
	if checks > 0 {
		parts = append(parts, fmt.Sprintf("루틴 %d번", checks))
	}
	if tasks > 0 {
		parts = append(parts, fmt.Sprintf("할 일 %d개", tasks))
	}
	now := t.AddDate(0, 1, 0)
	if ov, err := s.FinanceOverview(ctx, 0, month, now); err == nil && ov.Plan.Spendable > 0 {
		for _, m := range ov.Months {
			if m.Month == month {
				saved := ov.Plan.Spendable - (m.Variable - m.Unexpected)
				if saved >= 0 {
					parts = append(parts, "저금통 +"+won(saved))
				} else {
					parts = append(parts, "생활비 "+won(-saved)+" 넘김")
				}
			}
		}
	}
	if b, err := s.familyMilestones(ctx, mustChildren(s, ctx), now.AddDate(0, 0, -1)); err == nil {
		n := 0
		for _, m := range b {
			if m.Done && strings.HasPrefix(m.At, month) {
				n++
			}
		}
		if n > 0 {
			parts = append(parts, fmt.Sprintf("마일스톤 %d개", n))
		}
	}
	if len(parts) == 0 {
		return "지난달은 조용했어요. 이번 달 가족 퀘스트를 확인해보세요.", nil
	}
	return strings.Join(parts, " · ") + " — 함께 해냈어요.", nil
}

func mustChildren(s *Store, ctx context.Context) []BFChild {
	cs, _ := s.BFChildren(ctx)
	return cs
}
