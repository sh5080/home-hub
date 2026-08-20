package store

// Sort는 카드 목록의 정렬 기준이다.
//
// 수동(SortManual)일 때만 드래그로 바꾼 position이 화면 순서가 된다. 다른
// 기준을 켜면 정렬이 매번 덮어쓰므로 드래그 재정렬은 의미가 없어진다 —
// UI도 그때는 재정렬을 막는다.
type Sort string

const (
	SortManual   Sort = "manual"
	SortTime     Sort = "time"
	SortPriority Sort = "priority"
)

// ParseSort는 쿼리 파라미터를 받는다. 모르는 값은 수동으로 떨어뜨린다 —
// 정렬 이름 오타로 500을 내기보다 기본 동작을 하는 편이 낫다.
func ParseSort(s string) Sort {
	switch Sort(s) {
	case SortTime:
		return SortTime
	case SortPriority:
		return SortPriority
	default:
		return SortManual
	}
}

// orderBy는 정렬 기준에 해당하는 SQL 절을 준다. 문자열 조립이지만 값이
// 하드코딩 상수뿐이라 주입 경로가 없다(ParseSort가 외부 입력을 걸러낸다).
//
// 두 기준 모두 마지막에 position으로 끊는다 — 수동 순서가 최종 동점자다.
// 마감 없는 카드는 항상 뒤로 보낸다(NULL이 먼저 오면 날짜순이 안 보인다).
func orderBy(s Sort, prefix string) string {
	p := prefix
	switch s {
	case SortTime:
		return p + "due_at IS NULL, " + p + "due_at, " + p + "priority DESC, " + p + "position, " + p + "id"
	case SortPriority:
		return p + "priority DESC, " + p + "due_at IS NULL, " + p + "due_at, " + p + "position, " + p + "id"
	default:
		return p + "position, " + p + "id"
	}
}
