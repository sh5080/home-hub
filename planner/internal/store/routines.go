package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Routine 은 주간 반복 항목. WeekdaysMask 는 bit0=월 … bit6=일.
type Routine struct {
	ID           int64   `json:"id"`
	Title        string  `json:"title"`
	WeekdaysMask int     `json:"weekdays_mask"`
	TimeOfDay    *string `json:"time_of_day"`
	AssigneeID   *int64  `json:"assignee_id"`
	Active       bool    `json:"active"`
	Position     int     `json:"position"`
	// CreatedBy 는 0010 이전 루틴은 NULL.
	CreatedBy *int64 `json:"created_by"`
	CreatedAt int64  `json:"created_at"`

	// Populated only by RoutinesForDate.
	CheckedBy *int64 `json:"checked_by"`
	CheckedAt *int64 `json:"checked_at"`
}

// RoutineInput is create/patch input; nil = not provided.
type RoutineInput struct {
	Title        *string
	WeekdaysMask *int
	TimeOfDay    *string // "" clears
	AssigneeID   *int64  // 0 clears
	Active       *bool
}

const allWeekdays = 0x7F

func validTimeOfDay(s string) bool {
	_, err := time.Parse("15:04", s)
	return err == nil && len(s) == 5
}

// CreateRoutine appends a routine.
func (s *Store) CreateRoutine(ctx context.Context, in RoutineInput, by int64) (Routine, error) {
	title := ""
	if in.Title != nil {
		title = strings.TrimSpace(*in.Title)
	}
	if title == "" {
		return Routine{}, invalid("title is empty")
	}
	mask := allWeekdays
	if in.WeekdaysMask != nil {
		mask = *in.WeekdaysMask
	}
	if mask <= 0 || mask > allWeekdays {
		return Routine{}, invalid("weekdays_mask must be 1..127")
	}
	var tod *string
	if in.TimeOfDay != nil && *in.TimeOfDay != "" {
		if !validTimeOfDay(*in.TimeOfDay) {
			return Routine{}, invalid("time_of_day must be HH:MM")
		}
		tod = in.TimeOfDay
	}
	var assignee *int64
	if in.AssigneeID != nil && *in.AssigneeID != 0 {
		assignee = in.AssigneeID
	}
	active := in.Active == nil || *in.Active

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Routine{}, err
	}
	defer tx.Rollback()
	var pos int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position)+1, 0) FROM routines`).Scan(&pos); err != nil {
		return Routine{}, err
	}
	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO routines (title, weekdays_mask, time_of_day, assignee_id, active, position, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, title, mask, tod, assignee, active, pos, by, now)
	if err != nil {
		return Routine{}, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return Routine{}, err
	}
	return Routine{ID: id, Title: title, WeekdaysMask: mask, TimeOfDay: tod, AssigneeID: assignee, Active: active, Position: pos, CreatedBy: &by, CreatedAt: now}, nil
}

// UpdateRoutine patches the named fields.
func (s *Store) UpdateRoutine(ctx context.Context, id int64, in RoutineInput) (Routine, error) {
	sets := []string{}
	args := []any{}
	if in.Title != nil {
		t := strings.TrimSpace(*in.Title)
		if t == "" {
			return Routine{}, invalid("title is empty")
		}
		sets = append(sets, "title=?")
		args = append(args, t)
	}
	if in.WeekdaysMask != nil {
		if *in.WeekdaysMask <= 0 || *in.WeekdaysMask > allWeekdays {
			return Routine{}, invalid("weekdays_mask must be 1..127")
		}
		sets = append(sets, "weekdays_mask=?")
		args = append(args, *in.WeekdaysMask)
	}
	if in.TimeOfDay != nil {
		if *in.TimeOfDay == "" {
			sets = append(sets, "time_of_day=NULL")
		} else {
			if !validTimeOfDay(*in.TimeOfDay) {
				return Routine{}, invalid("time_of_day must be HH:MM")
			}
			sets = append(sets, "time_of_day=?")
			args = append(args, *in.TimeOfDay)
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
	if in.Active != nil {
		sets = append(sets, "active=?")
		args = append(args, *in.Active)
	}
	if len(sets) > 0 {
		args = append(args, id)
		res, err := s.db.ExecContext(ctx, `UPDATE routines SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...)
		if err != nil {
			return Routine{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return Routine{}, ErrNotFound
		}
	}
	return s.GetRoutine(ctx, id)
}

