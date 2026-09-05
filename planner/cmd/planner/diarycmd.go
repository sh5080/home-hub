package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/sh5080/home-hub/planner/internal/store"
)

// planner diary import <폴더>... --as <이름> [--child <이름>] [--apply]
// 베이비타임 일기 내보내기(*_asc.txt + images/).
// 글 번호는 사진 파일명(181_1_0 의 가운데)을 따른다 — 텍스트 순서는 믿지 않는다.
// 출처 키 'babytime:<day>_<n>' 로 중복을 막는다.
func cmdDiary(args []string) error {
	if len(args) == 0 || args[0] != "import" {
		return fmt.Errorf("usage: planner diary import <폴더>... --as <이름> [--child <이름>] [--apply]")
	}
	fs := flag.NewFlagSet("diary import", flag.ExitOnError)
	data := dataFlag(fs)
	as := fs.String("as", "", "글쓴이로 기록할 가족 이름")
	child := fs.String("child", "", "day 번호를 이 아이의 생일과 맞춰본다(생후 일수, 태어난 날=1)")
	apply := fs.Bool("apply", false, "실제로 넣는다. 없으면 읽고 세기만")
	// 폴더 이름에 ❤️ 가 있어 플래그를 따로 걸러낸다.
	var dirs []string
	var rest []string
	for i := 1; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			rest = append(rest, args[i:]...)
			break
		}
		dirs = append(dirs, args[i])
	}
	fs.Parse(rest)
	if len(dirs) == 0 || *as == "" {
		return fmt.Errorf("폴더와 --as <이름> 이 필요해요")
	}

	st, err := store.Open(*data)
	if err != nil {
		return err
	}
	defer st.Close()
	ctx := context.Background()

	users, err := st.ListUsers(ctx)
	if err != nil {
		return err
	}
	var author *store.User
	for i := range users {
		if users[i].Name == *as {
			author = &users[i]
		}
	}
	if author == nil {
		return fmt.Errorf("가족 중에 %q 가 없어요", *as)
	}
	birth := ""
	if *child != "" {
		c, err := findChild(ctx, st, *child)
		if err != nil {
			return err
		}
		birth = c.BirthDate
	}

	var all []btEntry
	for _, d := range dirs {
		es, err := readBabyTimeDir(d)
		if err != nil {
			return fmt.Errorf("%s: %w", d, err)
		}
		all = append(all, es...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].date != all[j].date {
			return all[i].date < all[j].date
		}
		return all[i].n < all[j].n
	})

	photos := 0
	for _, e := range all {
		photos += len(e.images)
		if birth != "" {
			dday, err := store.BFDDayOf(birth, e.date)
			if err != nil {
				return err
			}
			if dday+1 != e.day {
				return fmt.Errorf("%s: day %d 인데 생일로 세면 생후 %d일 — 생일이 다르거나 파일이 이상해요", e.date, e.day, dday+1)
			}
		}
	}
	fmt.Printf("글 %d장, 사진 %d장 (%s ~ %s)\n", len(all), photos, all[0].date, all[len(all)-1].date)
	if birth != "" {
		fmt.Printf("day 번호가 %s 의 생일과 전부 맞아요.\n", *child)
	}
	if !*apply {
		fmt.Println("넣으려면 --apply 를 붙이세요.")
		return nil
	}

	uid := author.ID
	var entries []store.DiaryImportEntry
	for _, e := range all {
		var urls []string
		for _, img := range e.images {
			b, err := os.ReadFile(img)
			if err != nil {
				return err
			}
			m, err := st.MediaPut(ctx, b, &uid)
			if err != nil {
				return fmt.Errorf("%s: %w", img, err)
			}
			urls = append(urls, m.URL)
		}
		entries = append(entries, store.DiaryImportEntry{
			Source:  fmt.Sprintf("babytime:%d_%d", e.day, e.n),
			Date:    e.date,
			Content: blocksWithImages(e.text, urls),
		})
	}
	added, skipped, err := st.DiaryImport(ctx, entries, uid)
	if err != nil {
		return err
	}
	fmt.Printf("%d장을 넣었고 %d장은 이미 있어서 건너뛰었어요. 글쓴이: %s\n", added, skipped, author.Name)
	return nil
}

