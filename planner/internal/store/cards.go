package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// CardInput 은 생성·수정 입력. nil 필드는 건드리지 않는다.
type CardInput struct {
	Title      *string
	Content    *string // 블록 문서 JSON. description은 여기서 파생한다.
	DueAt      *string // "" clears. 'YYYY-MM-DD' 또는 'YYYY-MM-DDTHH:MM'
	EndAt      *string // "" clears. 여러 날에 걸치는 항목의 끝
	Recur      *string // "" clears. store/recur.go 의 규칙
	RecurUntil *string // "" clears
	AssigneeID *int64  // 0 clears
	Priority   *int    // 0=없음, 1~3
}

// validDue 는 'YYYY-MM-DD' 또는 'YYYY-MM-DDTHH:MM'. 사전식 비교가 시간순이다.
func validDue(s string) bool { return validDate(s) || validDateTime(s) }

const maxPriority = 3

func clampPriority(p int) int {
	if p < 0 {
		return 0
	}
	if p > maxPriority {
		return maxPriority
	}
	return p
}

// CreateCard appends a card to the end of columnID.
func (s *Store) CreateCard(ctx context.Context, columnID int64, in CardInput, by int64) (Card, error) {
	title := ""
	if in.Title != nil {
		title = strings.TrimSpace(*in.Title)
	}
	if title == "" {
		return Card{}, invalid("title is empty")
	}
	var content *string
	desc := ""
	if in.Content != nil {
		plain, err := ValidateContent(*in.Content)
		if err != nil {
			return Card{}, err
		}
		content, desc = in.Content, plain
	}
	var due *string
	if in.DueAt != nil && *in.DueAt != "" {
		if !validDue(*in.DueAt) {
			return Card{}, invalid("마감은 YYYY-MM-DD 또는 YYYY-MM-DDTHH:MM 형식이어야 해요")
		}
		due = in.DueAt
	}
	var end *string
	if in.EndAt != nil && *in.EndAt != "" {
		if !validDue(*in.EndAt) {
			return Card{}, invalid("끝은 YYYY-MM-DD 또는 YYYY-MM-DDTHH:MM 형식이어야 해요")
		}
		if due == nil {
			return Card{}, invalid("끝만 있고 시작이 없어요")
		}
		if *in.EndAt < *due {
			return Card{}, invalid("끝이 시작보다 빨라요")
		}
		end = in.EndAt
	}
	recur, until, err := validateRecurInput(in, due)
	if err != nil {
		return Card{}, err
	}
	priority := 0
	if in.Priority != nil {
		priority = clampPriority(*in.Priority)
	}
	var assignee *int64
	if in.AssigneeID != nil && *in.AssigneeID != 0 {
		assignee = in.AssigneeID
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Card{}, err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM columns WHERE id=?`, columnID).Scan(&exists); err != nil {
		return Card{}, err
	}
	if exists == 0 {
		return Card{}, ErrNotFound
	}
	var pos int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position)+1, 0) FROM cards WHERE column_id=?`, columnID).Scan(&pos); err != nil {
		return Card{}, err
	}
	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO cards (column_id, title, description, content, position, due_at, end_at, priority, assignee_id, created_by, created_at, updated_at, recur, recur_until, first_due)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		columnID, title, desc, content, pos, due, end, priority, assignee, by, now, now, recur, until, due)
	if err != nil {
		return Card{}, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return Card{}, err
	}
	c := Card{ID: id, ColumnID: columnID, Title: title, Description: desc, Content: content, Position: pos,
		DueAt: due, EndAt: end, Priority: priority, AssigneeID: assignee, CreatedBy: by, CreatedAt: now, UpdatedAt: now,
		Recur: recur, RecurUntil: until}
	fillRecurLabel(&c)
	return c, nil
}

