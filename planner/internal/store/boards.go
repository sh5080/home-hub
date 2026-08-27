package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Board is a kanban board; everyone in the family sees every board.
type Board struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedBy *int64 `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
	CardCount int    `json:"card_count"`
}

// Column is a lane on a board.
type Column struct {
	ID       int64  `json:"id"`
	BoardID  int64  `json:"board_id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	Cards    []Card `json:"cards"`
}

// Card is a task on a column.
type Card struct {
	ID          int64   `json:"id"`
	ColumnID    int64   `json:"column_id"`
	Title       string  `json:"title"`
	Description string  `json:"description"` // content에서 파생한 평문 (미리보기·검색용)
	Content     *string `json:"content"`     // 권위 있는 본문. 블록 문서 JSON
	Position    int     `json:"position"`
	DueAt       *string `json:"due_at"`   // 'YYYY-MM-DD' 또는 'YYYY-MM-DDTHH:MM'
	EndAt       *string `json:"end_at"`   // 여러 날 항목의 끝. 없으면 하루짜리
	Priority    int     `json:"priority"` // 0=없음, 1~3
	AssigneeID  *int64  `json:"assignee_id"`
	CreatedBy   int64   `json:"created_by"`
	CreatedAt   int64   `json:"created_at"`
	UpdatedAt   int64   `json:"updated_at"`
}

// BoardDetail is what the board page renders in one request.
type BoardDetail struct {
	Board   Board    `json:"board"`
	Columns []Column `json:"columns"`
}

// ListBoards returns all boards with their card counts.
func (s *Store) ListBoards(ctx context.Context) ([]Board, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.id, b.name, b.created_by, b.created_at,
		       (SELECT count(*) FROM cards c JOIN columns col ON col.id = c.column_id WHERE col.board_id = b.id)
		FROM boards b ORDER BY b.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Board{}
	for rows.Next() {
		var b Board
		if err := rows.Scan(&b.ID, &b.Name, &b.CreatedBy, &b.CreatedAt, &b.CardCount); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// CreateBoard makes a board with the standard three columns.
func (s *Store) CreateBoard(ctx context.Context, name string, by int64) (Board, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Board{}, invalid("name is empty")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Board{}, err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `INSERT INTO boards (name, created_by, created_at) VALUES (?, ?, ?)`, name, by, now)
	if err != nil {
		return Board{}, err
	}
	id, _ := res.LastInsertId()
	for i, col := range []string{"할 일", "진행 중", "완료"} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO columns (board_id, name, position) VALUES (?, ?, ?)`, id, col, i); err != nil {
			return Board{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Board{}, err
	}
	return Board{ID: id, Name: name, CreatedBy: &by, CreatedAt: now}, nil
}

// RenameBoard updates the name.
func (s *Store) RenameBoard(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return invalid("name is empty")
	}
	res, err := s.db.ExecContext(ctx, `UPDATE boards SET name=? WHERE id=?`, name, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteBoard removes the board; columns and cards cascade.
func (s *Store) DeleteBoard(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM boards WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetBoard loads the board, its columns in order, and each column's cards
// ordered by sort (SortManual = 드래그로 정한 순서).
func (s *Store) GetBoard(ctx context.Context, id int64, sortBy Sort, order Order) (BoardDetail, error) {
	var d BoardDetail
	err := s.db.QueryRowContext(ctx, `SELECT id, name, created_by, created_at FROM boards WHERE id=?`, id).
		Scan(&d.Board.ID, &d.Board.Name, &d.Board.CreatedBy, &d.Board.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, err
	}

	// Two flat queries, assembled in Go: cheaper than a JOIN that repeats
	// column rows per card, and the single connection means we must not nest
	// an open rows cursor inside another query anyway.
	cols, err := s.db.QueryContext(ctx, `SELECT id, board_id, name, position FROM columns WHERE board_id=? ORDER BY position, id`, id)
	if err != nil {
		return d, err
	}
	byID := map[int64]int{}
	for cols.Next() {
		var c Column
		if err := cols.Scan(&c.ID, &c.BoardID, &c.Name, &c.Position); err != nil {
			cols.Close()
			return d, err
		}
		c.Cards = []Card{}
		byID[c.ID] = len(d.Columns)
		d.Columns = append(d.Columns, c)
	}
	cols.Close()
	if d.Columns == nil {
		d.Columns = []Column{}
	}

	cards, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.column_id, c.title, c.description, c.content, c.position, c.due_at, c.end_at, c.priority, c.assignee_id, c.created_by, c.created_at, c.updated_at
		FROM cards c JOIN columns col ON col.id = c.column_id
		WHERE col.board_id=? ORDER BY c.column_id, `+orderBy(sortBy, order, "c."), id)
	if err != nil {
		return d, err
	}
	defer cards.Close()
	for cards.Next() {
		var c Card
		if err := cards.Scan(&c.ID, &c.ColumnID, &c.Title, &c.Description, &c.Content, &c.Position, &c.DueAt, &c.EndAt, &c.Priority, &c.AssigneeID, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return d, err
		}
		if i, ok := byID[c.ColumnID]; ok {
			d.Columns[i].Cards = append(d.Columns[i].Cards, c)
		}
	}
	return d, cards.Err()
}

// --- columns ---

// AddColumn appends a column to the board.
func (s *Store) AddColumn(ctx context.Context, boardID int64, name string) (Column, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Column{}, invalid("name is empty")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Column{}, err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM boards WHERE id=?`, boardID).Scan(&exists); err != nil {
		return Column{}, err
	}
	if exists == 0 {
		return Column{}, ErrNotFound
	}
	var pos int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position)+1, 0) FROM columns WHERE board_id=?`, boardID).Scan(&pos); err != nil {
		return Column{}, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO columns (board_id, name, position) VALUES (?, ?, ?)`, boardID, name, pos)
	if err != nil {
		return Column{}, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return Column{}, err
	}
	return Column{ID: id, BoardID: boardID, Name: name, Position: pos, Cards: []Card{}}, nil
}

// RenameColumn updates the name.
func (s *Store) RenameColumn(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return invalid("name is empty")
	}
	res, err := s.db.ExecContext(ctx, `UPDATE columns SET name=? WHERE id=?`, name, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteColumn removes the column and its cards (cascade), then closes the
// positional gap among the board's remaining columns.
func (s *Store) DeleteColumn(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var boardID int64
	var pos int
	err = tx.QueryRowContext(ctx, `SELECT board_id, position FROM columns WHERE id=?`, id).Scan(&boardID, &pos)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM columns WHERE id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE columns SET position=position-1 WHERE board_id=? AND position>?`, boardID, pos); err != nil {
		return err
	}
	return tx.Commit()
}
