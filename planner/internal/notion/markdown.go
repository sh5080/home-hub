package notion

import (
	"encoding/json"
	"regexp"
	"strings"
)

// 마크다운 → 블록 문서. 이 export 에 나온 것만 다룬다. 모르는 줄은 단락으로 남긴다(조용한 손실 방지).

// 인라인은 두 모양이다: 텍스트는 styles 필수, 링크는 styles 가 없어야 한다(null 이 붙으면 편집기가 깨진다).
type inline interface{ isInline() }

type textNode struct {
	Type   string          `json:"type"` // "text"
	Text   string          `json:"text"`
	Styles map[string]bool `json:"styles"`
}

type linkNode struct {
	Type    string     `json:"type"` // "link"
	Href    string     `json:"href"`
	Content []textNode `json:"content"`
}

func (textNode) isInline() {}
func (linkNode) isInline() {}

type outBlock struct {
	Type     string         `json:"type"`
	Props    map[string]any `json:"props,omitempty"`
	Content  []inline       `json:"content"`
	Children []outBlock     `json:"children"`
}

var (
	rxHeading  = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	rxBullet   = regexp.MustCompile(`^\s*[-*]\s+(.*)$`)
	rxNumbered = regexp.MustCompile(`^\s*\d+[.)]\s+(.*)$`)
	rxCheck    = regexp.MustCompile(`^\s*[-*]\s+\[([ xX])\]\s*(.*)$`)
	rxQuote    = regexp.MustCompile(`^>\s?(.*)$`)
	rxDivider  = regexp.MustCompile(`^\s*(-{3,}|\*{3,}|_{3,})\s*$`)
	rxLink     = regexp.MustCompile(`\[([^\]]*)\]\(([^)]+)\)`)
	rxBold     = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	rxCode     = regexp.MustCompile("`([^`]+)`")
	rxBareURL  = regexp.MustCompile(`^https?://\S+$`)
	rxCallout  = regexp.MustCompile(`(?s)<aside>(.*?)</aside>`)
)

// MarkdownToBlocks converts a Notion markdown body into our block document JSON.
func MarkdownToBlocks(md string) string {
	md = rxCallout.ReplaceAllString(md, "\x00CALLOUT\x00$1\x00END\x00")
	lines := strings.Split(md, "\n")
	var blocks []outBlock
	inCode := false
	var codeBuf []string

	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)

		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				blocks = append(blocks, block("codeBlock", text(strings.Join(codeBuf, "\n"))))
				codeBuf, inCode = nil, false
			} else {
				inCode = true
			}
			continue
		}
		if inCode {
			codeBuf = append(codeBuf, ln)
			continue
		}

		// 콜아웃 표식(<aside> 를 치환해 둔 것)
		if strings.HasPrefix(trimmed, "\x00CALLOUT\x00") {
			body := strings.TrimSuffix(strings.TrimPrefix(trimmed, "\x00CALLOUT\x00"), "\x00END\x00")
			for _, part := range strings.Split(body, "\n") {
				if s := strings.TrimSpace(part); s != "" {
					blocks = append(blocks, block("callout", parseInline(s)))
				}
			}
			continue
		}
		if trimmed == "" {
			continue
		}

		switch {
		case rxDivider.MatchString(trimmed):
			blocks = append(blocks, outBlock{Type: "divider", Content: []inline{}, Children: []outBlock{}})

		case rxHeading.MatchString(trimmed):
			m := rxHeading.FindStringSubmatch(trimmed)
			lvl := len(m[1])
			if lvl > 3 {
				lvl = 3 // 편집기는 3단계까지만 낸다
			}
			b := block("heading", parseInline(m[2]))
			b.Props = map[string]any{"level": lvl}
			blocks = append(blocks, b)

		case rxCheck.MatchString(trimmed):
			m := rxCheck.FindStringSubmatch(trimmed)
			b := block("checkListItem", parseInline(m[2]))
			b.Props = map[string]any{"checked": strings.ToLower(m[1]) == "x"}
			blocks = append(blocks, b)

		case rxBullet.MatchString(trimmed):
			blocks = append(blocks, block("bulletListItem", parseInline(rxBullet.FindStringSubmatch(trimmed)[1])))

		case rxNumbered.MatchString(trimmed):
			blocks = append(blocks, block("numberedListItem", parseInline(rxNumbered.FindStringSubmatch(trimmed)[1])))

		case rxQuote.MatchString(trimmed):
			blocks = append(blocks, block("quote", parseInline(rxQuote.FindStringSubmatch(trimmed)[1])))

		case rxBareURL.MatchString(trimmed):
			blocks = append(blocks, block("paragraph", []inline{link(trimmed, trimmed)}))

		default:
			blocks = append(blocks, block("paragraph", parseInline(trimmed)))
		}
	}
	if inCode && len(codeBuf) > 0 { // 닫히지 않은 펜스
		blocks = append(blocks, block("codeBlock", text(strings.Join(codeBuf, "\n"))))
	}
	if len(blocks) == 0 {
		return ""
	}
	b, _ := json.Marshal(blocks)
	return string(b)
}

func block(typ string, content []inline) outBlock {
	return outBlock{Type: typ, Content: content, Children: []outBlock{}}
}

func text(s string) []inline {
	if s == "" {
		return []inline{}
	}
	return []inline{textNode{Type: "text", Text: s, Styles: map[string]bool{}}}
}

func link(label, href string) inline {
	return linkNode{Type: "link", Href: href, Content: []textNode{{Type: "text", Text: label, Styles: map[string]bool{}}}}
}

// parseInline 은 링크 → 굵게 → 인라인코드 순. 중첩은 다루지 않는다.
func parseInline(s string) []inline {
	if s == "" {
		return []inline{}
	}
	var out []inline
	rest := s
	for {
		loc := rxLink.FindStringSubmatchIndex(rest)
		if loc == nil {
			break
		}
		out = append(out, styled(rest[:loc[0]])...)
		label := rest[loc[2]:loc[3]]
		href := rest[loc[4]:loc[5]]
		if label == "" {
			label = href
		}
		out = append(out, link(label, href))
		rest = rest[loc[1]:]
	}
	out = append(out, styled(rest)...)
	if len(out) == 0 {
		return []inline{}
	}
	return out
}

// styled는 **굵게** 와 `코드` 를 스타일 붙은 텍스트 조각으로 나눈다.
func styled(s string) []inline {
	if s == "" {
		return nil
	}
	var out []inline
	rest := s
	for {
		bold := rxBold.FindStringSubmatchIndex(rest)
		code := rxCode.FindStringSubmatchIndex(rest)
		var loc []int
		var style string
		switch {
		case bold != nil && (code == nil || bold[0] < code[0]):
			loc, style = bold, "bold"
		case code != nil:
			loc, style = code, "code"
		default:
			out = append(out, text(rest)...)
			return out
		}
		out = append(out, text(rest[:loc[0]])...)
		out = append(out, textNode{Type: "text", Text: rest[loc[2]:loc[3]], Styles: map[string]bool{style: true}})
		rest = rest[loc[1]:]
	}
}