// UpdateCard patches the named fields.
func (s *Store) UpdateCard(ctx context.Context, id int64, in CardInput) (Card, error) {
	// 반복 규칙 값은 마감에서 끌어온다(바뀌는 마감이 있으면 그것, 없으면 저장된 것).
	dueForRecur := ""
	if in.Recur != nil && *in.Recur != "" {
		if in.DueAt != nil {
			dueForRecur = *in.DueAt
		} else {
			var cur *string
			if err := s.db.QueryRowContext(ctx, `SELECT due_at FROM cards WHERE id=?`, id).Scan(&cur); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return Card{}, ErrNotFound
				}
				return Card{}, err
			}
			if cur != nil {
				dueForRecur = *cur
			}
		}
	}
	sets := []string{}
	args := []any{}
	if in.Title != nil {
		t := strings.TrimSpace(*in.Title)
		if t == "" {
			return Card{}, invalid("title is empty")
		}
		sets = append(sets, "title=?")
		args = append(args, t)
	}
	if in.Content != nil {
		plain, err := ValidateContent(*in.Content)
		if err != nil {
			return Card{}, err
		}
		sets = append(sets, "content=?", "description=?")
		args = append(args, *in.Content, plain)
	}
	if in.DueAt != nil {
		if *in.DueAt == "" {
			sets = append(sets, "due_at=NULL")
		} else {
			if !validDue(*in.DueAt) {
				return Card{}, invalid("마감은 YYYY-MM-DD 또는 YYYY-MM-DDTHH:MM 형식이어야 해요")
			}
			// 처음 정한 마감은 한 번만 적는다(미뤄도 남는다).
			sets = append(sets, "due_at=?", "first_due=COALESCE(first_due, ?)")
			args = append(args, *in.DueAt, *in.DueAt)
		}
	}
	if in.EndAt != nil {
		if *in.EndAt == "" {
			sets = append(sets, "end_at=NULL")
		} else {
			if !validDue(*in.EndAt) {
				return Card{}, invalid("끝은 YYYY-MM-DD 또는 YYYY-MM-DDTHH:MM 형식이어야 해요")
			}
			sets = append(sets, "end_at=?")
			args = append(args, *in.EndAt)
		}
	}
	if in.Recur != nil {
		if *in.Recur == "" {
			sets = append(sets, "recur=NULL")
		} else {
			rule, err := ExpandRecur(*in.Recur, dueForRecur)
			if err != nil {
				return Card{}, err
			}
			sets = append(sets, "recur=?")
			args = append(args, rule)
		}
	}
	if in.RecurUntil != nil {
		if *in.RecurUntil == "" {
			sets = append(sets, "recur_until=NULL")
		} else {
			if !validDue(*in.RecurUntil) {
				return Card{}, invalid("반복 종료일은 YYYY-MM-DD 형식이어야 해요")
			}
			sets = append(sets, "recur_until=?")
			args = append(args, (*in.RecurUntil)[:10])
		}
	}
	if in.Priority != nil {
		sets = append(sets, "priority=?")
		args = append(args, clampPriority(*in.Priority))
	}
	if in.AssigneeID != nil {
		if *in.AssigneeID == 0 {
			sets = append(sets, "assignee_id=NULL")
		} else {
			sets = append(sets, "assignee_id=?")
			args = append(args, *in.AssigneeID)
		}
	}
	if len(sets) > 0 {
		sets = append(sets, "updated_at=?")
		args = append(args, time.Now().Unix())
		args = append(args, id)
		res, err := s.db.ExecContext(ctx, `UPDATE cards SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...)
		if err != nil {
			return Card{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return Card{}, ErrNotFound
		}
	}
	return s.GetCard(ctx, id)
}

// GetCard loads one card.
func (s *Store) GetCard(ctx context.Context, id int64) (Card, error) {
	var c Card
	err := s.db.QueryRowContext(ctx, `
		SELECT id, column_id, title, description, content, position, due_at, end_at, priority, assignee_id, created_by, created_at, updated_at, recur, recur_until, recur_parent_id, done_at, archived_at
		FROM cards WHERE id=?`, id).
		Scan(&c.ID, &c.ColumnID, &c.Title, &c.Description, &c.Content, &c.Position, &c.DueAt, &c.EndAt, &c.Priority, &c.AssigneeID, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.Recur, &c.RecurUntil, &c.RecurParentID, &c.DoneAt, &c.ArchivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, ErrNotFound
	}
	fillRecurLabel(&c)
	return c, err
}

// CardDetail 은 카드 상세용 — 카드, 보드, 상태(칸) 선택지. 칸에는 카드를 싣지 않는다.
type CardDetail struct {
	Card    Card     `json:"card"`
	Board   Board    `json:"board"`
	Columns []Column `json:"columns"`
}

// GetCardDetail loads a card with its board and the board's columns.
func (s *Store) GetCardDetail(ctx context.Context, id int64) (CardDetail, error) {
	var d CardDetail
	c, err := s.GetCard(ctx, id)
	if err != nil {
		return d, err
	}
	d.Card = c

	err = s.db.QueryRowContext(ctx, `
		SELECT b.id, b.name, b.created_by, b.created_at
		FROM boards b JOIN columns col ON col.board_id = b.id
		WHERE col.id = ?`, c.ColumnID).
		Scan(&d.Board.ID, &d.Board.Name, &d.Board.CreatedBy, &d.Board.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, board_id, name, position FROM columns WHERE board_id=? ORDER BY position, id`, d.Board.ID)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	d.Columns = []Column{}
	for rows.Next() {
		var col Column
		if err := rows.Scan(&col.ID, &col.BoardID, &col.Name, &col.Position); err != nil {
			return d, err
		}
		col.Cards = []Card{}
		d.Columns = append(d.Columns, col)
	}
	return d, rows.Err()
}

