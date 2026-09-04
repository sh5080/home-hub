package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// DiaryRef는 글이 가리키는 대상 하나다.
type DiaryRef struct {
	Kind    string `json:"kind"` // 'card' | 'routine' | 'babyfood'
	RefID   *int64 `json:"ref_id"`
	RefDate string `json:"ref_date"`
	ChildID *int64 `json:"child_id"`
	// Label 은 걸던 때의 이름. 대상이 바뀌거나 지워져도 그대로 둔다.
	Label string `json:"label"`
	// Gone: 대상이 지금은 없다.
	Gone bool `json:"gone"`
}

// DiaryEntry는 일기 한 장이다.
type DiaryEntry struct {
	ID    int64  `json:"id"`
	Date  string `json:"date"`
	Title string `json:"title"`
	// Content 는 목록 조회에서는 비어 있다(Plain 으로 미리보기).
	Content   *string    `json:"content"`
	Plain     string     `json:"plain"`
	CreatedBy *int64     `json:"created_by"`
	CreatedAt int64      `json:"created_at"`
	UpdatedAt int64      `json:"updated_at"`
	Refs      []DiaryRef `json:"refs"`
	// Photos 는 본문 사진 앞 몇 장(목록용).
	Photos []string `json:"photos"`
	// Source 는 가져온 글의 출처('babytime:181_1'). 직접 쓴 글은 nil.
	Source *string `json:"source"`
}

const diaryPhotoPreview = 4

const diaryPreview = 140

// DiaryPage 는 목록 조건. 더 보기는 offset 이 아니라 (date, id) 커서로 한다
// (최신순이라 위에 새 글이 끼면 offset 은 겹치거나 건너뛴다).
type DiaryPage struct {
	From, To   string
	Limit      int
	BeforeDate string // 이 (날짜, id) 보다 오래된 글부터
	BeforeID   int64
}

// DiaryList는 기간 안의 글을 최신순으로 준다. from/to 가 비면 전체다.
func (s *Store) DiaryList(ctx context.Context, from, to string, limit int) ([]DiaryEntry, error) {
	return s.DiaryListPage(ctx, DiaryPage{From: from, To: to, Limit: limit})
}

