package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sh5080/home-hub/planner/internal/store"
)

// planner care import <폴더>... --child <이름> --as <이름> [--apply]
// 베이비타임 활동 내보내기(*_asc.txt). 첫 줄은 시각 또는 '시작 ~ 끝', 이어서 '키: 값' 줄들.
// 끝이 있는데 소요시간이 없으면 차이로 채운다.
func cmdCare(args []string) error {
	if len(args) > 0 && args[0] == "relink-solids" {
		return cmdCareRelinkSolids(args[1:])
	}
	if len(args) == 0 || args[0] != "import" {
		return fmt.Errorf("usage: planner care import <폴더>... --child <이름> --as <이름> [--apply]\n       planner care relink-solids --child <이름> [--apply]")
	}
	fs := flag.NewFlagSet("care import", flag.ExitOnError)
	data := dataFlag(fs)
	child := fs.String("child", "", "누구의 기록인지 (이유식 대상 이름)")
	as := fs.String("as", "", "글쓴이로 남길 가족 이름")
	apply := fs.Bool("apply", false, "실제로 넣는다. 없으면 읽고 세기만")
	var dirs, rest []string
	for i := 1; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			rest = append(rest, args[i:]...)
			break
		}
		dirs = append(dirs, args[i])
	}
	fs.Parse(rest)
	if len(dirs) == 0 || *child == "" || *as == "" {
		return fmt.Errorf("폴더, --child, --as 가 필요해요")
	}

	st, err := store.Open(*data)
	if err != nil {
		return err
	}
	defer st.Close()
	ctx := context.Background()
	c, err := findChild(ctx, st, *child)
	if err != nil {
		return err
	}
	users, err := st.ListUsers(ctx)
	if err != nil {
		return err
	}
	var uid int64
	for _, u := range users {
		if u.Name == *as {
			uid = u.ID
		}
	}
	if uid == 0 {
		return fmt.Errorf("가족 중에 %q 가 없어요", *as)
	}

	var all []store.CareImportEntry
	skipped := map[string]int{}
	for _, d := range dirs {
		es, sk, err := readBabyTimeActivity(d)
		if err != nil {
			return fmt.Errorf("%s: %w", d, err)
		}
		all = append(all, es...)
		for k, v := range sk {
			skipped[k] += v
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].At < all[j].At })
	byKind := map[string]int{}
	for _, e := range all {
		byKind[e.Kind]++
	}
	fmt.Printf("기록 %d건 (%s ~ %s)\n", len(all), all[0].At, all[len(all)-1].At)
	keys := make([]string, 0, len(byKind))
	for k := range byKind {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %-10s %d\n", k, byKind[k])
	}
	for k, v := range skipped {
		fmt.Printf("  (건너뜀) %s: %d\n", k, v)
	}
	if !*apply {
		fmt.Println("넣으려면 --apply 를 붙이세요.")
		return nil
	}
	added, dupSrc, dupManual, err := st.CareImport(ctx, c.UserID, all, uid)
	if err != nil {
		return err
	}
	fmt.Printf("%d건 넣음 · 이미 가져온 것 %d건 · 앱에서 직접 적은 것과 겹쳐 뺀 것 %d건\n", added, dupSrc, dupManual)
	return nil
}

var btNum = regexp.MustCompile(`^\s*([0-9.]+)\s*\(([^)]*)\)`)

func btParseTime(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 03:04 PM", strings.TrimSpace(s), time.Local)
}

