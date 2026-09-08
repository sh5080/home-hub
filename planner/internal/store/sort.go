package store

// Sort 는 카드 정렬 기준.
type Sort string

const (
	SortManual   Sort = "manual"
	SortTime     Sort = "time"
	SortPriority Sort = "priority"
)

// ParseSort 는 모르는 값을 수동으로 떨어뜨린다.
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

// Order 는 정렬 방향.
type Order bool

const (
	Asc  Order = false
	Desc Order = true
)

// ParseOrder는 ?order=desc 만 역순으로 본다. 그 외는 오름차순.
func ParseOrder(s string) Order {
	if s == "desc" {
		return Desc
	}
	return Asc
}

// orderBy 는 정렬 SQL 절(상수 조합뿐). 마지막 동점자는 position.
// 마감 없는 카드는 방향과 무관하게 항상 뒤.
func orderBy(s Sort, o Order, prefix string) string {
	p := prefix
	dir := func(asc, desc string) string {
		if o == Desc {
			return desc
		}
		return asc
	}
	switch s {
	case SortTime:
		return p + "due_at IS NULL, " +
			p + "due_at " + dir("ASC", "DESC") + ", " +
			p + "priority DESC, " + p + "position, " + p + "id"
	case SortPriority:
		return p + "priority " + dir("DESC", "ASC") + ", " +
			p + "due_at IS NULL, " + p + "due_at, " + p + "position, " + p + "id"
	default:
		return p + "position " + dir("ASC", "DESC") + ", " + p + "id"
	}
}
