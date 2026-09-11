package store

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"
)

type FinTag struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// 화면이 아는 색 이름만 받는다(Tailwind 팔레트).
var finTagColors = map[string]bool{
	"emerald": true, "amber": true, "sky": true, "indigo": true, "violet": true, "pink": true, "rose": true,
	"cyan": true, "orange": true, "lime": true, "fuchsia": true, "slate": true, "teal": true, "yellow": true,
}

func (s *Store) FinanceTags(ctx context.Context) ([]FinTag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, color FROM fin_tags ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FinTag{}
	for rows.Next() {
		var t FinTag
		if err := rows.Scan(&t.ID, &t.Name, &t.Color); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// finTagLinks: key → 태그 id 들.
func (s *Store) finTagLinks(ctx context.Context) (map[string][]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT l.key, l.tag_id FROM fin_tag_links l JOIN fin_tags t ON t.id = l.tag_id ORDER BY t.position, t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]int64{}
	for rows.Next() {
		var k string
		var id int64
		if err := rows.Scan(&k, &id); err != nil {
			return nil, err
		}
		out[k] = append(out[k], id)
	}
	return out, rows.Err()
}

func finTagInput(name, color string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 12 {
		return "", "", invalid("태그 이름은 1~12자예요")
	}
	if color == "" {
		color = "slate"
	}
	if !finTagColors[color] {
		return "", "", invalid("모르는 색이에요")
	}
	return name, color, nil
}

func (s *Store) FinanceCreateTag(ctx context.Context, name, color string) (FinTag, error) {
	name, color, err := finTagInput(name, color)
	if err != nil {
		return FinTag{}, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO fin_tags (name, color, position) VALUES (?, ?, (SELECT coalesce(max(position), -1) + 1 FROM fin_tags))`, name, color)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return FinTag{}, invalid("같은 이름의 태그가 있어요")
		}
		return FinTag{}, err
	}
	id, _ := res.LastInsertId()
	return FinTag{ID: id, Name: name, Color: color}, nil
}

func (s *Store) FinanceUpdateTag(ctx context.Context, id int64, name, color string) error {
	name, color, err := finTagInput(name, color)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE fin_tags SET name=?, color=? WHERE id=?`, name, color, id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return invalid("같은 이름의 태그가 있어요")
		}
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) FinanceDeleteTag(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM fin_tags WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// FinanceSetTags 는 항목(key)의 태그를 통째로 바꾼다.
func (s *Store) FinanceSetTags(ctx context.Context, key string, ids []int64) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return invalid("항목이 비었어요")
	}
	if len(ids) > 10 {
		return invalid("태그는 10개까지예요")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM fin_tag_links WHERE key=?`, key); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO fin_tag_links (key, tag_id) VALUES (?, ?)`, key, id); err != nil {
			if strings.Contains(err.Error(), "FOREIGN KEY") {
				return invalid("없는 태그예요")
			}
			return err
		}
	}
	return tx.Commit()
}

var finRefRe = regexp.MustCompile(`^[tw][0-9]+$`)

func uniqIDs(xs []int64) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// finTxTags: 거래(ref) → 태그 id.
func (s *Store) finTxTags(ctx context.Context) (map[string][]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT x.ref, x.tag_id FROM fin_tx_tags x JOIN fin_tags t ON t.id = x.tag_id ORDER BY t.position, t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]int64{}
	for rows.Next() {
		var ref string
		var id int64
		if err := rows.Scan(&ref, &id); err != nil {
			return nil, err
		}
		out[ref] = append(out[ref], id)
	}
	return out, rows.Err()
}

// FinanceSetTxTags 는 거래들의 태그를 통째로 바꾼다.
func (s *Store) FinanceSetTxTags(ctx context.Context, refs []string, ids []int64) error {
	if len(refs) == 0 || len(refs) > 500 {
		return invalid("거래가 비었어요")
	}
	if len(ids) > 10 {
		return invalid("태그는 10개까지예요")
	}
	for _, r := range refs {
		if !finRefRe.MatchString(r) {
			return invalid("모르는 거래예요")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range refs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM fin_tx_tags WHERE ref=?`, r); err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO fin_tx_tags (ref, tag_id) VALUES (?, ?)`, r, id); err != nil {
				if strings.Contains(err.Error(), "FOREIGN KEY") {
					return invalid("없는 태그예요")
				}
				return err
			}
		}
	}
	return tx.Commit()
}

// finMigrateTxTags 는 이름 단위로 붙였던 태그를 지금 있는 거래들에 한 번 옮긴다(새 거래는 물려받지 않는다).
func (s *Store) finMigrateTxTags(ctx context.Context, links map[string][]int64) error {
	if _, ok, err := s.KVGet(ctx, "fin.tx_tags_migrated"); err != nil || ok {
		return err
	}
	type item struct{ ref, content string }
	var items []item
	rows, err := s.db.QueryContext(ctx, `SELECT 't' || id, content FROM fin_tx UNION ALL SELECT 'w' || id, title FROM fin_wallet_spends`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ref, &it.content); err != nil {
			rows.Close()
			return err
		}
		items = append(items, it)
	}
	rows.Close()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, it := range items {
		for _, id := range links[finKey(it.content)] {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO fin_tx_tags (ref, tag_id) VALUES (?, ?)`, it.ref, id); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO kv (key, value) VALUES ('fin.tx_tags_migrated', '1') ON CONFLICT(key) DO UPDATE SET value=excluded.value`); err != nil {
		return err
	}
	return tx.Commit()
}

