package store

import (
	"encoding/json"
	"strings"
)

// 블록 문서는 편집기가 내는 JSON 배열. type 과 inline text 만 해석하고 나머지는 그대로 보관한다.

// allowedBlockTypes 에 없는 타입은 400. 편집기에 블록을 더하면 여기도 넓힌다.
var allowedBlockTypes = map[string]bool{
	// 텍스트
	"paragraph": true,
	"heading":   true,
	"quote":     true,
	"codeBlock": true,
	"callout":   true, // 직접 만든 블록 (노션의 콜아웃)
	// 목록
	"bulletListItem":   true,
	"numberedListItem": true,
	"checkListItem":    true,
	"toggleListItem":   true,
	// 구조
	"divider":      true,
	"table":        true,
	"tableContent": true,
	"tableRow":     true,
	"tableCell":    true,
	// 미디어는 URL 참조만(base64 는 1MiB 본문 제한에 걸린다).
	"image": true,
	"video": true,
	"audio": true,
	"file":  true,
}

// maxBlocks 는 깊게 중첩된 문서로 파싱을 태우는 걸 막는다.
const maxBlocks = 2000

type block struct {
	Type     string          `json:"type"`
	Props    json.RawMessage `json:"props"`
	Content  json.RawMessage `json:"content"`
	Children []block         `json:"children"`
}

// ValidateContent 는 파싱·타입 허용을 확인하고 파생 평문을 준다.
func ValidateContent(raw string) (plain string, err error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	var blocks []block
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		return "", invalid("본문 형식이 올바르지 않아요")
	}
	var sb strings.Builder
	n := 0
	if err := walk(blocks, &sb, &n); err != nil {
		return "", err
	}
	return strings.TrimSpace(sb.String()), nil
}

func walk(blocks []block, sb *strings.Builder, n *int) error {
	for _, b := range blocks {
		*n++
		if *n > maxBlocks {
			return invalid("본문이 너무 길어요")
		}
		if b.Type != "" && !allowedBlockTypes[b.Type] {
			return invalid("지원하지 않는 블록: " + b.Type)
		}
		appendText(b.Content, sb)
		sb.WriteByte('\n')
		if err := walk(b.Children, sb, n); err != nil {
			return err
		}
	}
	return nil
}

// appendText 는 inline 에서 텍스트만 뽑는다(table 은 객체).
func appendText(raw json.RawMessage, sb *strings.Builder) {
	if len(raw) == 0 {
		return
	}
	var items []struct {
		Text    string          `json:"text"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &items); err == nil {
		for _, it := range items {
			sb.WriteString(it.Text)
			appendText(it.Content, sb)
		}
		return
	}
	var obj struct {
		Rows []struct {
			Cells []json.RawMessage `json:"cells"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, r := range obj.Rows {
			for _, c := range r.Cells {
				appendText(c, sb)
				sb.WriteByte(' ')
			}
		}
	}
}

// PlainToContent 는 평문을 단락 문서로 감싼다.
func PlainToContent(text string) string {
	type inline struct {
		Type   string            `json:"type"`
		Text   string            `json:"text"`
		Styles map[string]string `json:"styles"`
	}
	type outBlock struct {
		Type     string            `json:"type"`
		Props    map[string]string `json:"props"`
		Content  []inline          `json:"content"`
		Children []outBlock        `json:"children"`
	}
	lines := strings.Split(text, "\n")
	out := make([]outBlock, 0, len(lines))
	for _, ln := range lines {
		b := outBlock{Type: "paragraph", Props: map[string]string{}, Content: []inline{}, Children: []outBlock{}}
		if ln != "" {
			b.Content = append(b.Content, inline{Type: "text", Text: ln, Styles: map[string]string{}})
		}
		out = append(out, b)
	}
	if len(out) == 0 {
		out = append(out, outBlock{Type: "paragraph", Props: map[string]string{}, Content: []inline{}, Children: []outBlock{}})
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// ChecklistContent 는 안내 문단 + 체크리스트 문서를 만든다(문서 형식은 서버 한 곳에서만 조립).
func ChecklistContent(note string, items []string) string {
	type inline struct {
		Type   string            `json:"type"`
		Text   string            `json:"text"`
		Styles map[string]string `json:"styles"`
	}
	type outBlock struct {
		Type     string         `json:"type"`
		Props    map[string]any `json:"props"`
		Content  []inline       `json:"content"`
		Children []outBlock     `json:"children"`
	}
	text := func(s string) []inline {
		if s == "" {
			return []inline{}
		}
		return []inline{{Type: "text", Text: s, Styles: map[string]string{}}}
	}
	out := make([]outBlock, 0, len(items)+1)
	if note != "" {
		out = append(out, outBlock{Type: "paragraph", Props: map[string]any{}, Content: text(note), Children: []outBlock{}})
	}
	for _, it := range items {
		out = append(out, outBlock{
			Type:     "checkListItem",
			Props:    map[string]any{"checked": false},
			Content:  text(it),
			Children: []outBlock{},
		})
	}
	if len(out) == 0 {
		out = append(out, outBlock{Type: "paragraph", Props: map[string]any{}, Content: []inline{}, Children: []outBlock{}})
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// ContentPhotos 는 이미지 블록의 /media/ 주소만 순서대로 뽑는다(외부 URL 은 세지 않는다).
func ContentPhotos(raw string, limit int) []string {
	out := []string{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	var blocks []block
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		return out
	}
	var walkPhotos func([]block)
	walkPhotos = func(bs []block) {
		for _, b := range bs {
			if len(out) >= limit {
				return
			}
			if b.Type == "image" && len(b.Props) > 0 {
				var p struct {
					URL string `json:"url"`
				}
				if json.Unmarshal(b.Props, &p) == nil && strings.HasPrefix(p.URL, "/media/") {
					out = append(out, p.URL)
				}
			}
			walkPhotos(b.Children)
		}
	}
	walkPhotos(blocks)
	return out
}
