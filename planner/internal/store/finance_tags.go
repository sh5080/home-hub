package store

import (
	"context"
	"strings"
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
