package store

import (
	"encoding/json"
	"strings"
)

// 블록 문서는 편집기가 내는 JSON 배열이다. 우리는 최소한만 안다: 각 블록에
// type이 있고, 텍스트는 inline content 배열의 text 필드에 들어간다. 나머지
// (props, children)는 그대로 보관한다 — 편집기가 진화해도 서버를 안 고친다.

// allowedBlockTypes는 저장을 허용하는 블록 종류다. 목록에 없는 타입은 400으로
// 거절한다. 새 클라이언트가 조용히 이상한 걸 넣는 대신 시끄럽게 실패하도록.
// 편집기에 블록을 추가하면 여기도 함께 넓힌다.
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
	// 미디어 — 지금은 URL 참조만 받는다. 파일 업로드는 별도 기능이고,
	// 본문에 base64를 박는 건 1MiB 본문 제한에 바로 걸려서 허용하지 않는다.
	"image": true,
	"video": true,
	"audio": true,
	"file":  true,
}

// maxBlocks는 한 카드의 블록 수 상한이다. 본문 자체는 1MiB 본문 제한에 걸리지만,
// 깊게 중첩된 문서로 파싱을 태우는 걸 막는다.
const maxBlocks = 2000

type block struct {
	Type     string          `json:"type"`
	Content  json.RawMessage `json:"content"`
	Children []block         `json:"children"`
}

// ValidateContent는 문서가 파싱되고 모든 블록 타입이 허용 목록에 있는지 본다.
// 반환값은 파생 평문이다.
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

// appendText는 inline content에서 텍스트만 뽑는다. content는 배열이거나
// (table처럼) 객체일 수 있어 둘 다 받는다.
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

// PlainToContent는 평문을 단락 문서로 감싼다. 0003 백필과, 편집기를 쓰지 못하는
// 클라이언트(평문 폴백)가 같은 모양을 내도록 한 곳에 둔다.
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
