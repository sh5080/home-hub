package store

import (
	"strings"
	"testing"
)

// 편집기가 받아들이는 형식인지는 ValidateContent가 유일한 판정자다.
// 직접 조립한 JSON은 반드시 그걸 통과시켜 본다.
func TestChecklistContentIsValid(t *testing.T) {
	raw := ChecklistContent("3주치 기준이에요", []string{"소고기 21", "당근 21"})
	plain, err := ValidateContent(raw)
	if err != nil {
		t.Fatalf("만든 블록 문서를 서버가 거절했다: %v\n%s", err, raw)
	}
	for _, want := range []string{"3주치 기준이에요", "소고기 21", "당근 21"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("평문에 %q 가 없다: %q", want, plain)
		}
	}
}

func TestChecklistContentEmpty(t *testing.T) {
	if _, err := ValidateContent(ChecklistContent("", nil)); err != nil {
		t.Fatalf("빈 목록도 유효해야 한다: %v", err)
	}
}
