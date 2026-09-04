package store

import (
	"context"
	"strings"
)

// 검색은 FTS5 가 아니라 LIKE 다. FTS5 기본 토크나이저는 '고기'로 '소고기'를 못 찾고,
// trigram 은 3자 미만을 못 받고, bigram 은 C API 가 필요해 CGO_ENABLED=0 이 깨진다.

// SearchResult 는 검색 한 줄(보드·칸 이름 포함).
type SearchResult struct {
	Card
	BoardID    int64  `json:"board_id"`
	BoardName  string `json:"board_name"`
	ColumnName string `json:"column_name"`
	// Snippet 은 본문에서 검색어 주변. 제목에만 맞으면 비어 있다.
	Snippet string `json:"snippet"`
}

const searchLimit = 50

// likeEscape 는 %·_ 를 리터럴로 만든다(안 하면 '_' 가 전부 맞는다).
func likeEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// SearchCards 는 제목·본문에서 낱말을 모두 포함하는 카드(낱말 AND).
func (s *Store) SearchCards(ctx context.Context, q string) ([]SearchResult, error) {
	terms := strings.Fields(q)
	if len(terms) == 0 {
		return []SearchResult{}, nil
	}
	if len(terms) > 8 {
		terms = terms[:8]
	}

	var where strings.Builder
	args := []any{}
	for i, t := range terms {
		if i > 0 {
			where.WriteString(" AND ")
		}
		// COLLATE NOCASE 는 ASCII 만 접는다.
		where.WriteString(`(c.title LIKE ? ESCAPE '\' COLLATE NOCASE OR c.description LIKE ? ESCAPE '\' COLLATE NOCASE)`)
		pat := "%" + likeEscape(t) + "%"
		args = append(args, pat, pat)
	}
	args = append(args, searchLimit)

	// 조건절은 상수 조합, 값은 전부 ?.
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.column_id, c.title, c.description, c.content, c.position,
		       c.due_at, c.end_at, c.priority, c.assignee_id, c.created_by, c.created_at, c.updated_at,
		       c.recur, c.recur_until, c.recur_parent_id,
		       b.id, b.name, col.name
		  FROM cards c
		  JOIN columns col ON col.id = c.column_id
		  JOIN boards  b   ON b.id  = col.board_id
		 WHERE `+where.String()+`
		 ORDER BY c.updated_at DESC
		 LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []SearchResult{}
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.ID, &r.ColumnID, &r.Title, &r.Description, &r.Content, &r.Position,
			&r.DueAt, &r.EndAt, &r.Priority, &r.AssigneeID, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
			&r.Recur, &r.RecurUntil, &r.RecurParentID,
			&r.BoardID, &r.BoardName, &r.ColumnName); err != nil {
			return nil, err
		}
		// 목록엔 본문을 싣지 않는다.
		r.Content = nil
		r.Snippet = snippet(r.Description, terms)
		out = append(out, r)
	}
	return out, rows.Err()
}

// snippet은 본문에서 첫 번째로 맞은 낱말 주변을 잘라낸다.
func snippet(text string, terms []string) string {
	if text == "" {
		return ""
	}
	lower := strings.ToLower(text)
	at := -1
	for _, t := range terms {
		if i := strings.Index(lower, strings.ToLower(t)); i >= 0 && (at < 0 || i < at) {
			at = i
		}
	}
	if at < 0 {
		return ""
	}
	// 글자 단위로 자른다(한글 깨짐 방지).
	r := []rune(text[:at])
	rest := []rune(text[at:])
	const before, after = 12, 48
	head := ""
	if len(r) > before {
		head = "…"
		r = r[len(r)-before:]
	}
	tail := ""
	if len(rest) > after {
		rest = rest[:after]
		tail = "…"
	}
	return head + strings.TrimSpace(string(r)+string(rest)) + tail
}

// SearchAll 은 카드·루틴·재료·일기를 한 번에 찾는다.
type SearchAll struct {
	Cards    []SearchResult `json:"cards"`
	Routines []Routine      `json:"routines"`
	Foods    []BFFood       `json:"foods"`
	Diary    []DiaryEntry   `json:"diary"`
}

func (s *Store) SearchAll(ctx context.Context, q string) (SearchAll, error) {
	var out SearchAll
	if len(strings.Fields(q)) == 0 {
		out.Cards, out.Routines, out.Foods, out.Diary = []SearchResult{}, []Routine{}, []BFFood{}, []DiaryEntry{}
		return out, nil
	}
	var err error
	if out.Cards, err = s.SearchCards(ctx, q); err != nil {
		return SearchAll{}, err
	}
	if out.Routines, err = s.SearchRoutines(ctx, q); err != nil {
		return SearchAll{}, err
	}
	if out.Foods, err = s.SearchFoods(ctx, q); err != nil {
		return SearchAll{}, err
	}
	if out.Diary, err = s.SearchDiary(ctx, q); err != nil {
		return SearchAll{}, err
	}
	return out, nil
}

// SearchRoutines는 루틴 제목에서 찾는다. 루틴엔 본문이 없다.
func (s *Store) SearchRoutines(ctx context.Context, q string) ([]Routine, error) {
	terms := strings.Fields(q)
	if len(terms) == 0 {
		return []Routine{}, nil
	}
	if len(terms) > 8 {
		terms = terms[:8]
	}
	var where strings.Builder
	args := []any{}
	for i, t := range terms {
		if i > 0 {
			where.WriteString(" AND ")
		}
		where.WriteString(`title LIKE ? ESCAPE '\' COLLATE NOCASE`)
		args = append(args, "%"+likeEscape(t)+"%")
	}
	args = append(args, searchLimit)
	// 조건절은 상수 조합, 값은 전부 ?.
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, weekdays_mask, time_of_day, assignee_id, active, position, created_at
		  FROM routines
		 WHERE `+where.String()+`
		 ORDER BY active DESC, position, id
		 LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Routine{}
	for rows.Next() {
		var r Routine
		var active int
		if err := rows.Scan(&r.ID, &r.Title, &r.WeekdaysMask, &r.TimeOfDay, &r.AssigneeID, &active, &r.Position, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Active = active != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// SearchFoods 는 재료를 이름으로 찾는다.
func (s *Store) SearchFoods(ctx context.Context, q string) ([]BFFood, error) {
	terms := strings.Fields(q)
	if len(terms) == 0 {
		return []BFFood{}, nil
	}
	// 아이가 여럿이면 전원의 재료에서 찾는다.
	kids, err := s.BFChildren(ctx)
	if err != nil {
		return nil, err
	}
	var all []BFFood
	for _, k := range kids {
		if k.BirthDate == "" {
			continue
		}
		fs, err := s.BFFoods(ctx, k.UserID)
		if err != nil {
			return nil, err
		}
		for i := range fs {
			fs[i].ChildID, fs[i].ChildName = k.UserID, k.Name
		}
		all = append(all, fs...)
	}
	out := []BFFood{}
	for _, f := range all {
		hit := true
		for _, t := range terms {
			if !strings.Contains(strings.ToLower(f.Name), strings.ToLower(t)) {
				hit = false
				break
			}
		}
		if hit {
			out = append(out, f)
		}
		if len(out) >= searchLimit {
			break
		}
	}
	return out, nil
}
