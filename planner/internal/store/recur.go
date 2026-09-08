package store

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// 반복 규칙(카드의 문자열 하나):
//
//   daily          매일
//   every:14       14일마다
//   weekly:5       요일 마스크. bit0=월 … bit6=일 (루틴 weekdays_mask 와 같다)
//   monthly:15     매월 15일. 없는 날은 그 달 마지막 날로
//   yearly:03-05   매년 3월 5일. 2/29 는 평년에 28일로

const recurAllWeekdays = 0x7F

// ValidateRecur는 규칙을 검사하고 정규화한다. 빈 문자열은 '반복 없음'이다.
func ValidateRecur(rule string) (string, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return "", nil
	}
	kind, arg, _ := strings.Cut(rule, ":")
	switch kind {
	case "daily":
		if arg != "" {
			return "", invalid("daily 에는 값을 붙이지 않아요")
		}
		return "daily", nil
	case "every":
		n, err := strconv.Atoi(arg)
		if err != nil || n < 1 || n > 365 {
			return "", invalid("every 는 1~365 사이의 날 수예요")
		}
		return "every:" + strconv.Itoa(n), nil
	case "weekly":
		m, err := strconv.Atoi(arg)
		if err != nil || m <= 0 || m > recurAllWeekdays {
			return "", invalid("weekly 는 1~127 사이의 요일 마스크예요")
		}
		return "weekly:" + strconv.Itoa(m), nil
	case "monthly":
		d, err := strconv.Atoi(arg)
		if err != nil || d < 1 || d > 31 {
			return "", invalid("monthly 는 1~31 사이의 날짜예요")
		}
		return "monthly:" + strconv.Itoa(d), nil
	case "yearly":
		t, err := time.Parse("01-02", arg)
		if err != nil {
			return "", invalid("yearly 는 MM-DD 형식이에요")
		}
		return "yearly:" + t.Format("01-02"), nil
	}
	return "", invalid("모르는 반복 규칙이에요")
}

// NextOccurrence 는 from **다음** 회차. 시각이 붙어 있으면 유지한다.
func NextOccurrence(rule, from string) (string, error) {
	rule, err := ValidateRecur(rule)
	if err != nil {
		return "", err
	}
	if rule == "" {
		return "", invalid("반복 규칙이 없어요")
	}
	if !validDue(from) {
		return "", invalid("마감이 올바르지 않아요")
	}
	day, clock := from[:10], ""
	if len(from) > 10 {
		clock = from[10:]
	}
	d, err := time.Parse("2006-01-02", day)
	if err != nil {
		return "", invalid("마감이 올바르지 않아요")
	}

	kind, arg, _ := strings.Cut(rule, ":")
	var next time.Time
	switch kind {
	case "daily":
		next = d.AddDate(0, 0, 1)
	case "every":
		n, _ := strconv.Atoi(arg)
		next = d.AddDate(0, 0, n)
	case "weekly":
		mask, _ := strconv.Atoi(arg)
		// 최대 7일만 보면 반드시 만난다 — 마스크가 0이 아님은 검증됐다.
		for i := 1; i <= 7; i++ {
			c := d.AddDate(0, 0, i)
			if mask&(1<<isoWeekdayBit(c)) != 0 {
				next = c
				break
			}
		}
	case "monthly":
		want, _ := strconv.Atoi(arg)
		next = clampDay(d.Year(), d.Month()+1, want)
		// 당겨진 날짜가 현재보다 앞서지 않게 한 달 더 민다(1/31 → 2월).
		for !next.After(d) {
			next = clampDay(next.Year(), next.Month()+1, want)
		}
	case "yearly":
		t, _ := time.Parse("01-02", arg)
		next = clampDay(d.Year(), t.Month(), t.Day())
		for !next.After(d) {
			next = clampDay(next.Year()+1, t.Month(), t.Day())
		}
	}
	if next.IsZero() {
		return "", invalid("다음 회차를 구하지 못했어요")
	}
	return next.Format("2006-01-02") + clock, nil
}

// isoWeekdayBit 은 월=0 … 일=6.
func isoWeekdayBit(t time.Time) uint {
	return uint((int(t.Weekday()) + 6) % 7)
}

// clampDay 는 그 달에 없는 날짜를 마지막 날로 당긴다(time.Date 는 다음 달로 넘긴다).
func clampDay(year int, month time.Month, day int) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	if day > last {
		day = last
	}
	return time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, time.UTC)
}

// RecurLabel 은 화면용 짧은 설명.
func RecurLabel(rule string) string {
	kind, arg, _ := strings.Cut(rule, ":")
	switch kind {
	case "daily":
		return "매일"
	case "every":
		return arg + "일마다"
	case "weekly":
		m, _ := strconv.Atoi(arg)
		if m == recurAllWeekdays {
			return "매일"
		}
		names := []string{"월", "화", "수", "목", "금", "토", "일"}
		var on []string
		for i := 0; i < 7; i++ {
			if m&(1<<uint(i)) != 0 {
				on = append(on, names[i])
			}
		}
		return "매주 " + strings.Join(on, "·")
	case "monthly":
		return "매월 " + arg + "일"
	case "yearly":
		t, err := time.Parse("01-02", arg)
		if err != nil {
			return "매년"
		}
		return fmt.Sprintf("매년 %d월 %d일", int(t.Month()), t.Day())
	}
	return ""
}

// ExpandRecur 는 값이 빠진 규칙(weekly/monthly/yearly)을 마감에서 채운다.
func ExpandRecur(rule, due string) (string, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return "", nil
	}
	if strings.Contains(rule, ":") || rule == "daily" {
		return ValidateRecur(rule)
	}
	if due == "" {
		return "", invalid("반복하려면 마감이 있어야 해요")
	}
	d, err := time.Parse("2006-01-02", due[:10])
	if err != nil {
		return "", invalid("마감이 올바르지 않아요")
	}
	switch rule {
	case "weekly":
		return fmt.Sprintf("weekly:%d", 1<<isoWeekdayBit(d)), nil
	case "monthly":
		return fmt.Sprintf("monthly:%d", d.Day()), nil
	case "yearly":
		return "yearly:" + d.Format("01-02"), nil
	}
	return "", invalid("모르는 반복 규칙이에요")
}