// readBabyTimeActivity 는 한 폴더를 읽는다. 둘째 값은 못 옮긴 항목 수.
func readBabyTimeActivity(dir string) ([]store.CareImportEntry, map[string]int, error) {
	matches, _ := filepath.Glob(filepath.Join(dir, "*_asc.txt"))
	if len(matches) == 0 {
		return nil, nil, fmt.Errorf("*_asc.txt 가 없어요")
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		return nil, nil, err
	}
	var out []store.CareImportEntry
	skipped := map[string]int{}
	seen := map[string]int{} // 같은 시각·같은 종류가 둘이면 번호를 붙인다
	for _, chunk := range strings.Split(string(raw), "====================") {
		var lines []string
		for _, l := range strings.Split(strings.ReplaceAll(chunk, "\r\n", "\n"), "\n") {
			if strings.TrimSpace(l) != "" {
				lines = append(lines, strings.TrimSpace(l))
			}
		}
		if len(lines) < 2 {
			continue
		}
		startS, endS, ranged := strings.Cut(lines[0], "~")
		start, err := btParseTime(startS)
		if err != nil {
			return nil, nil, fmt.Errorf("시각을 못 읽었어요: %q", lines[0])
		}
		f := map[string]string{}
		for _, l := range lines[1:] {
			if k, v, ok := strings.Cut(l, ":"); ok {
				f[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
		num := func(key string) (*int, string) {
			m := btNum.FindStringSubmatch(f[key])
			if m == nil {
				return nil, ""
			}
			x, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				return nil, ""
			}
			n := int(x + 0.5)
			return &n, m[2]
		}
		minutes, _ := num("소요시간")
		if minutes == nil && ranged {
			if end, err := btParseTime(endS); err == nil && end.After(start) {
				n := int(end.Sub(start).Minutes() + 0.5)
				minutes = &n
			}
		}
		note := f["메모"]
		addNote := func(s string) {
			if s == "" {
				return
			}
			if note != "" {
				note += " · "
			}
			note += s
		}

		e := store.CareImportEntry{At: start.Format("2006-01-02T15:04"), Minutes: minutes}
		switch kind := f["기록 종류"]; kind {
		case "분유":
			e.Kind = "formula"
			e.AmountML, _ = num("분유 총 양(ml)")
		case "유축수유":
			e.Kind = "pump_feed"
			e.AmountML, _ = num("유축수유 총 양(ml)")
		case "유축":
			e.Kind = "pump"
			e.AmountML, _ = num("유축 총 양(ml)")
			l, _ := num("유축 왼쪽 시간")
			r, _ := num("유축 오른쪽 시간")
			if l != nil || r != nil {
				parts := []string{}
				if l != nil {
					parts = append(parts, fmt.Sprintf("왼쪽 %d분", *l))
				}
				if r != nil {
					parts = append(parts, fmt.Sprintf("오른쪽 %d분", *r))
				}
				e.Detail = strings.Join(parts, " · ")
			}
		case "이유식":
			e.Kind = "solids"
			amt, unit := num("이유식 총 양(ml)")
			e.AmountML = amt
			if unit != "" && unit != "ml" {
				addNote("양 단위 " + unit)
			}
			e.Detail = f["이유식 종류"]
		case "기저귀":
			e.Kind = "diaper"
			e.Detail = f["배변 형태"]
			if e.Detail == "둘다" {
				e.Detail = "둘 다"
			}
			if c := f["배변색"]; c != "" {
				addNote("색 #" + c)
			}
			if f["image"] != "" {
				addNote("사진 있었음")
			}
		case "낮잠", "밤잠":
			e.Kind = "sleep"
			e.Detail = kind
		case "목욕":
			e.Kind = "bath"
		case "놀이":
			e.Kind = "play"
			e.Detail = f["놀이 종류"]
		case "터미타임":
			e.Kind = "tummy"
		case "간식":
			e.Kind = "snack"
			e.Detail = f["간식 종류"]
		case "투약":
			e.Kind = "medicine"
			e.Detail = f["약 종류"]
		case "병원":
			e.Kind = "hospital"
			e.Detail = strings.Trim(strings.Join([]string{f["방문사유"], f["병원명"]}, " · "), " ·")
			addNote(f["방문유형"])
		case "열":
			e.Kind = "temp"
			if t := btNum.FindStringSubmatch(f["체온"]); t != nil {
				e.Detail = t[1] + "°C"
			}
		case "기타":
			e.Kind = "etc"
			e.Detail = f["기타 항목"]
		default:
			// 사용자가 만든 항목(기록A 등)은 이름을 살려 '기타'로.
			if name := f["항목 이름"]; name != "" {
				e.Kind = "etc"
				e.Detail = name
			} else {
				skipped[kind]++
				continue
			}
		}
		e.Note = note
		k := e.At + "|" + e.Kind
		seen[k]++
		e.Source = fmt.Sprintf("babytime:%s|%d", k, seen[k])
		out = append(out, e)
	}
	return out, skipped, nil
}

// planner care relink-solids — 글자로 적힌 이유식 기록을 식단 재료 id 로 걸고 같은 날 끼니와 잇는다.
// 베이비타임 이름과 식단 이름이 달라 아래 표로 맞춘다.
var solidsRename = map[string][]string{
	"쌀오트밀미음": {"쌀오트밀"},
	"미음":     {"쌀"},        // 첫 주 식단이 '쌀'이다. 미음은 곧 쌀죽이다
	"소고기미음":  {"쌀", "소고기"}, // 미음(쌀) + 소고기
	"단호밧":    {"단호박"},      // 오타
}

func cmdCareRelinkSolids(args []string) error {
	fs := flag.NewFlagSet("care relink-solids", flag.ExitOnError)
	data := dataFlag(fs)
	child := fs.String("child", "", "누구의 기록인지")
	apply := fs.Bool("apply", false, "실제로 바꾼다. 없으면 무엇이 바뀔지만 보여준다")
	fs.Parse(args)
	if *child == "" {
		return fmt.Errorf("--child 가 필요해요")
	}
	st, err := store.Open(*data)
	if err != nil {
		return err
	}
	defer st.Close()
	ctx := context.Background()
	c, err := findChild(ctx, st, *child)
	if err != nil {
		return err
	}

	type job struct {
		log   store.CareLog
		names []string
	}
	byDate := map[string][]job{}
	var dates []string
	days, err := st.CareSolidsDates(ctx, c.UserID)
	if err != nil {
		return err
	}
	for _, date := range days {
		day, err := st.CareList(ctx, c.UserID, date)
		if err != nil {
			return err
		}
		for _, l := range day.Logs {
			if l.Kind != "solids" {
				continue
			}
			var names []string
			seen := map[string]bool{}
			src := ""
			if l.Detail != nil {
				src = *l.Detail
			}
			for _, part := range strings.Split(src, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				mapped := []string{part}
				if m, ok := solidsRename[part]; ok {
					mapped = m
				}
				for _, n := range mapped {
					if !seen[n] {
						seen[n] = true
						names = append(names, n)
					}
				}
			}
			// 쌀오트밀이 이미 있으면 '미음'에서 나온 쌀은 뺀다(베이스 중복).
			if seen["쌀오트밀"] && seen["쌀"] {
				kept := names[:0]
				for _, n := range names {
					if n != "쌀" {
						kept = append(kept, n)
					}
				}
				names = kept
			}
			if byDate[date] == nil {
				dates = append(dates, date)
			}
			byDate[date] = append(byDate[date], job{l, names})
		}
	}
	sort.Strings(dates)

	changed, linked := 0, 0
	for _, date := range dates {
		dday, err := store.BFDDayOf(c.BirthDate, date)
		if err != nil {
			return err
		}
		plan, err := st.BFRange(ctx, c.UserID, dday, dday)
		if err != nil {
			return err
		}
		used := map[int64]bool{}
		for _, j := range byDate[date] {
			// 같은 날 끼니 중 재료가 가장 많이 겹치는 것에 잇는다.
			var best *store.BFMeal
			bestN := 0
			if len(plan) > 0 {
				for i := range plan[0].Meals {
					m := &plan[0].Meals[i]
					if used[m.ID] {
						continue
					}
					have := map[string]bool{m.Base: true}
					for _, t := range m.Toppings {
						have[t] = true
					}
					n := 0
					for _, x := range j.names {
						if have[x] {
							n++
						}
					}
					if n > bestN {
						best, bestN = m, n
					}
				}
			}
			old := ""
			if j.log.Detail != nil {
				old = *j.log.Detail
			}
			link := "끼니 없음"
			if best != nil {
				used[best.ID] = true
				link = fmt.Sprintf("%s 끼니(%d)", best.Title, best.ID)
				linked++
			}
			fmt.Printf("%s  %-40q → %v  · %s\n", j.log.At, old, j.names, link)
			if !*apply {
				continue
			}
			names := j.names
			in := store.CareInput{Ingredients: &names}
			if best != nil {
				id := best.ID
				in.MealID = &id
			}
			if _, err := st.CareUpdate(ctx, j.log.ID, in); err != nil {
				return fmt.Errorf("%s: %w", j.log.At, err)
			}
			changed++
		}
	}
	if !*apply {
		fmt.Printf("\n%d건을 바꾸고 그중 %d건을 식단 끼니와 잇습니다. 실제로 하려면 --apply\n", countJobs(byDate), linked)
		return nil
	}
	fmt.Printf("\n%d건 바꿈 · %d건 식단 끼니와 연결\n", changed, linked)
	return nil
}

func countJobs[T any](m map[string][]T) int {
	n := 0
	for _, v := range m {
		n += len(v)
	}
	return n
}
