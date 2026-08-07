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
	Title       *string
	Description *string
	DueDate     *string // "" clears
	AssigneeID  *int64  // 0 clears
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
	desc := ""
	if in.Description != nil {
		desc = *in.Description
	}
	var due *string
	if in.DueDate != nil && *in.DueDate != "" {
		if !validDate(*in.DueDate) {
			return Card{}, invalid("due_date must be YYYY-MM-DD")
		}
		due = in.DueDate
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
		INSERT INTO cards (column_id, title, description, position, due_date, assignee_id, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		columnID, title, desc, pos, due, assignee, by, now, now)
	if err != nil {
		return Card{}, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return Card{}, err
	}
	return Card{ID: id, ColumnID: columnID, Title: title, Description: desc, Position: pos,
		DueDate: due, AssigneeID: assignee, CreatedBy: by, CreatedAt: now, UpdatedAt: now}, nil
}

// UpdateCard patches the named fields.
func (s *Store) UpdateCard(ctx context.Context, id int64, in CardInput) (Card, error) {
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
	if in.Description != nil {
		sets = append(sets, "description=?")
		args = append(args, *in.Description)
	}
	if in.DueDate != nil {
		if *in.DueDate == "" {
			sets = append(sets, "due_date=NULL")
		} else {
			if !validDate(*in.DueDate) {
				return Card{}, invalid("due_date must be YYYY-MM-DD")
			}
			sets = append(sets, "due_date=?")
			args = append(args, *in.DueDate)
		}
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
		SELECT id, column_id, title, description, position, due_date, assignee_id, created_by, created_at, updated_at
		FROM cards WHERE id=?`, id).
		Scan(&c.ID, &c.ColumnID, &c.Title, &c.Description, &c.Position, &c.DueDate, &c.AssigneeID, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, ErrNotFound
	}
	return c, err
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
	if err := tx.Commit(); err != nil {
		return Card{}, err
	}
	return s.GetCard(ctx, id)
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

// DueCards returns cards with a due date in [from, to).
func (s *Store) DueCards(ctx context.Context, from, to string) ([]Card, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, column_id, title, description, position, due_date, assignee_id, created_by, created_at, updated_at
		FROM cards WHERE due_date >= ? AND due_date < ? ORDER BY due_date, id`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Card{}
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ID, &c.ColumnID, &c.Title, &c.Description, &c.Position, &c.DueDate, &c.AssigneeID, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
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
func (s *Store) TodoCards(ctx context.Context) ([]TodoCard, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.column_id, c.title, c.description, c.position, c.due_date, c.assignee_id, c.created_by, c.created_at, c.updated_at,
		       b.id, b.name,
		       (SELECT id FROM columns d WHERE d.board_id = b.id ORDER BY d.position DESC, d.id DESC LIMIT 1)
		FROM cards c
		JOIN columns col ON col.id = c.column_id
		JOIN boards b ON b.id = col.board_id
		WHERE col.position = 0
		ORDER BY c.due_date IS NULL, c.due_date, b.id, c.position, c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TodoCard{}
	for rows.Next() {
		var t TodoCard
		if err := rows.Scan(&t.ID, &t.ColumnID, &t.Title, &t.Description, &t.Position, &t.DueDate, &t.AssigneeID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
			&t.BoardID, &t.BoardName, &t.DoneColumnID); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
