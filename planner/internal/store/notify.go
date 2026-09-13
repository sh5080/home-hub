package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// 알림 규칙과 판정. 실제 전송은 notify 패키지.

// 알림 규칙은 "무엇을(Kind=대상) + 어떤 조건에(Params.Cond)"의 조합이다. 대상마다 쓸 수 있는 조건이 다르다.
//   pending    남아 있으면(오늘까지 할 일·오늘 루틴)
//   none_today 오늘 아직 없으면
//   since      마지막 이후 Amount(Unit) 지나면 — 풀릴 때까지 확인할 때마다 알린다
//   always     조건 없이
// 확인 시점은 Times 의 각 시각, 또는 Watch 면 Times[0]~Times[1] 동안 계속(since 만).

type NotifySource struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Help  string   `json:"help"`
	Conds []string `json:"conds"`
}

var NotifySources = []NotifySource{
	{"tasks", "할 일", "오늘까지 마감인 할 일", []string{"pending"}},
	{"routines", "루틴", "오늘 해야 할 루틴", []string{"pending"}},
	{"care", "육아 기록", "고른 기록(수유·기저귀 등) 중 하나라도", []string{"since", "none_today"}},
	{"diary", "일기", "가족 일기", []string{"none_today", "since"}},
	{"finance", "자산 파일", "사람마다 마지막으로 올린 파일 — 그 사람에게 보내요", []string{"since"}},
	{"report", "월간 리포트", "매달 1일 정한 시각에, 지난달 우리 가족이 함께 쌓은 것을 정리해 보내요", []string{"always"}},
	{"custom", "조건 없이", "정한 시각에 문구만 보내요", []string{"always"}},
}

var NotifyConds = map[string]string{
	"pending":    "남아 있으면",
	"none_today": "오늘 아직 없으면",
	"since":      "마지막 이후 정한 시간이 지나면",
	"always":     "항상",
}

// NotifyParams 는 조건과 대상별 세부. 안 쓰는 칸은 비어 있다.
type NotifyParams struct {
	Cond   string  `json:"cond"`
	Amount float64 `json:"amount,omitempty"` // since: 얼마나
	Unit   string  `json:"unit,omitempty"`   // since: hours | days
	Watch  bool    `json:"watch,omitempty"`  // since: Times[0]~Times[1] 동안 계속 지켜본다
	// care
	ChildID   int64    `json:"child_id,omitempty"`
	CareKinds []string `json:"care_kinds,omitempty"`
	// tasks: true 면 담당자가 받는 사람인 카드만 센다
	MineOnly bool `json:"mine_only,omitempty"`
	// custom
	Message string `json:"message,omitempty"`
}

// NotifyRule 은 알림 하나다.
type NotifyRule struct {
	ID         int64        `json:"id"`
	Kind       string       `json:"kind"`
	Title      string       `json:"title"`
	DaysMask   int          `json:"days_mask"`
	Times      []string     `json:"times"`
	Params     NotifyParams `json:"params"`
	Recipients []int64      `json:"recipients"`
	Enabled    bool         `json:"enabled"`
	CreatedBy  *int64       `json:"created_by"`
	CreatedAt  int64        `json:"created_at"`
}

func notifySource(k string) (NotifySource, bool) {
	for _, x := range NotifySources {
		if x.Key == k {
			return x, true
		}
	}
	return NotifySource{}, false
}

// sinceDur 는 since 조건의 길이.
func (p NotifyParams) sinceDur() time.Duration {
	if p.Unit == "days" {
		return time.Duration(p.Amount * 24 * float64(time.Hour))
	}
	return time.Duration(p.Amount * float64(time.Hour))
}