// MoveCard 는 splice 의미론으로 옮긴다: 원래 칸에서 빼 간격을 닫고, 대상 칸에 끼운다.
// newPos 는 [0, len(dest)] 로 자른다.
func (s *Store) MoveCard(ctx context.Context, id, toColumn int64, newPos int) (Card, error) {
	var reward *TaskReward
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Card{}, err
	}
	defer tx.Rollback()

	var fromColumn int64
	var oldPos int
	err = tx.QueryRowContext(ctx, `SELECT column_id, position FROM cards WHERE id=?`, id).Scan(&fromColumn, &oldPos)
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, ErrNotFound
	}
	if err != nil {
		return Card{}, err
	}

	var sameBoard int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*) FROM columns a JOIN columns b ON a.board_id = b.board_id
		WHERE a.id=? AND b.id=?`, fromColumn, toColumn).Scan(&sameBoard); err != nil {
		return Card{}, err
	}
	if sameBoard == 0 {
		return Card{}, ErrNotFound
	}

	// 1. remove from source: everything after it shifts down.
	if _, err := tx.ExecContext(ctx, `UPDATE cards SET position=position-1 WHERE column_id=? AND position>?`, fromColumn, oldPos); err != nil {
		return Card{}, err
	}
	// 2. clamp against the destination's size *after* removal.
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM cards WHERE column_id=? AND id<>?`, toColumn, id).Scan(&count); err != nil {
		return Card{}, err
	}
	if newPos < 0 {
		newPos = 0
	}
	if newPos > count {
		newPos = count
	}
	// 3. open a gap in the destination, then drop the card in.
	if _, err := tx.ExecContext(ctx, `UPDATE cards SET position=position+1 WHERE column_id=? AND position>=? AND id<>?`, toColumn, newPos, id); err != nil {
		return Card{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cards SET column_id=?, position=?, updated_at=? WHERE id=?`, toColumn, newPos, time.Now().Unix(), id); err != nil {
		return Card{}, err
	}
	// 완료 칸에 들어오면 done_at 을 적고, 나가면 지운다(보관도 풀린다).
	if toColumn != fromColumn {
		var last int64
		if err := tx.QueryRowContext(ctx,
			`SELECT c2.id FROM columns c1 JOIN columns c2 ON c2.board_id = c1.board_id
			  WHERE c1.id=? ORDER BY c2.position DESC, c2.id DESC LIMIT 1`, toColumn).Scan(&last); err != nil {
			return Card{}, err
		}
		if toColumn == last {
			if _, err := tx.ExecContext(ctx, `UPDATE cards SET done_at=? WHERE id=?`, time.Now().Unix(), id); err != nil {
				return Card{}, err
			}
			if reward, err = taskReward(ctx, tx, id, time.Now()); err != nil {
				return Card{}, err
			}
		} else if _, err := tx.ExecContext(ctx, `UPDATE cards SET done_at=NULL, archived_at=NULL WHERE id=?`, id); err != nil {
			return Card{}, err
		}
	}
	if err := spawnNextOccurrence(ctx, tx, id, toColumn); err != nil {
		return Card{}, err
	}
	if err := tx.Commit(); err != nil {
		return Card{}, err
	}
	c, err := s.GetCard(ctx, id)
	c.Reward = reward
	return c, err
}

// TaskReward 는 할 일을 끝냈을 때의 판정. 처음 정한 마감 안이면 물방울을 받고, 넘겼으면 받지 않는다.
type TaskReward struct {
	OnTime   bool   `json:"on_time"`
	FirstDue string `json:"first_due"`
	Drops    int64  `json:"drops"` // 이번에 받은 물방울(이미 받았거나 늦었으면 0)
}

const taskRewardDrops = 2

func taskReward(ctx context.Context, tx *sql.Tx, id int64, now time.Time) (*TaskReward, error) {
	var first sql.NullString
	var title string
	if err := tx.QueryRowContext(ctx, `SELECT first_due, title FROM cards WHERE id=?`, id).Scan(&first, &title); err != nil {
		return nil, err
	}
	if !first.Valid || first.String == "" {
		return nil, nil // 마감 없는 할 일은 판정하지 않는다
	}
	r := &TaskReward{FirstDue: first.String}
	// 시각까지 정했으면 그 시각까지, 날짜만이면 그날 끝까지.
	if len(first.String) > 10 {
		r.OnTime = now.Format("2006-01-02T15:04") <= first.String
	} else {
		r.OnTime = now.Format("2006-01-02") <= first.String
	}
	if !r.OnTime {
		return r, nil
	}
	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO family_drops (key, amount, day, label) VALUES (?, ?, ?, ?)`,
		fmt.Sprintf("task:%d", id), taskRewardDrops, now.Format("2006-01-02"), "할 일 · "+title+" (마감 지킴)")
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		r.Drops = taskRewardDrops
	}
	return r, nil
}

