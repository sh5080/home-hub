// Package notion imports a Notion "Markdown & CSV" workspace export.
//
// 이 패키지는 특정 export를 보고 썼다. 노션의 CSV는 한국어 열 이름과 한국어
// 날짜 문자열을 그대로 내보내고, 페이지 본문은 제목 + "키: 값" 속성 블록 +
// 실제 본문 순서로 된 마크다운이다. 그 구조에 맞춰 파싱한다.
package notion

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Item은 CSV 한 행 + (있으면) 같은 이름의 페이지 본문이다.
type Item struct {
	Title    string
	Kind     Kind
	Start    string // 'YYYY-MM-DD' 또는 'YYYY-MM-DDTHH:MM'
	End      string // 범위일 때만
	AllDay   bool
	Category string // 분류 (관계 속성에서 이름만)
	Priority int    // 중요도 별 개수
	Note     string // 내용 열
	Body     string // 페이지 마크다운 본문 (속성 블록 제외)
}

// Kind는 항목이 캘린더 일정인지 할 일인지다. 노션의 '선택' 열에서 온다.
type Kind int

const (
	KindTodo Kind = iota
	KindEvent
)

// Export는 파싱 결과다.
type Export struct {
	Items []Item
	// Skipped는 가져오지 않은 것들의 사유별 개수 — dry-run이 보여준다.
	Skipped map[string]int
	// Images는 export에 들어 있던 이미지 파일 이름이다. 지금은 가져오지
	// 않지만, 몇 개가 남는지 알려주려고 센다.
	Images []string
}

// 노션 export의 CSV 열 이름. 다른 워크스페이스는 다를 수 있어 한 곳에 모은다.
const (
	colName     = "이름"
	colDate     = "날짜"
	colNote     = "내용"
	colCategory = "분류"
	colSelect   = "선택"
	colPriority = "중요도"
)

const eventSelect = "📆" // '선택' 값이 이걸 포함하면 캘린더 일정

// utf8BOM은 노션 export의 CSV·MD 앞에 붙는 바이트 순서 표시다. 남겨두면
// 첫 열 이름이나 제목이 어긋난다. (소스에 문자를 직접 쓰면 Go가 거부한다.)
const utf8BOM = "\xef\xbb\xbf"

// Parse reads a Notion export zip and returns the items to import.
//
// 여러 CSV가 있을 때 "_all.csv"(전체 데이터베이스)를 우선한다 — 나머지는
// 필터된 뷰라서 행이 빠진다. 같은 이름이 여러 CSV에 나오면 한 번만 넣는다.
func Parse(zipPath string) (*Export, error) {
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer z.Close()

	out := &Export{Skipped: map[string]int{}}

	// 1) 페이지 본문을 제목으로 찾을 수 있게 색인한다.
	bodies := map[string]string{}
	var csvFiles []*zip.File
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := decodeName(f)
		switch strings.ToLower(path.Ext(name)) {
		case ".csv":
			csvFiles = append(csvFiles, f)
		case ".md":
			title, body, err := readMarkdown(f)
			if err != nil {
				return nil, err
			}
			if title != "" && body != "" {
				bodies[title] = body
			}
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
			out.Images = append(out.Images, path.Base(name))
		}
	}

	// 2) "_all"이 붙은 전체 CSV를 먼저 읽는다.
	sort.Slice(csvFiles, func(i, j int) bool {
		return strings.Contains(decodeName(csvFiles[i]), "_all.") &&
			!strings.Contains(decodeName(csvFiles[j]), "_all.")
	})

	seen := map[string]bool{}
	primary := "" // 가장 먼저(=_all) 읽은 CSV. 그 안의 중복과 뷰 중복을 구분한다.
	for _, f := range csvFiles {
		if primary == "" {
			primary = decodeName(f)
		}
		rows, err := readCSV(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", decodeName(f), err)
		}
		for _, row := range rows {
			title := strings.TrimSpace(row[colName])
			if title == "" {
				out.Skipped["이름이 빈 행"]++
				continue
			}
			if seen[title] {
				if decodeName(f) == primary {
					out.Skipped["같은 CSV 안에서 이름 중복"]++
				} else {
					out.Skipped["필터된 뷰·다른 CSV와 중복(정상)"]++
				}
				continue
			}
			seen[title] = true

			it := Item{
				Title:    title,
				Kind:     KindTodo,
				Category: relationName(row[colCategory]),
				Priority: strings.Count(row[colPriority], "⭐"),
				Note:     strings.TrimSpace(row[colNote]),
				Body:     bodies[title],
			}
			if strings.Contains(row[colSelect], eventSelect) {
				it.Kind = KindEvent
			}
			it.Start, it.End, it.AllDay = parseKoreanDate(row[colDate])
			if it.Start == "" && it.Kind == KindEvent {
				// 날짜 없는 일정은 캘린더에 놓을 자리가 없다 — 할 일로 돌린다.
				it.Kind = KindTodo
				out.Skipped["날짜 없어 할 일로 변환된 일정"]++
			}
			out.Items = append(out.Items, it)
		}
	}
	if len(out.Images) > 0 {
		out.Skipped["이미지(업로드 미지원)"] = len(out.Images)
	}
	return out, nil
}

