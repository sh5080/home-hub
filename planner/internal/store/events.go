package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Event is a calendar entry. Times are floating local strings (see 0001_init.sql).
type Event struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	StartAt     string  `json:"start_at"`
	EndAt       *string `json:"end_at"`
	AllDay      bool    `json:"all_day"`
	Description string  `json:"description"` // content에서 파생한 평문
	Content     *string `json:"content"`     // 권위 있는 본문. 블록 문서 JSON
	CreatedBy   int64   `json:"created_by"`
	CreatedAt   int64   `json:"created_at"`
}

// EventInput is create/patch input; nil = not provided.
type EventInput struct {
	Title   *string
	StartAt *string
	EndAt   *string // "" clears
	AllDay  *bool
	Content *string // 블록 문서 JSON. description은 여기서 파생한다.
}

// validateEventTimes checks the start/end strings against all_day.
func validateEventTimes(start string, end *string, allDay bool) error {
	ok := validDateTime
	what := "YYYY-MM-DDTHH:MM"
	if allDay {
		ok = validDate
		what = "YYYY-MM-DD"
	}
	if !ok(start) {
		return invalid("start_at must be " + what)
	}
	if end != nil && *end != "" {
		if !ok(*end) {
			return invalid("end_at must be " + what)
		}
		if *end < start {
			return invalid("end_at is before start_at")
		}
	}
	return nil
}

// CreateEvent inserts an event.
func (s *Store) CreateEvent(ctx context.Context, in EventInput, by int64) (Event, error) {
	title := ""
	if in.Title != nil {
		title = strings.TrimSpace(*in.Title)
	}
	if title == "" {
		return Event{}, invalid("title is empty")
	}
	if in.StartAt == nil {
		return Event{}, invalid("start_at is required")
	}
	allDay := in.AllDay != nil && *in.AllDay
	if err := validateEventTimes(*in.StartAt, in.EndAt, allDay); err != nil {
		return Event{}, err
	}
	var end *string
	if in.EndAt != nil && *in.EndAt != "" {
		end = in.EndAt
	}
	var content *string
	desc := ""
	if in.Content != nil {
		plain, err := ValidateContent(*in.Content)
		if err != nil {
			return Event{}, err
		}
		content, desc = in.Content, plain
	}
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO events (title, start_at, end_at, all_day, description, content, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		title, *in.StartAt, end, allDay, desc, content, by, now)
	if err != nil {
		return Event{}, err
	}
	id, _ := res.LastInsertId()
	return Event{ID: id, Title: title, StartAt: *in.StartAt, EndAt: end, AllDay: allDay,
		Description: desc, Content: content, CreatedBy: by, CreatedAt: now}, nil
}

// UpdateEvent patches the named fields, re-validating times against the
// resulting all_day value.
func (s *Store) UpdateEvent(ctx context.Context, id int64, in EventInput) (Event, error) {
	cur, err := s.GetEvent(ctx, id)
	if err != nil {
		return Event{}, err
	}
	if in.Title != nil {
		t := strings.TrimSpace(*in.Title)
		if t == "" {
			return Event{}, invalid("title is empty")
		}
		cur.Title = t
	}
	if in.AllDay != nil {
		cur.AllDay = *in.AllDay
	}
	if in.StartAt != nil {
		cur.StartAt = *in.StartAt
	}
	if in.EndAt != nil {
		if *in.EndAt == "" {
			cur.EndAt = nil
		} else {
			cur.EndAt = in.EndAt
		}
	}
	if in.Content != nil {
		plain, err := ValidateContent(*in.Content)
		if err != nil {
			return Event{}, err
		}
		cur.Content, cur.Description = in.Content, plain
	}
	if err := validateEventTimes(cur.StartAt, cur.EndAt, cur.AllDay); err != nil {
		return Event{}, err
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE events SET title=?, start_at=?, end_at=?, all_day=?, description=?, content=? WHERE id=?`,
		cur.Title, cur.StartAt, cur.EndAt, cur.AllDay, cur.Description, cur.Content, id); err != nil {
		return Event{}, err
	}
	return cur, nil
}

// GetEvent loads one event.
func (s *Store) GetEvent(ctx context.Context, id int64) (Event, error) {
	var e Event
	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, start_at, end_at, all_day, description, content, created_by, created_at
		FROM events WHERE id=?`, id).
		Scan(&e.ID, &e.Title, &e.StartAt, &e.EndAt, &e.AllDay, &e.Description, &e.Content, &e.CreatedBy, &e.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Event{}, ErrNotFound
	}
	return e, err
}

// DeleteEvent removes an event.
func (s *Store) DeleteEvent(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListEvents returns events overlapping [from, to). Both bounds are date or
// datetime strings; lexicographic comparison is chronological for this format.
// An event with no end is treated as a point at start_at.
func (s *Store) ListEvents(ctx context.Context, from, to string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, start_at, end_at, all_day, description, content, created_by, created_at
		FROM events WHERE start_at < ? AND COALESCE(end_at, start_at) >= ?
		ORDER BY start_at, id`, to, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Title, &e.StartAt, &e.EndAt, &e.AllDay, &e.Description, &e.Content, &e.CreatedBy, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
