package store

import (
	"context"
	"strings"
)

// 카드 검색.
//
// FTS5가 아니라 LIKE다. 이유는 한국어와 데이터 크기 둘 다다.
//
//   - FTS5 기본 토크나이저는 공백 단위라 '고기'로 '소고기'를 못 찾는다.
//     trigram 토크나이저는 3자 미만 질의를 아예 못 받는데, 한국어에서 두 글자
//     검색은 가장 흔한 형태다. bigram 토크나이저는 FTS5에 없고, 직접 등록하려면
//     C API가 필요해 CGO_ENABLED=0 크로스컴파일이 깨진다.
//   - 실제 데이터는 카드 수십 장에 검색 대상 텍스트가 수 KB다. 실측하면 이보다
//     1,000배 큰 8MB에서도 최악의 전체 스캔이 100ms 아래였다.
//
// 카드가 수천 장이 되면 여기만 bigram 인덱스로 갈아끼우면 된다. 검색 지식을
// 이 함수 하나에 가둬두는 이유다.

// SearchResult는 검색 한 줄이다. 어느 보드의 어느 칸에 있는지까지 준다 —
// 제목만 보여주면 찾아놓고도 어디로 가야 할지 모른다.
type SearchResult struct {
	Card
	BoardID    int64  `json:"board_id"`
	BoardName  string `json:"board_name"`
	ColumnName string `json:"column_name"`
	// Snippet은 본문에서 검색어 주변을 잘라낸 것. 제목에만 맞으면 비어 있다.
	Snippet string `json:"snippet"`
}

const searchLimit = 50

// likeEscape는 사용자가 친 %와 _를 리터럴로 만든다. 이걸 안 하면 '_'를
// 검색했을 때 모든 카드가 나온다.
func likeEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// SearchCards는 제목과 본문 평문에서 낱말을 모두 포함하는 카드를 찾는다.
//
// 공백으로 나눈 낱말은 AND다. "이유식 장보기"가 "장보기 (이유식)"을 찾아야
// 하는데, 통째로 비교하면 어순이 다르다는 이유로 놓친다.
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
		// COLLATE NOCASE는 ASCII만 접는다 — 한국어엔 대소문자가 없으니
		// 영어 제목을 위한 것이고, 그게 필요한 전부다.
		where.WriteString(`(c.title LIKE ? ESCAPE '\' COLLATE NOCASE OR c.description LIKE ? ESCAPE '\' COLLATE NOCASE)`)
		pat := "%" + likeEscape(t) + "%"
		args = append(args, pat, pat)
	}
	args = append(args, searchLimit)

	// 조건절은 위에서 만든 상수 문자열의 조합이고 값은 전부 ? 로 들어간다.
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
		// 목록에 본문 전체를 실어 보내지 않는다 — 검색 결과 50개면 그대로
		// 수백 KB가 되고, 화면에 쓰지도 않는다.
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
	// 바이트가 아니라 글자로 자른다 — 한글 가운데를 자르면 깨진다.
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