// spawnNextOccurrence 는 완료된 반복 카드의 다음 회차를 첫 칸에 새 행으로 만든다.
// 완료한 카드는 완료 칸에 그대로 두고 규칙만 뗀다.
func spawnNextOccurrence(ctx context.Context, tx *sql.Tx, id, toColumn int64) error {
	var boardID int64
	var lastCol, firstCol int64
	err := tx.QueryRowContext(ctx, `
		SELECT col.board_id,
		       (SELECT id FROM columns d WHERE d.board_id = col.board_id ORDER BY d.position DESC, d.id DESC LIMIT 1),
		       (SELECT id FROM columns d WHERE d.board_id = col.board_id ORDER BY d.position,     d.id     LIMIT 1)
		  FROM columns col WHERE col.id = ?`, toColumn).Scan(&boardID, &lastCol, &firstCol)
	if err != nil {
		return err
	}
	// 칸이 하나뿐이면 옮길 때마다 새 카드가 생기므로 아무것도 안 한다.
	if toColumn != lastCol || lastCol == firstCol {
		return nil
	}

	var title, desc string
	var content, due, end, recur, until *string
	var priority int
	var assignee *int64
	var by int64
	err = tx.QueryRowContext(ctx, `
		SELECT title, description, content, due_at, end_at, priority, assignee_id, created_by, recur, recur_until
		  FROM cards WHERE id=?`, id).
		Scan(&title, &desc, &content, &due, &end, &priority, &assignee, &by, &recur, &until)
	if err != nil {
		return err
	}
	if recur == nil || *recur == "" || due == nil {
		return nil
	}
	next, err := NextOccurrence(*recur, *due)
	if err != nil {
		return err
	}
	if until != nil && next[:10] > *until {
		_, err = tx.ExecContext(ctx, `UPDATE cards SET recur=NULL WHERE id=?`, id)
		return err
	}
	var nextEnd *string
	if end != nil {
		if shifted, err := shiftEnd(*due, *end, next); err == nil {
			nextEnd = &shifted
		}
	}

	var pos int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position)+1, 0) FROM cards WHERE column_id=?`, firstCol).Scan(&pos); err != nil {
		return err
	}
	now := time.Now().Unix()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cards (column_id, title, description, content, position, due_at, end_at, priority,
		                   assignee_id, created_by, created_at, updated_at, recur, recur_until, recur_parent_id, first_due)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		firstCol, title, desc, content, pos, next, nextEnd, priority, assignee, by, now, now, *recur, until, id, next); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE cards SET recur=NULL WHERE id=?`, id)
	return err
}

// shiftEnd는 기간의 길이를 유지한 채 끝을 새 시작에 맞춘다.
func shiftEnd(oldDue, oldEnd, newDue string) (string, error) {
	a, err := time.Parse("2006-01-02", oldDue[:10])
	if err != nil {
		return "", err
	}
	b, err := time.Parse("2006-01-02", oldEnd[:10])
	if err != nil {
		return "", err
	}
	c, err := time.Parse("2006-01-02", newDue[:10])
	if err != nil {
		return "", err
	}
	days := int(b.Sub(a).Hours() / 24)
	out := c.AddDate(0, 0, days).Format("2006-01-02")
	if len(oldEnd) > 10 {
		out += oldEnd[10:]
	}
	return out, nil
}