// DiaryListPage 는 커서까지 받는 목록 조회다.
func (s *Store) DiaryListPage(ctx context.Context, pg DiaryPage) ([]DiaryEntry, error) {
	limit := pg.Limit
	if limit <= 0 || limit > 200 {
		limit = 60
	}
	from, to := pg.From, pg.To
	where, args := []string{}, []any{}
	if from != "" {
		where, args = append(where, "date >= ?"), append(args, from)
	}
	if to != "" {
		where, args = append(where, "date <= ?"), append(args, to)
	}
	if pg.BeforeDate != "" && pg.BeforeID > 0 {
		where = append(where, "(date < ? OR (date = ? AND id < ?))")
		args = append(args, pg.BeforeDate, pg.BeforeDate, pg.BeforeID)
	}
	q := `SELECT id, date, title, plain, photos, source, created_by, created_at, updated_at FROM diary`
	if len(where) > 0 {
		// 조건절은 상수 조합, 값은 전부 ?.
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY date DESC, id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	out := []DiaryEntry{}
	ids := []int64{}
	for rows.Next() {
		var e DiaryEntry
		var photos string
		if err := rows.Scan(&e.ID, &e.Date, &e.Title, &e.Plain, &photos, &e.Source, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		e.Plain = cutRunes(e.Plain, diaryPreview)
		e.Refs = []DiaryRef{}
		e.Photos = parsePhotos(photos)
		out = append(out, e)
		ids = append(ids, e.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	// 참조는 한 번에 읽는다(커넥션 하나).
	refs, err := s.diaryRefs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if r := refs[out[i].ID]; r != nil {
			out[i].Refs = r
		}
	}
	return out, nil
}

// cutRunes 는 글자 단위로 자른다(바이트로 자르면 한글이 깨진다).
func cutRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}

// DiaryGet은 본문까지 준다.
func (s *Store) DiaryGet(ctx context.Context, id int64) (DiaryEntry, error) {
	var e DiaryEntry
	var photos string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, date, title, content, plain, photos, source, created_by, created_at, updated_at
		   FROM diary WHERE id=?`, id).
		Scan(&e.ID, &e.Date, &e.Title, &e.Content, &e.Plain, &photos, &e.Source, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt)
	e.Photos = parsePhotos(photos)
	if errors.Is(err, sql.ErrNoRows) {
		return DiaryEntry{}, ErrNotFound
	}
	if err != nil {
		return DiaryEntry{}, err
	}
	refs, err := s.diaryRefs(ctx, []int64{id})
	if err != nil {
		return DiaryEntry{}, err
	}
	if e.Refs = refs[id]; e.Refs == nil {
		e.Refs = []DiaryRef{}
	}
	return e, nil
}

// diaryRefs 는 여러 글의 참조를 한 번에 읽고 대상이 남아 있는지 본다.
// 이유식 참조는 날짜라 사라지지 않는다.
func (s *Store) diaryRefs(ctx context.Context, ids []int64) (map[int64][]DiaryRef, error) {
	if len(ids) == 0 {
		return map[int64][]DiaryRef{}, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, v := range ids {
		args[i] = v
	}
	// 자리표시자 개수만 이어 붙인다. 값은 전부 ?.
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.entry_id, r.kind, r.ref_id, r.ref_date, r.child_id, r.label,
		        CASE r.kind
		          WHEN 'card'    THEN (SELECT count(*) FROM cards    c WHERE c.id = r.ref_id)
		          WHEN 'routine' THEN (SELECT count(*) FROM routines t WHERE t.id = r.ref_id)
		          ELSE 1 END
		   FROM diary_refs r WHERE r.entry_id IN (`+ph+`) ORDER BY r.entry_id, r.pos`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]DiaryRef{}
	for rows.Next() {
		var id, alive int64
		var r DiaryRef
		var date sql.NullString
		var child sql.NullInt64
		if err := rows.Scan(&id, &r.Kind, &r.RefID, &date, &child, &r.Label, &alive); err != nil {
			return nil, err
		}
		r.RefDate = date.String
		if child.Valid {
			v := child.Int64
			r.ChildID = &v
		}
		r.Gone = alive == 0
		out[id] = append(out[id], r)
	}
	return out, rows.Err()
}

// DiaryDates 는 기간 안에 글이 있는 날짜(캘린더 점).
func (s *Store) DiaryDates(ctx context.Context, from, to string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT date FROM diary WHERE date >= ? AND date <= ? ORDER BY date`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DiaryInput은 부분 수정이다. nil이면 안 건드린다.
type DiaryInput struct {
	Date    *string
	Title   *string
	Content *string
	// Refs 가 nil 이 아니면 통째로 바꾼다.
	Refs *[]DiaryRef
}

const maxDiaryRefs = 20

// DiaryCreate 는 빈 글을 만든다.
func (s *Store) DiaryCreate(ctx context.Context, date, title string, userID int64) (DiaryEntry, error) {
	if !validDate(date) {
		return DiaryEntry{}, invalid("날짜가 올바르지 않아요")
	}
	title = strings.TrimSpace(title)
	if len([]rune(title)) > 200 {
		return DiaryEntry{}, invalid("제목이 너무 길어요")
	}
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO diary (date, title, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		date, title, userID, now, now)
	if err != nil {
		return DiaryEntry{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return DiaryEntry{}, err
	}
	return s.DiaryGet(ctx, id)
}

// DiaryUpdate는 제목·날짜·본문·참조를 고친다.
func (s *Store) DiaryUpdate(ctx context.Context, id int64, in DiaryInput) (DiaryEntry, error) {
	sets, args := []string{}, []any{}
	if in.Date != nil {
		if !validDate(*in.Date) {
			return DiaryEntry{}, invalid("날짜가 올바르지 않아요")
		}
		sets, args = append(sets, "date=?"), append(args, *in.Date)
	}
	if in.Title != nil {
		t := strings.TrimSpace(*in.Title)
		if len([]rune(t)) > 200 {
			return DiaryEntry{}, invalid("제목이 너무 길어요")
		}
		sets, args = append(sets, "title=?"), append(args, t)
	}
	if in.Content != nil {
		plain, err := ValidateContent(*in.Content)
		if err != nil {
			return DiaryEntry{}, err
		}
		sets = append(sets, "content=?", "plain=?", "photos=?")
		args = append(args, *in.Content, plain, encodePhotos(*in.Content))
	}
	if in.Refs != nil && len(*in.Refs) > maxDiaryRefs {
		return DiaryEntry{}, invalid("참조는 20개까지예요")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DiaryEntry{}, err
	}
	defer tx.Rollback()

	if len(sets) > 0 {
		sets = append(sets, "updated_at=?")
		args = append(args, time.Now().Unix(), id)
		// sets 는 상수 문자열뿐, 값은 ?.
		res, err := tx.ExecContext(ctx, "UPDATE diary SET "+strings.Join(sets, ", ")+" WHERE id=?", args...)
		if err != nil {
			return DiaryEntry{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return DiaryEntry{}, ErrNotFound
		}
	}
	if in.Refs != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM diary_refs WHERE entry_id=?`, id); err != nil {
			return DiaryEntry{}, err
		}
		for i, r := range *in.Refs {
			label := strings.TrimSpace(r.Label)
			if label == "" {
				continue
			}
			if len([]rune(label)) > 200 {
				label = string([]rune(label)[:200])
			}
			switch r.Kind {
			case "card", "routine", "babyfood":
			default:
				return DiaryEntry{}, invalid("모르는 참조예요")
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO diary_refs (entry_id, pos, kind, ref_id, ref_date, child_id, label)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				id, i, r.Kind, r.RefID, nullStr(r.RefDate), r.ChildID, label); err != nil {
				return DiaryEntry{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return DiaryEntry{}, err
	}
	return s.DiaryGet(ctx, id)
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// DiaryDelete는 글을 지운다. 참조는 CASCADE 로 따라간다.
func (s *Store) DiaryDelete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM diary WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SearchDiary 는 제목·평문에서 찾는다(LIKE, 낱말 AND).
func (s *Store) SearchDiary(ctx context.Context, q string) ([]DiaryEntry, error) {
	terms := strings.Fields(q)
	if len(terms) == 0 {
		return []DiaryEntry{}, nil
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
		where.WriteString(`(title LIKE ? ESCAPE '\' COLLATE NOCASE OR plain LIKE ? ESCAPE '\' COLLATE NOCASE)`)
		pat := "%" + likeEscape(t) + "%"
		args = append(args, pat, pat)
	}
	args = append(args, searchLimit)
	// 조건절은 상수 조합, 값은 전부 ?.
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, date, title, plain, photos, source, created_by, created_at, updated_at
		  FROM diary WHERE `+where.String()+`
		 ORDER BY date DESC, id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DiaryEntry{}
	for rows.Next() {
		var e DiaryEntry
		var photos string
		if err := rows.Scan(&e.ID, &e.Date, &e.Title, &e.Plain, &photos, &e.Source, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.Plain = cutRunes(e.Plain, diaryPreview)
		e.Refs = []DiaryRef{}
		e.Photos = parsePhotos(photos)
		out = append(out, e)
	}
	return out, rows.Err()
}

func parsePhotos(raw string) []string {
	out := []string{}
	if raw == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func encodePhotos(content string) string {
	b, _ := json.Marshal(ContentPhotos(content, diaryPhotoPreview))
	return string(b)
}

// DiaryImportEntry 는 가져올 글 한 장. Content 는 이미 블록 형식이어야 한다.
type DiaryImportEntry struct {
	Source  string // 'babytime:181_1'
	Date    string
	Title   string
	Content string
}

// DiaryImport 는 출처가 같은 글을 건너뛰며 넣는다. (넣은 수, 이미 있던 수).
// diary_source 가 부분 인덱스라 ON CONFLICT 에도 같은 WHERE 를 적어야 한다.
func (s *Store) DiaryImport(ctx context.Context, entries []DiaryImportEntry, userID int64) (added, skipped int, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	for _, e := range entries {
		if e.Source == "" || !validDate(e.Date) {
			return 0, 0, invalid("출처나 날짜가 비었어요: " + e.Source)
		}
		plain, err := ValidateContent(e.Content)
		if err != nil {
			return 0, 0, err
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO diary (date, title, content, plain, photos, source, created_by, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(source) WHERE source IS NOT NULL DO NOTHING`,
			e.Date, strings.TrimSpace(e.Title), e.Content, plain, encodePhotos(e.Content), e.Source, userID, now, now)
		if err != nil {
			return 0, 0, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			skipped++
		} else {
			added++
		}
	}
	return added, skipped, tx.Commit()
}