func (r *NotifyRule) normalize() error {
	src, ok := notifySource(r.Kind)
	if !ok {
		return invalid("모르는 알림 대상이에요")
	}
	if r.Params.Cond == "" {
		r.Params.Cond = src.Conds[0]
	}
	if !slices.Contains(src.Conds, r.Params.Cond) {
		return invalid(src.Label + "에는 '" + NotifyConds[r.Params.Cond] + "' 조건을 쓸 수 없어요")
	}
	r.Title = strings.TrimSpace(r.Title)
	if len([]rune(r.Title)) > 100 {
		return invalid("이름이 너무 길어요")
	}
	if r.DaysMask <= 0 || r.DaysMask > 127 {
		return invalid("요일을 하나 이상 골라주세요")
	}
	if len(r.Times) == 0 || len(r.Times) > 12 {
		return invalid("시각을 1~12개 정해주세요")
	}
	for _, t := range r.Times {
		if err := bfCheckTime(t); err != nil {
			return err
		}
	}
	sort.Strings(r.Times)
	if r.Params.Cond == "since" {
		switch r.Params.Unit {
		case "hours":
			if r.Params.Amount <= 0 || r.Params.Amount > 72 {
				return invalid("몇 시간인지 정해주세요(최대 72)")
			}
		case "days":
			if r.Params.Amount < 1 || r.Params.Amount > 365 {
				return invalid("며칠인지 정해주세요(1~365)")
			}
		default:
			return invalid("시간인지 일인지 골라주세요")
		}
	} else {
		r.Params.Amount, r.Params.Unit, r.Params.Watch = 0, "", false
	}
	if r.Params.Watch && (len(r.Times) != 2 || r.Times[0] >= r.Times[1]) {
		return invalid("계속 지켜보려면 시작·끝 시각 두 개가 필요해요")
	}
	if r.Kind == "care" {
		if len(r.Params.CareKinds) == 0 {
			return invalid("어떤 기록을 볼지 골라주세요")
		}
		for _, k := range r.Params.CareKinds {
			if _, ok := careKind(k); !ok {
				return invalid("모르는 기록 종류예요: " + k)
			}
		}
	}
	if r.Kind == "custom" {
		r.Params.Message = strings.TrimSpace(r.Params.Message)
		if r.Params.Message == "" && r.Title == "" {
			return invalid("보낼 문구를 적어주세요")
		}
	}
	return nil
}

