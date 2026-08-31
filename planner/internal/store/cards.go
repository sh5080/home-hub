package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// CardInput is what the API accepts for create/patch. Pointer fields are
// "not provided" when nil, so a PATCH only touches what it names.
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

// validDue는 마감 문자열을 받는다. 날짜만이면 "그날 언제든", 시각이 있으면
// 그 시각이다. 사전식 비교가 둘 다에서 시간순이라 범위 쿼리는 동일하다.
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
		INSERT INTO cards (column_id, title, description, content, position, due_at, end_at, priority, assignee_id, created_by, created_at, updated_at, recur, recur_until)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		columnID, title, desc, content, pos, due, end, priority, assignee, by, now, now, recur, until)
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
	// 반복 규칙의 값은 마감에서 끌어온다. 같이 바뀌는 마감이 있으면 그걸,
	// 없으면 지금 저장된 마감을 기준으로 한다.
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
			sets = append(sets, "due_at=?")
			args = append(args, *in.DueAt)
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
		SELECT id, column_id, title, description, content, position, due_at, end_at, priority, assignee_id, created_by, created_at, updated_at, recur, recur_until, recur_parent_id
		FROM cards WHERE id=?`, id).
		Scan(&c.ID, &c.ColumnID, &c.Title, &c.Description, &c.Content, &c.Position, &c.DueAt, &c.EndAt, &c.Priority, &c.AssigneeID, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.Recur, &c.RecurUntil, &c.RecurParentID)
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, ErrNotFound
	}
	fillRecurLabel(&c)
	return c, err
}

// CardDetail은 카드 상세 페이지가 한 번에 필요한 것 — 카드, 속한 보드,
// 그리고 상태(컬럼) 선택지. 컬럼에는 카드를 싣지 않는다(페이지가 안 쓴다).
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

// MoveCard places card id at index newPos in column toColumn using splice
// semantics: remove from the source list (closing the gap), then insert into
// the destination list (opening a gap). Works for same-column moves in either
// direction because the removal compacts first. newPos is clamped to
// [0, len(dest)].
//
// Three UPDATEs; O(n) on a column's cards, which is tiny. No fractional or
// gapped positions to renumber later.
func (s *Store) MoveCard(ctx context.Context, id, toColumn int64, newPos int) (Card, error) {
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

	// Destination must exist and be on the same board — cards don't cross boards.
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
	// 완료 칸으로 옮겼고 반복 카드라면 다음 회차를 새 카드로 만든다.
	if err := spawnNextOccurrence(ctx, tx, id, toColumn); err != nil {
		return Card{}, err
	}
	if err := tx.Commit(); err != nil {
		return Card{}, err
	}
	return s.GetCard(ctx, id)
}

// spawnNextOccurrence는 완료된 반복 카드의 다음 회차를 첫 칸에 만든다.
//
// 완료한 카드를 되돌리지 않는 게 요점이다. 드래그해서 '완료'에 넣었는데 그게
// 사라지고 '할 일'에 다시 나타나면, 사용자는 완료가 안 된 줄 안다. 완료한
// 것은 완료 칸에 그대로 남기고(반복 규칙은 떼어낸다 — 그 카드는 이제 지나간
// 한 회차다), 다음 회차를 새 행으로 만든다.
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
	// 칸이 하나뿐인 보드에서는 첫 칸이 곧 완료 칸이다. 그런 보드에서
	// 반복을 돌리면 옮길 때마다 새 카드가 생기므로 아무것도 하지 않는다.
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
	// 종료일을 넘었으면 여기서 멈춘다. 반복 규칙만 떼어내고 끝.
	if until != nil && next[:10] > *until {
		_, err = tx.ExecContext(ctx, `UPDATE cards SET recur=NULL WHERE id=?`, id)
		return err
	}
	// 여러 날 항목이면 길이를 유지한 채로 함께 민다.
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
		                   assignee_id, created_by, created_at, updated_at, recur, recur_until, recur_parent_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		firstCol, title, desc, content, pos, next, nextEnd, priority, assignee, by, now, now, *recur, until, id); err != nil {
		return err
	}
	// 완료한 카드는 이제 지나간 한 회차다 — 규칙을 떼어낸다.
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

// CalendarCards returns cards whose date range overlaps [from, to).
// 끝이 없으면 시작 하루짜리로 본다. 여러 날 항목이 창 중간에서 시작해도 잡힌다.
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

// TodoCard is a card in a board's first column, with what the home screen
// needs to show it and to "complete" it (move to the board's last column).
type TodoCard struct {
	Card
	BoardID      int64  `json:"board_id"`
	BoardName    string `json:"board_name"`
	DoneColumnID int64  `json:"done_column_id"`
}

// TodoCards returns cards sitting in the first column (position 0) of every
// board — the kanban convention for "to do" — ordered by due date first, then
// board, then position. The last column (max position) is reported as the
// completion target.
func (s *Store) TodoCards(ctx context.Context, sortBy Sort, order Order) ([]TodoCard, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.column_id, c.title, c.description, c.content, c.position, c.due_at, c.end_at, c.priority, c.assignee_id, c.created_by, c.created_at, c.updated_at, c.recur, c.recur_until, c.recur_parent_id,
		       b.id, b.name,
		       (SELECT id FROM columns d WHERE d.board_id = b.id ORDER BY d.position DESC, d.id DESC LIMIT 1)
		FROM cards c
		JOIN columns col ON col.id = c.column_id
		JOIN boards b ON b.id = col.board_id
		WHERE col.position = 0
		ORDER BY `+orderBy(sortBy, order, "c.")+`, b.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TodoCard{}
	for rows.Next() {
		var t TodoCard
		if err := rows.Scan(&t.ID, &t.ColumnID, &t.Title, &t.Description, &t.Content, &t.Position, &t.DueAt, &t.EndAt, &t.Priority, &t.AssigneeID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
			&t.BoardID, &t.BoardName, &t.DoneColumnID); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// validateRecurInput은 생성 시 반복 규칙을 검사한다. 마감 없는 반복은
// 다음 회차를 계산할 기준이 없으므로 막는다.
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

// fillRecurLabel은 화면에 쓸 설명을 채운다. 규칙 해석은 서버에만 둔다.
func fillRecurLabel(c *Card) {
	if c.Recur != nil {
		c.RecurLabel = RecurLabel(*c.Recur)
	}
}
