package store

import (
	"fmt"
	"time"
)

// backfillEvents 는 0006 이전 events 행을 cards 로 옮기고 표를 지운다(멱등).
// 지난 일정은 완료 칸, 앞으로의 일정은 첫 칸으로.
func (s *Store) backfillEvents() error {
	var exists int
	if err := s.db.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='events'`).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}

	type ev struct {
		title, startAt string
		endAt, content *string
		desc           string
		createdBy      int64
		createdAt      int64
	}
	rows, err := s.db.Query(`
		SELECT title, start_at, end_at, description, content, created_by, created_at
		FROM events ORDER BY id`)
	if err != nil {
		return fmt.Errorf("read events: %w", err)
	}
	var evs []ev
	for rows.Next() {
		var e ev
		if err := rows.Scan(&e.title, &e.startAt, &e.endAt, &e.desc, &e.content, &e.createdBy, &e.createdAt); err != nil {
			rows.Close()
			return err
		}
		evs = append(evs, e)
	}
	rows.Close()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if len(evs) > 0 {
		// 카드가 가장 많은 보드로 보낸다.
		var boardID int64
		if err := tx.QueryRow(`
			SELECT b.id FROM boards b
			LEFT JOIN columns col ON col.board_id = b.id
			LEFT JOIN cards c ON c.column_id = col.id
			GROUP BY b.id ORDER BY count(c.id) DESC, b.id LIMIT 1`).Scan(&boardID); err != nil {
			return fmt.Errorf("pick board for events: %w", err)
		}
		var firstCol, lastCol int64
		if err := tx.QueryRow(`SELECT id FROM columns WHERE board_id=? ORDER BY position, id LIMIT 1`, boardID).Scan(&firstCol); err != nil {
			return fmt.Errorf("first column: %w", err)
		}
		if err := tx.QueryRow(`SELECT id FROM columns WHERE board_id=? ORDER BY position DESC, id DESC LIMIT 1`, boardID).Scan(&lastCol); err != nil {
			return fmt.Errorf("last column: %w", err)
		}

		next := map[int64]int{}
		for _, col := range []int64{firstCol, lastCol} {
			var n int
			if err := tx.QueryRow(`SELECT COALESCE(MAX(position)+1, 0) FROM cards WHERE column_id=?`, col).Scan(&n); err != nil {
				return err
			}
			next[col] = n
		}

		todayStr := time.Now().Format("2006-01-02")
		for _, e := range evs {
			col := firstCol
			if e.startAt < todayStr { // 사전식 비교 = 시간순
				col = lastCol
			}
			if _, err := tx.Exec(`
				INSERT INTO cards (column_id, title, description, content, position, due_at, end_at, priority, created_by, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)`,
				col, e.title, e.desc, e.content, next[col], e.startAt, e.endAt, e.createdBy, e.createdAt, e.createdAt); err != nil {
				return fmt.Errorf("move event %q: %w", e.title, err)
			}
			next[col]++
		}
	}

	if _, err := tx.Exec(`DROP TABLE events`); err != nil {
		return fmt.Errorf("drop events: %w", err)
	}
	return tx.Commit()
}