// 미분류 보기에서 이보다 작은 거래(앱테크 적립, 몇백 원 결제)는 생략한다.
const finUntaggedMin = 1000

// FinanceUntagged 는 태그가 없는(분류하지 않은) 거래를 최근 달부터, 달 안에선 큰 금액부터 limit 개씩 준다.
// kind: regular(평소 지출) | unexpected(예상 외 지출) | income(예상 외 수입). 목록 밖의 칸은 overview 그대로다
// (태그·메모·이름 등을 화면이 같이 쓴다).
func (s *Store) FinanceUntagged(ctx context.Context, owner int64, kind string, offset, limit int, now time.Time) (FinOverview, int, error) {
	ov, err := s.financeOverview(ctx, owner, "", now, true)
	if err != nil {
		return ov, 0, err
	}
	type item struct {
		at     string
		amount int64
		i      int
	}
	var items []item
	switch kind {
	case "regular":
		for i, u := range ov.Regular {
			if len(ov.TxTags[u.Ref]) == 0 && u.Amount >= finUntaggedMin {
				items = append(items, item{u.At, u.Amount, i})
			}
		}
	case "unexpected":
		for i, u := range ov.Unexpected {
			if len(ov.TxTags[u.Ref]) == 0 && u.Amount >= finUntaggedMin {
				items = append(items, item{u.At, u.Amount, i})
			}
		}
	case "income":
		for i, u := range ov.IncomeTx {
			if len(ov.TxTags[u.Ref]) == 0 && u.Amount >= finUntaggedMin {
				items = append(items, item{u.At, u.Amount, i})
			}
		}
	default:
		return ov, 0, invalid("모르는 목록이에요")
	}
	sort.SliceStable(items, func(a, b int) bool {
		if items[a].at[:7] != items[b].at[:7] {
			return items[a].at[:7] > items[b].at[:7]
		}
		return items[a].amount > items[b].amount
	})
	total := len(items)
	if offset > total {
		offset = total
	}
	end := min(offset+limit, total)
	page := items[offset:end]
	reg, un, inc := []FinRegular{}, []FinUnexpected{}, []FinIncomeTx{}
	for _, it := range page {
		switch kind {
		case "regular":
			reg = append(reg, ov.Regular[it.i])
		case "unexpected":
			un = append(un, ov.Unexpected[it.i])
		case "income":
			inc = append(inc, ov.IncomeTx[it.i])
		}
	}
	ov.Regular, ov.Unexpected, ov.IncomeTx = reg, un, inc
	return ov, total, nil
}

// FinTagMatch 는 이름으로 찾은 거래 한 건.
type FinTagMatch struct {
	Ref     string
	At      string
	Content string
	Amount  int64
}

// FinanceTagByName 은 내용에 needle 이 든 거래(통장·지역화폐 사용)를 찾아, apply 면 태그들을 더한다(있던 태그는 둔다).
// 새 거래는 이름으로 태그를 물려받지 않으므로, 한 번에 정리할 때 쓴다.
// cat 이 있으면 이름 대신 분류(대분류)로 찾는다 — 사람이 고친 분류가 우선이다.
func (s *Store) FinanceTagByName(ctx context.Context, needle, cat string, tagNames []string, apply bool) ([]FinTagMatch, []FinTag, error) {
	needle, cat = strings.TrimSpace(needle), strings.TrimSpace(cat)
	if needle == "" && cat == "" {
		return nil, nil, invalid("찾을 이름이나 분류가 비었어요")
	}
	all, err := s.FinanceTags(ctx)
	if err != nil {
		return nil, nil, err
	}
	var picked []FinTag
	for _, n := range tagNames {
		found := false
		for _, t := range all {
			if t.Name == strings.TrimSpace(n) {
				picked, found = append(picked, t), true
			}
		}
		if !found {
			return nil, nil, invalid("없는 태그예요: " + n)
		}
	}
	labels, err := s.finLabels(ctx)
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT 't' || id, at, content, amount, cat1 FROM fin_tx
		UNION ALL
		SELECT 'w' || id, at, title, -amount, '' FROM fin_wallet_spends
		ORDER BY 2`)
	if err != nil {
		return nil, nil, err
	}
	var out []FinTagMatch
	low := strings.ToLower(needle)
	for rows.Next() {
		var m FinTagMatch
		var c1 string
		if err := rows.Scan(&m.Ref, &m.At, &m.Content, &m.Amount, &c1); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if l := labels[finKey(m.Content)]; l.Cat1 != "" {
			c1 = l.Cat1
		}
		if (needle == "" || strings.Contains(strings.ToLower(m.Content), low)) && (cat == "" || c1 == cat) {
			out = append(out, m)
		}
	}
	rows.Close()
	if !apply || len(out) == 0 {
		return out, picked, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	for _, m := range out {
		for _, t := range picked {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO fin_tx_tags (ref, tag_id) VALUES (?, ?)`, m.Ref, t.ID); err != nil {
				return nil, nil, err
			}
		}
	}
	return out, picked, tx.Commit()
}
