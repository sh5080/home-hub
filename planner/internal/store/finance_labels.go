package store

import (
	"context"
	"strings"
	"time"
)

// FinLabel 은 사람이 고친 이름·분류. 빈 값은 파일 그대로.
type FinLabel struct {
	Label string `json:"label"`
	Cat1  string `json:"cat1"`
	Memo  string `json:"memo"`
}

func (s *Store) finLabels(ctx context.Context) (map[string]FinLabel, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, label, cat1, memo FROM fin_labels`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]FinLabel{}
	for rows.Next() {
		var k string
		var l FinLabel
		if err := rows.Scan(&k, &l.Label, &l.Cat1, &l.Memo); err != nil {
			return nil, err
		}
		out[k] = l
	}
	return out, rows.Err()
}

// FinanceSetLabel 은 항목들(keys)의 이름·분류·메모를 고친다. 셋 다 비우면 원래대로.
func (s *Store) FinanceSetLabel(ctx context.Context, keys []string, label, cat1, memo string) error {
	label, cat1, memo = strings.TrimSpace(label), strings.TrimSpace(cat1), strings.TrimSpace(memo)
	if len([]rune(memo)) > 200 {
		return invalid("메모는 200자까지예요")
	}
	if len(keys) == 0 || len(keys) > 50 {
		return invalid("항목이 비었어요")
	}
	if len([]rune(label)) > 40 || len([]rune(cat1)) > 20 {
		return invalid("이름은 40자, 분류는 20자까지예요")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			return invalid("항목이 비었어요")
		}
		if label == "" && cat1 == "" && memo == "" {
			_, err = tx.ExecContext(ctx, `DELETE FROM fin_labels WHERE key=?`, k)
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO fin_labels (key, label, cat1, memo) VALUES (?, ?, ?, ?)
				ON CONFLICT(key) DO UPDATE SET label=excluded.label, cat1=excluded.cat1, memo=excluded.memo`, k, label, cat1, memo)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// FinDetail 은 항목 하나(또는 묶인 여러 key)의 최근 13개월 거래.
type FinDetail struct {
	Months []struct {
		Month  string `json:"month"`
		Amount int64  `json:"amount"`
		Count  int    `json:"count"`
	} `json:"months"`
	Tx []FinDetailTx `json:"tx"`
}

type FinDetailTx struct {
	OwnerID int64 `json:"owner_id"`
	FinTx
}

func (s *Store) FinanceDetail(ctx context.Context, owner int64, keys []string, now time.Time) (FinDetail, error) {
	out := FinDetail{Tx: []FinDetailTx{}}
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	from := now.AddDate(0, -13, 0).Format("2006-01") + "-01T00:00"
	q := `SELECT owner_id, at, type, cat1, cat2, content, amount, method, memo FROM fin_tx WHERE at >= ?`
	args := []any{from}
	if owner != 0 {
		q += ` AND owner_id = ?`
		args = append(args, owner)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY at DESC`, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	byMonth := map[string]int{}
	for rows.Next() {
		var t FinDetailTx
		if err := rows.Scan(&t.OwnerID, &t.At, &t.Type, &t.Cat1, &t.Cat2, &t.Content, &t.Amount, &t.Method, &t.Memo); err != nil {
			return out, err
		}
		if !want[finKey(t.Content)] {
			continue
		}
		out.Tx = append(out.Tx, t)
		m := t.At[:7]
		i, ok := byMonth[m]
		if !ok {
			i = len(out.Months)
			byMonth[m] = i
			out.Months = append(out.Months, struct {
				Month  string `json:"month"`
				Amount int64  `json:"amount"`
				Count  int    `json:"count"`
			}{Month: m})
		}
		out.Months[i].Amount += t.Amount
		out.Months[i].Count++
	}
	return out, rows.Err()
}