// GetRoutine loads one routine (no check state).
func (s *Store) GetRoutine(ctx context.Context, id int64) (Routine, error) {
	var r Routine
	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, weekdays_mask, time_of_day, assignee_id, active, position, created_by, created_at FROM routines WHERE id=?`, id).
		Scan(&r.ID, &r.Title, &r.WeekdaysMask, &r.TimeOfDay, &r.AssigneeID, &r.Active, &r.Position, &r.CreatedBy, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Routine{}, ErrNotFound
	}
	return r, err
}

// DeleteRoutine removes it (checks cascade) and compacts positions.
func (s *Store) DeleteRoutine(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var pos int
	err = tx.QueryRowContext(ctx, `SELECT position FROM routines WHERE id=?`, id).Scan(&pos)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM routines WHERE id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE routines SET position=position-1 WHERE position>?`, pos); err != nil {
		return err
	}
	return tx.Commit()
}

// ListRoutines returns every routine (active or not) in order, without check state.
func (s *Store) ListRoutines(ctx context.Context) ([]Routine, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, weekdays_mask, time_of_day, assignee_id, active, position, created_by, created_at
		FROM routines ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Routine{}
	for rows.Next() {
		var r Routine
		if err := rows.Scan(&r.ID, &r.Title, &r.WeekdaysMask, &r.TimeOfDay, &r.AssigneeID, &r.Active, &r.Position, &r.CreatedBy, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RoutinesForDate returns active routines scheduled on date (YYYY-MM-DD)
// with that date's check state joined in.
func (s *Store) RoutinesForDate(ctx context.Context, date string) ([]Routine, error) {
	if !validDate(date) {
		return nil, invalid("date must be YYYY-MM-DD")
	}
	d, _ := time.Parse("2006-01-02", date)
	bit := 1 << ((int(d.Weekday()) + 6) % 7) // Go: Sun=0 → ISO: Mon=0

	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, r.title, r.weekdays_mask, r.time_of_day, r.assignee_id, r.active, r.position, r.created_by, r.created_at,
		       c.checked_by, c.checked_at
		FROM routines r
		LEFT JOIN routine_checks c ON c.routine_id = r.id AND c.date = ?
		WHERE r.active = 1 AND (r.weekdays_mask & ?) <> 0
		ORDER BY r.time_of_day IS NULL, r.time_of_day, r.position, r.id`, date, bit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Routine{}
	for rows.Next() {
		var r Routine
		if err := rows.Scan(&r.ID, &r.Title, &r.WeekdaysMask, &r.TimeOfDay, &r.AssigneeID, &r.Active, &r.Position, &r.CreatedBy, &r.CreatedAt,
			&r.CheckedBy, &r.CheckedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RoutineChecksInRange 는 [from, to) 의 (routine_id, date) 체크들.
func (s *Store) RoutineChecksInRange(ctx context.Context, from, to string) (map[int64][]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT routine_id, date FROM routine_checks WHERE date >= ? AND date < ?`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]string{}
	for rows.Next() {
		var id int64
		var date string
		if err := rows.Scan(&id, &date); err != nil {
			return nil, err
		}
		out[id] = append(out[id], date)
	}
	return out, rows.Err()
}

// SetRoutineCheck marks routine id done on date by user. Idempotent.
func (s *Store) SetRoutineCheck(ctx context.Context, id int64, date string, by int64) error {
	if !validDate(date) {
		return invalid("date must be YYYY-MM-DD")
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM routines WHERE id=?`, id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrNotFound
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO routine_checks (routine_id, date, checked_by, checked_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(routine_id, date) DO NOTHING`, id, date, by, time.Now().Unix())
	return err
}

// ClearRoutineCheck unmarks routine id on date. Clearing a missing check is fine.
func (s *Store) ClearRoutineCheck(ctx context.Context, id int64, date string) error {
	if !validDate(date) {
		return invalid("date must be YYYY-MM-DD")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM routine_checks WHERE routine_id=? AND date=?`, id, date)
	return err
}