// DeleteCard removes the card and closes the gap in its column.
func (s *Store) DeleteCard(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var col int64
	var pos int
	err = tx.QueryRowContext(ctx, `SELECT column_id, position FROM cards WHERE id=?`, id).Scan(&col, &pos)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cards WHERE id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cards SET position=position-1 WHERE column_id=? AND position>?`, col, pos); err != nil {
		return err
	}
	return tx.Commit()
}

// CalendarCards 는 [from, to) 와 겹치는 카드. end_at 이 없으면 하루짜리로 본다.
func (s *Store) CalendarCards(ctx context.Context, from, to string) ([]Card, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, column_id, title, description, content, position, due_at, end_at, priority, assignee_id, created_by, created_at, updated_at, recur, recur_until, recur_parent_id
		FROM cards
		WHERE due_at IS NOT NULL AND due_at < ? AND COALESCE(end_at, due_at) >= ?
		ORDER BY due_at, id`, to, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Card{}
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ID, &c.ColumnID, &c.Title, &c.Description, &c.Content, &c.Position, &c.DueAt, &c.EndAt, &c.Priority, &c.AssigneeID, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.Recur, &c.RecurUntil, &c.RecurParentID); err != nil {
			return nil, err
		}
		fillRecurLabel(&c)
		out = append(out, c)
	}
	return out, rows.Err()
}

// validDate accepts exactly YYYY-MM-DD and rejects impossible dates.
func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil && len(s) == 10
}

// validDateTime accepts YYYY-MM-DDTHH:MM (the datetime-local format).
func validDateTime(s string) bool {
	_, err := time.Parse("2006-01-02T15:04", s)
	return err == nil && len(s) == 16
}

// TodoCard 는 첫 칸의 카드 + 완료 칸 id (홈에서 '완료' 처리용).
type TodoCard struct {
	Card
	BoardID      int64  `json:"board_id"`
	BoardName    string `json:"board_name"`
	DoneColumnID int64  `json:"done_column_id"`
}

// TodoCards 는 모든 보드의 첫 칸에서 오늘까지 마감인 카드(마감 없는 것 포함)를 준다.
func (s *Store) TodoCards(ctx context.Context, date string, sortBy Sort, order Order) ([]TodoCard, error) {
	if !validDue(date) {
		return nil, invalid("날짜는 YYYY-MM-DD 형식이어야 해요")
	}
	d, err := time.Parse("2006-01-02", date[:10])
	if err != nil {
		return nil, invalid("날짜는 YYYY-MM-DD 형식이어야 해요")
	}
	// 사전식 비교라 시각이 붙은 마감도 '< 내일' 경계가 맞다.
	tomorrow := d.AddDate(0, 0, 1).Format("2006-01-02")
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.column_id, c.title, c.description, c.content, c.position, c.due_at, c.end_at, c.priority, c.assignee_id, c.created_by, c.created_at, c.updated_at, c.recur, c.recur_until, c.recur_parent_id,
		       b.id, b.name,
		       (SELECT id FROM columns d WHERE d.board_id = b.id ORDER BY d.position DESC, d.id DESC LIMIT 1)
		FROM cards c
		JOIN columns col ON col.id = c.column_id
		JOIN boards b ON b.id = col.board_id
		WHERE col.position = 0 AND (c.due_at IS NULL OR c.due_at < ?)
		ORDER BY `+orderBy(sortBy, order, "c.")+`, b.id`, tomorrow)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TodoCard{}
	for rows.Next() {
		var t TodoCard
		if err := rows.Scan(&t.ID, &t.ColumnID, &t.Title, &t.Description, &t.Content, &t.Position, &t.DueAt, &t.EndAt, &t.Priority, &t.AssigneeID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
			&t.Recur, &t.RecurUntil, &t.RecurParentID,
			&t.BoardID, &t.BoardName, &t.DoneColumnID); err != nil {
			return nil, err
		}
		fillRecurLabel(&t.Card)
		out = append(out, t)
	}
	return out, rows.Err()
}

// validateRecurInput 은 반복 규칙을 검사한다. 마감 없는 반복은 막는다.
func validateRecurInput(in CardInput, due *string) (recur, until *string, err error) {
	if in.Recur != nil && *in.Recur != "" {
		if due == nil {
			return nil, nil, invalid("반복하려면 마감이 있어야 해요")
		}
		rule, e := ExpandRecur(*in.Recur, *due)
		if e != nil {
			return nil, nil, e
		}
		recur = &rule
	}
	if in.RecurUntil != nil && *in.RecurUntil != "" {
		if !validDue(*in.RecurUntil) {
			return nil, nil, invalid("반복 종료일은 YYYY-MM-DD 형식이어야 해요")
		}
		v := (*in.RecurUntil)[:10]
		until = &v
	}
	return recur, until, nil
}

// fillRecurLabel 은 화면용 설명을 채운다(규칙 해석은 서버에만).
func fillRecurLabel(c *Card) {
	if c.Recur != nil {
		c.RecurLabel = RecurLabel(*c.Recur)
	}
}
