// Package notion 은 노션 'Markdown & CSV' export 를 읽는다(한국어 열 이름·날짜).
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
	// Skipped 는 가져오지 않은 것의 사유별 개수.
	Skipped map[string]int
	// Images 는 export 의 이미지 파일 이름(가져오지는 않는다).
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

// utf8BOM 은 노션 CSV·MD 앞의 BOM. 남기면 첫 열 이름이 어긋난다.
const utf8BOM = "\xef\xbb\xbf"

// Parse 는 export zip 을 읽는다. '_all.csv'(전체 DB)를 우선하고 같은 이름은 한 번만 넣는다.
func Parse(zipPath string) (*Export, error) {
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer z.Close()

	out := &Export{Skipped: map[string]int{}}

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
				// 날짜 없는 일정은 할 일로 돌린다.
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

// decodeName 은 zip 이름을 UTF-8 로 되돌린다(플래그 없으면 archive/zip 이 CP437 로 읽는다).
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

// parseKoreanDate 는 노션의 한국어 날짜를 우리 형식으로 옮긴다.
//
//	"2026년 9월 17일" → 2026-09-17 / "… 오전 9:00 (GMT+9)" → T09:00 / "A → B" → 시작·끝
//
// 타임존 표기는 버린다(저장 형식이 떠다니는 로컬 시각).
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
	// 시각 있는 항목의 끝도 시각 형식이어야 검증을 통과한다.
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