// NotifyRules 는 전부 준다.
func (s *Store) NotifyRules(ctx context.Context) ([]NotifyRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, kind, title, days_mask, times, params, recipients, enabled, created_by, created_at
		   FROM notify_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NotifyRule{}
	for rows.Next() {
		var r NotifyRule
		var times, params, recip string
		var en int
		if err := rows.Scan(&r.ID, &r.Kind, &r.Title, &r.DaysMask, &times, &params, &recip, &en, &r.CreatedBy, &r.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(times), &r.Times)
		_ = json.Unmarshal([]byte(params), &r.Params)
		_ = json.Unmarshal([]byte(recip), &r.Recipients)
		if r.Times == nil {
			r.Times = []string{}
		}
		if r.Recipients == nil {
			r.Recipients = []int64{}
		}
		r.Enabled = en != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// NotifySave 는 만들거나(ID==0) 고친다.
func (s *Store) NotifySave(ctx context.Context, r NotifyRule, userID int64) (NotifyRule, error) {
	if err := r.normalize(); err != nil {
		return NotifyRule{}, err
	}
	if r.Recipients == nil {
		r.Recipients = []int64{}
	}
	times, _ := json.Marshal(r.Times)
	params, _ := json.Marshal(r.Params)
	recip, _ := json.Marshal(r.Recipients)
	if r.ID == 0 {
		res, err := s.db.ExecContext(ctx,
			`INSERT INTO notify_rules (kind, title, days_mask, times, params, recipients, enabled, created_by, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.Kind, r.Title, r.DaysMask, string(times), string(params), string(recip), boolInt(r.Enabled), userID, time.Now().Unix())
		if err != nil {
			return NotifyRule{}, err
		}
		r.ID, _ = res.LastInsertId()
	} else {
		res, err := s.db.ExecContext(ctx,
			`UPDATE notify_rules SET kind=?, title=?, days_mask=?, times=?, params=?, recipients=?, enabled=? WHERE id=?`,
			r.Kind, r.Title, r.DaysMask, string(times), string(params), string(recip), boolInt(r.Enabled), r.ID)
		if err != nil {
			return NotifyRule{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return NotifyRule{}, ErrNotFound
		}
	}
	all, err := s.NotifyRules(ctx)
	if err != nil {
		return NotifyRule{}, err
	}
	for _, x := range all {
		if x.ID == r.ID {
			return x, nil
		}
	}
	return NotifyRule{}, ErrNotFound
}

// NotifyDelete 는 규칙을 지운다.
func (s *Store) NotifyDelete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM notify_rules WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- 판정 ---

// Notification 은 보낼 것 하나다.
type Notification struct {
	RuleID     int64
	Key        string // 중복 방지 단위
	Title      string
	Body       string
	URL        string // 누르면 열 화면
	Recipients []int64
}

// NotifyDue 는 지금 보낼 알림을 모은다(이미 보낸 건 뺀다).
// 서버가 잠깐 멈춰도 놓치지 않게 정한 시각부터 10분 안이면 보낸다.
func (s *Store) NotifyDue(ctx context.Context, now time.Time) ([]Notification, error) {
	rules, err := s.NotifyRules(ctx)
	if err != nil {
		return nil, err
	}
	date := now.Format("2006-01-02")
	hm := now.Format("15:04")
	bit := 1 << ((int(now.Weekday()) + 6) % 7)

	var out []Notification
	for _, r := range rules {
		if !r.Enabled || r.DaysMask&bit == 0 {
			continue
		}
		if r.Params.Cond == "since" {
			// Watch: 구간 안이면 매번 확인하고, 같은 공백(마지막 시점)에는 한 번만.
			// 시각: 그 시각마다 확인 — 풀릴 때까지 매번 알린다.
			var slots []string
			if r.Params.Watch {
				if hm >= r.Times[0] && hm < r.Times[1] {
					slots = []string{""}
				}
			} else {
				for _, t := range r.Times {
					if withinMinutes(t, hm, 10) {
						slots = append(slots, t)
					}
				}
			}
			if len(slots) == 0 {
				continue
			}
			hits, err := s.notifySince(ctx, r, now)
			if err != nil {
				return nil, err
			}
			for _, slot := range slots {
				for _, n := range hits {
					if slot == "" {
						n.Key = "since:" + n.Key
					} else {
						n.Key = date + "T" + slot + "#" + n.Key
					}
					out = append(out, n)
				}
			}
			continue
		}
		for _, t := range r.Times {
			if !withinMinutes(t, hm, 10) {
				continue
			}
			key := date + "T" + t
			n, ok, err := s.evalAt(ctx, r, date)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			n.RuleID, n.Key, n.Recipients = r.ID, key, r.Recipients
			out = append(out, n)
		}
	}
	// 이미 보낸 것 빼기는 위 조회가 끝난 뒤에(커넥션 하나).
	kept := out[:0]
	for _, n := range out {
		var x int
		err := s.db.QueryRowContext(ctx, `SELECT 1 FROM notify_sent WHERE rule_id=? AND key=?`, n.RuleID, n.Key).Scan(&x)
		if errors.Is(err, sql.ErrNoRows) {
			kept = append(kept, n)
		} else if err != nil {
			return nil, err
		}
	}
	return kept, nil
}

// NotifyMarkSent 는 보냈다고 적는다.
func (s *Store) NotifyMarkSent(ctx context.Context, ruleID int64, key string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notify_sent (rule_id, key, sent_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`,
		ruleID, key, time.Now().Unix())
	return err
}

// withinMinutes: t ≤ now < t+n (같은 날 'HH:MM' 끼리).
func withinMinutes(t, now string, n int) bool {
	tm, err1 := time.Parse("15:04", t)
	nm, err2 := time.Parse("15:04", now)
	if err1 != nil || err2 != nil {
		return false
	}
	d := nm.Sub(tm)
	return d >= 0 && d < time.Duration(n)*time.Minute
}

// evalAt 은 조건을 보내는 순간 다시 본다. ok=false 면 풀린 것.
func (s *Store) evalAt(ctx context.Context, r NotifyRule, date string) (Notification, bool, error) {
	name := r.Title
	switch r.Kind {
	case "tasks":
		cards, err := s.TodoCards(ctx, date, SortTime, Asc)
		if err != nil {
			return Notification{}, false, err
		}
		var left []string
		for _, c := range cards {
			// 마감 없는 카드는 세지 않는다 — 안 그러면 매일 울린다.
			if c.DueAt == nil {
				continue
			}
			if r.Params.MineOnly && len(r.Recipients) > 0 && (c.AssigneeID == nil || !containsID(r.Recipients, *c.AssigneeID)) {
				continue
			}
			left = append(left, c.Title)
		}
		if len(left) == 0 {
			return Notification{}, false, nil
		}
		return Notification{Title: orDefault(name, "오늘 할 일이 남았어요"), Body: summarize(left), URL: "/"}, true, nil
	case "routines":
		rs, err := s.RoutinesForDate(ctx, date)
		if err != nil {
			return Notification{}, false, err
		}
		var left []string
		for _, x := range rs {
			if x.CheckedAt == nil {
				left = append(left, x.Title)
			}
		}
		if len(left) == 0 {
			return Notification{}, false, nil
		}
		return Notification{Title: orDefault(name, "오늘 루틴이 남았어요"), Body: summarize(left), URL: "/routines"}, true, nil
	case "care":
		n, err := s.careCountOn(ctx, r, date)
		if err != nil {
			return Notification{}, false, err
		}
		if n > 0 {
			return Notification{}, false, nil
		}
		return Notification{Title: orDefault(name, "오늘 "+s.careLabels(r)+" 기록이 아직 없어요"), Body: "잊지 않았는지 확인해보세요.", URL: "/care"}, true, nil
	case "diary":
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM diary WHERE date=?`, date).Scan(&n); err != nil {
			return Notification{}, false, err
		}
		if n > 0 {
			return Notification{}, false, nil
		}
		return Notification{Title: orDefault(name, "오늘 일기를 아직 안 썼어요"), Body: "오늘 있었던 일 한 줄만 남겨볼까요?", URL: "/diary/new"}, true, nil
	case "report":
		if date[8:10] != "01" {
			return Notification{}, false, nil
		}
		d, _ := time.Parse("2006-01-02", date)
		month := d.AddDate(0, -1, 0).Format("2006-01")
		body, err := s.FamilyMonthReport(ctx, month)
		if err != nil {
			return Notification{}, false, err
		}
		return Notification{Title: orDefault(name, fmt.Sprintf("%d월 우리 가족 리포트", int(d.AddDate(0, -1, 0).Month()))), Body: body, URL: "/family"}, true, nil
	case "custom":
		body := r.Params.Message
		title := orDefault(name, "알림")
		if body == "" {
			body, title = title, "알림"
		}
		return Notification{Title: title, Body: body, URL: "/"}, true, nil
	}
	return Notification{}, false, nil
}

// sinceHit 은 since 조건에 걸린 것 하나. Key 는 "같은 공백"의 단위(대상+마지막 시점).
// notifySince 는 대상의 마지막 시점을 보고, 정한 시간이 지난 것들을 준다.
func (s *Store) notifySince(ctx context.Context, r NotifyRule, now time.Time) ([]Notification, error) {
	limit := r.Params.sinceDur()
	ago := func(last time.Time) string {
		d := now.Sub(last)
		if r.Params.Unit == "days" {
			return fmt.Sprintf("%d일", int(d.Hours()/24))
		}
		return fmt.Sprintf("%d시간 %d분", int(d.Hours()), int(d.Minutes())%60)
	}
	switch r.Kind {
	case "care":
		ph := strings.TrimSuffix(strings.Repeat("?,", len(r.Params.CareKinds)), ",")
		args := []any{r.Params.ChildID}
		for _, k := range r.Params.CareKinds {
			args = append(args, k)
		}
		var id int64
		var at string
		// 종류는 검증된 키들이고 자리표시자로만 들어간다.
		err := s.db.QueryRowContext(ctx,
			`SELECT id, at FROM care_logs WHERE child_id=? AND kind IN (`+ph+`) ORDER BY at DESC, id DESC LIMIT 1`, args...).Scan(&id, &at)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // 한 번도 안 적었으면 기준이 없다
		}
		if err != nil {
			return nil, err
		}
		last, err := time.ParseInLocation("2006-01-02T15:04", at, now.Location())
		if err != nil || now.Sub(last) < limit {
			return nil, nil
		}
		return []Notification{{
			RuleID: r.ID, Key: fmt.Sprintf("log:%d", id), Recipients: r.Recipients, URL: "/care",
			Title: orDefault(r.Title, s.careLabels(r)+" 기록이 한동안 없어요"),
			Body:  fmt.Sprintf("마지막 기록이 %s 전(%s)이에요.", ago(last), at[5:10]+" "+at[11:]),
		}}, nil
	case "diary":
		var date string
		err := s.db.QueryRowContext(ctx, `SELECT max(date) FROM diary`).Scan(&date)
		if err != nil || date == "" {
			return nil, nil
		}
		last, err := time.ParseInLocation("2006-01-02", date, now.Location())
		if err != nil || now.Sub(last) < limit {
			return nil, nil
		}
		return []Notification{{
			RuleID: r.ID, Key: "diary:" + date, Recipients: r.Recipients, URL: "/diary/new",
			Title: orDefault(r.Title, "일기를 한동안 안 썼어요"),
			Body:  fmt.Sprintf("마지막 일기가 %s(%s 전)이에요. 한 줄만 남겨볼까요?", date[5:], ago(last)),
		}}, nil
	case "finance":
		rows, err := s.db.QueryContext(ctx, `SELECT s.owner_id, u.name, max(s.created_at) FROM fin_snapshots s JOIN users u ON u.id = s.owner_id GROUP BY s.owner_id`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []Notification
		for rows.Next() {
			var owner, lastUnix int64
			var name string
			if err := rows.Scan(&owner, &name, &lastUnix); err != nil {
				return nil, err
			}
			if len(r.Recipients) > 0 && !containsID(r.Recipients, owner) {
				continue
			}
			last := time.Unix(lastUnix, 0)
			if now.Sub(last) < limit {
				continue
			}
			out = append(out, Notification{
				RuleID: r.ID, Key: fmt.Sprintf("fin:%d:%d", owner, lastUnix), Recipients: []int64{owner}, URL: "/finance",
				Title: orDefault(r.Title, "자산 파일을 올릴 때예요"),
				Body:  fmt.Sprintf("%s님, 마지막으로 올린 지 %s 지났어요. 새로 받아서 올려주세요.", name, ago(last)),
			})
		}
		return out, rows.Err()
	}
	return nil, nil
}

func (s *Store) careLabels(r NotifyRule) string {
	var labels []string
	for _, k := range r.Params.CareKinds {
		kd, _ := careKind(k)
		labels = append(labels, kd.Label)
	}
	return strings.Join(labels, "·")
}

func (s *Store) careCountOn(ctx context.Context, r NotifyRule, date string) (int, error) {
	ph := strings.TrimSuffix(strings.Repeat("?,", len(r.Params.CareKinds)), ",")
	args := []any{r.Params.ChildID, date + "T00:00", date + "T23:59"}
	for _, k := range r.Params.CareKinds {
		args = append(args, k)
	}
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM care_logs WHERE child_id=? AND at >= ? AND at <= ? AND kind IN (`+ph+`)`, args...).Scan(&n)
	return n, err
}

func containsID(xs []int64, v int64) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

// summarize 는 "빨래, 장보기 외 2개" 처럼 줄인다.
func summarize(xs []string) string {
	if len(xs) <= 2 {
		return strings.Join(xs, ", ")
	}
	return fmt.Sprintf("%s, %s 외 %d개", xs[0], xs[1], len(xs)-2)
}

// --- 기기 ---

// PushSub 은 기기 하나의 구독이다.
type PushSub struct {
	Endpoint string `json:"endpoint"`
	UserID   int64  `json:"user_id"`
	P256dh   string `json:"p256dh"`
	Auth     string `json:"auth"`
	Device   string `json:"device"`
}

func (s *Store) PushSubscribe(ctx context.Context, p PushSub) error {
	if !strings.HasPrefix(p.Endpoint, "https://") || p.P256dh == "" || p.Auth == "" {
		return invalid("구독 정보가 올바르지 않아요")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO push_subs (endpoint, user_id, p256dh, auth, device, created_at) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(endpoint) DO UPDATE SET user_id=excluded.user_id, p256dh=excluded.p256dh, auth=excluded.auth, device=excluded.device`,
		p.Endpoint, p.UserID, p.P256dh, p.Auth, p.Device, time.Now().Unix())
	return err
}

func (s *Store) PushUnsubscribe(ctx context.Context, endpoint string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM push_subs WHERE endpoint=?`, endpoint)
	return err
}

// PushSubs 는 받는 사람들의 기기다. users 가 비면 모두.
func (s *Store) PushSubs(ctx context.Context, users []int64) ([]PushSub, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT endpoint, user_id, p256dh, auth, device FROM push_subs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PushSub
	for rows.Next() {
		var p PushSub
		if err := rows.Scan(&p.Endpoint, &p.UserID, &p.P256dh, &p.Auth, &p.Device); err != nil {
			return nil, err
		}
		if len(users) == 0 || containsID(users, p.UserID) {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

func (s *Store) PushTouch(ctx context.Context, endpoint string) {
	_, _ = s.db.ExecContext(ctx, `UPDATE push_subs SET last_ok_at=? WHERE endpoint=?`, time.Now().Unix(), endpoint)
}

// --- 설정 값 ---

func (s *Store) KVGet(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM kv WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

func (s *Store) KVSet(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO kv (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}