type btEntry struct {
	day    int
	n      int // 그날의 몇 번째 글 (1부터)
	date   string
	text   string
	images []string // 파일 경로
}

func readBabyTimeDir(dir string) ([]btEntry, error) {
	matches, _ := filepath.Glob(filepath.Join(dir, "*_asc.txt"))
	if len(matches) == 0 {
		return nil, fmt.Errorf("*_asc.txt 가 없어요")
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		return nil, err
	}
	var out []btEntry
	for _, chunk := range strings.Split(string(raw), "===================") {
		lines := strings.Split(strings.ReplaceAll(chunk, "\r\n", "\n"), "\n")
		var e btEntry
		var body []string
		inContent := false
		for _, ln := range lines {
			switch {
			case strings.HasPrefix(ln, "day:"):
				e.day, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(ln, "day:")))
			case strings.HasPrefix(ln, "date:"):
				e.date = strings.TrimSpace(strings.TrimPrefix(ln, "date:"))
			case strings.HasPrefix(ln, "content:"):
				inContent = true
			case strings.HasPrefix(ln, "images:"):
				inContent = false
				for _, part := range strings.Split(ln, ",") {
					name := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "images:"))
					if name != "" {
						e.images = append(e.images, filepath.Join(dir, "images", name))
					}
				}
			default:
				if inContent {
					body = append(body, ln)
				}
			}
		}
		if e.day == 0 || e.date == "" {
			continue
		}
		e.text = strings.TrimRight(strings.Join(body, "\n"), " \n")
		if len(e.images) > 0 {
			parts := strings.Split(strings.TrimSuffix(filepath.Base(e.images[0]), filepath.Ext(e.images[0])), "_")
			if len(parts) == 3 {
				e.n, _ = strconv.Atoi(parts[1])
			}
			for _, img := range e.images {
				if _, err := os.Stat(img); err != nil {
					return nil, fmt.Errorf("사진이 없어요: %s", img)
				}
			}
		}
		out = append(out, e)
	}
	// 사진 없는 글은 그날의 남는 번호를 차례로 받는다.
	used := map[int]map[int]bool{}
	for _, e := range out {
		if e.n > 0 {
			if used[e.day] == nil {
				used[e.day] = map[int]bool{}
			}
			if used[e.day][e.n] {
				return nil, fmt.Errorf("day %d: %d번 글이 둘이에요", e.day, e.n)
			}
			used[e.day][e.n] = true
		}
	}
	for i := range out {
		if out[i].n == 0 {
			n := 1
			for used[out[i].day][n] {
				n++
			}
			if used[out[i].day] == nil {
				used[out[i].day] = map[int]bool{}
			}
			used[out[i].day][n] = true
			out[i].n = n
		}
	}
	return out, nil
}

// blocksWithImages 는 평문을 단락, 사진을 이미지 블록으로 잇는다(BlockNote 저장 모양과 같아야 한다).
func blocksWithImages(text string, urls []string) string {
	type inline struct {
		Type   string            `json:"type"`
		Text   string            `json:"text"`
		Styles map[string]string `json:"styles"`
	}
	type blk struct {
		Type     string         `json:"type"`
		Props    map[string]any `json:"props"`
		Content  []inline       `json:"content,omitempty"`
		Children []blk          `json:"children"`
	}
	var out []blk
	for _, ln := range strings.Split(text, "\n") {
		b := blk{Type: "paragraph", Props: map[string]any{}, Content: []inline{}, Children: []blk{}}
		if strings.TrimSpace(ln) != "" {
			b.Content = append(b.Content, inline{Type: "text", Text: strings.TrimRight(ln, " "), Styles: map[string]string{}})
		}
		out = append(out, b)
	}
	for len(out) > 0 && len(out[len(out)-1].Content) == 0 {
		out = out[:len(out)-1]
	}
	for _, u := range urls {
		out = append(out, blk{
			Type: "image",
			Props: map[string]any{
				"url": u, "name": "", "caption": "", "showPreview": true,
				"textAlignment": "left", "backgroundColor": "default",
			},
			Children: []blk{},
		})
	}
	b, _ := json.Marshal(out)
	return string(b)
}