// decodeName은 zip 항목 이름을 UTF-8로 되돌린다. UTF-8 플래그가 없는 항목은
// archive/zip이 CP437로 디코드해두므로 바이트를 복원해 다시 읽는다.
func decodeName(f *zip.File) string {
	if f.NonUTF8 && !utf8.ValidString(f.Name) {
		b := make([]byte, 0, len(f.Name))
		for _, r := range f.Name {
			if r < 256 {
				b = append(b, byte(r))
			}
		}
		if utf8.Valid(b) {
			return string(b)
		}
	}
	return f.Name
}

func readCSV(f *zip.File) ([]map[string]string, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	// 노션 CSV는 UTF-8 BOM으로 시작한다 — 남겨두면 첫 열 이름이 안 맞는다.
	data = []byte(strings.TrimPrefix(string(data), utf8BOM))

	r := csv.NewReader(strings.NewReader(string(data)))
	r.FieldsPerRecord = -1 // 노션이 가끔 열 수가 안 맞는 행을 낸다
	recs, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) < 2 {
		return nil, nil
	}
	head := recs[0]
	out := make([]map[string]string, 0, len(recs)-1)
	for _, rec := range recs[1:] {
		m := map[string]string{}
		for i, h := range head {
			if i < len(rec) {
				m[strings.TrimSpace(h)] = rec[i]
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// readMarkdown은 "# 제목" + "키: 값" 속성 블록 + 본문 구조에서 제목과 본문을
// 갈라낸다. 속성은 CSV에 같은 내용이 있으므로 버린다.
var propLine = regexp.MustCompile(`^[^:\s][^:]{0,30}: `)

func readMarkdown(f *zip.File) (title, body string, err error) {
	rc, err := f.Open()
	if err != nil {
		return "", "", err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return "", "", err
	}
	lines := strings.Split(strings.TrimPrefix(string(data), utf8BOM), "\n")
	if len(lines) == 0 {
		return "", "", nil
	}
	title = strings.TrimSpace(strings.TrimPrefix(lines[0], "# "))

	i := 1
	for i < len(lines) && (strings.TrimSpace(lines[i]) == "" || propLine.MatchString(lines[i])) {
		i++
	}
	return title, strings.TrimSpace(strings.Join(lines[i:], "\n")), nil
}

// relationName은 관계 속성 "쇼핑 (https://…)"에서 이름만 꺼낸다.
func relationName(s string) string {
	if i := strings.Index(s, " ("); i > 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// parseKoreanDate는 노션이 내보내는 한국어 날짜를 우리 형식으로 옮긴다.
//
//	"2026년 9월 17일"                      → 2026-09-17, 종일
//	"2025년 11월 5일 오전 9:00 (GMT+9)"     → 2025-11-05T09:00
//	"2025년 11월 14일 → 2025년 11월 16일"   → 시작/끝, 종일
//
// 타임존 표기는 버린다. 저장 형식 자체가 타임존 없는 로컬 벽시계라
// (0001_init.sql 참고) GMT+9 문자열을 해석할 필요가 없다.
var dateRx = regexp.MustCompile(`(\d{4})년\s*(\d{1,2})월\s*(\d{1,2})일(?:\s*(오전|오후)\s*(\d{1,2}):(\d{2}))?`)

func parseKoreanDate(s string) (start, end string, allDay bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", false
	}
	ms := dateRx.FindAllStringSubmatch(s, 2)
	if len(ms) == 0 {
		return "", "", false
	}
	start, startTimed := formatMatch(ms[0])
	if len(ms) > 1 {
		end, _ = formatMatch(ms[1])
	}
	if !startTimed {
		return start, end, true
	}
	// 시각이 있는 항목의 끝도 시각 형식이어야 서버 검증을 통과한다.
	if end != "" && len(end) == 10 {
		end += "T23:59"
	}
	return start, end, false
}

func formatMatch(m []string) (value string, timed bool) {
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	date := fmt.Sprintf("%04d-%02d-%02d", y, mo, d)
	if m[4] == "" {
		return date, false
	}
	h, _ := strconv.Atoi(m[5])
	mi, _ := strconv.Atoi(m[6])
	if m[4] == "오후" && h != 12 {
		h += 12
	}
	if m[4] == "오전" && h == 12 {
		h = 0
	}
	return fmt.Sprintf("%sT%02d:%02d", date, h, mi), true
}
