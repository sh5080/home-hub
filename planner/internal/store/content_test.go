package store

import "testing"

func TestValidateContentDerivesPlainText(t *testing.T) {
	doc := `[
	  {"type":"heading","props":{"level":1},"content":[{"type":"text","text":"장보기"}],"children":[]},
	  {"type":"checkListItem","props":{"checked":false},"content":[{"type":"text","text":"우유"}],"children":[
	    {"type":"paragraph","content":[{"type":"text","text":"저지방"}],"children":[]}
	  ]},
	  {"type":"bulletListItem","content":[{"type":"text","text":"계란 "},{"type":"text","text":"한 판"}],"children":[]}
	]`
	plain, err := ValidateContent(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"장보기", "우유", "저지방", "계란 한 판"} {
		if !contains(plain, want) {
			t.Fatalf("plain text missing %q:\n%s", want, plain)
		}
	}
}

func TestValidateContentRejectsUnknownBlock(t *testing.T) {
	// 새 클라이언트가 모르는 블록을 저장하려 하면 조용히 받지 않고 거절한다.
	doc := `[{"type":"evilScript","content":[{"type":"text","text":"x"}],"children":[]}]`
	if _, err := ValidateContent(doc); err == nil {
		t.Fatal("unknown block type must be rejected")
	}
}

func TestValidateContentRejectsGarbage(t *testing.T) {
	if _, err := ValidateContent(`not json`); err == nil {
		t.Fatal("invalid JSON must be rejected")
	}
}

func TestValidateContentAllowsEmpty(t *testing.T) {
	if p, err := ValidateContent(""); err != nil || p != "" {
		t.Fatalf("empty content should be fine: %q %v", p, err)
	}
}

func TestPlainToContentRoundTrips(t *testing.T) {
	for _, in := range []string{"", "한 줄", "여러\n줄\n입니다", "'; DROP TABLE cards;--"} {
		doc := PlainToContent(in)
		plain, err := ValidateContent(doc)
		if err != nil {
			t.Fatalf("PlainToContent(%q) produced invalid doc: %v", in, err)
		}
		if want := trimLines(in); plain != want {
			t.Fatalf("round-trip %q: got %q want %q", in, plain, want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func trimLines(s string) string {
	for len(s) > 0 && (s[0] == '\n' || s[0] == ' ') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	return s
}

// 편집기가 기본으로 내는 블록은 전부 저장돼야 한다. 허용 목록이 좁으면
// 사용자는 "저장 실패"만 보고 원인을 알 수 없다 — toggleListItem이 실제로
// 그렇게 빠져 있었다.
func TestAllEditorBlocksAreAccepted(t *testing.T) {
	types := []string{
		"paragraph", "heading", "quote", "codeBlock", "callout",
		"bulletListItem", "numberedListItem", "checkListItem", "toggleListItem",
		"divider", "table", "image", "video", "audio", "file",
	}
	for _, ty := range types {
		doc := `[{"type":"` + ty + `","props":{},"content":[],"children":[]}]`
		if _, err := ValidateContent(doc); err != nil {
			t.Errorf("block %q rejected: %v", ty, err)
		}
	}
}
