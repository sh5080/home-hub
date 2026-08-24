package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sh5080/home-hub/planner/internal/notion"
	"github.com/sh5080/home-hub/planner/internal/store"
)

// cmdImport: planner import notion <export.zip> [--board NAME] [--apply] [--data DIR]
//
// 기본은 dry-run이다. 무엇이 들어가고 무엇이 버려지는지 먼저 보여주고,
// --apply를 줘야 실제로 쓴다. 가져오기는 되돌리기 어려운 작업이라 기본값을
// 안전한 쪽에 둔다.
func cmdImport(args []string) error {
	if len(args) < 2 || args[0] != "notion" {
		return errors.New("usage: planner import notion <export.zip> [--board NAME] [--apply] [--data DIR]")
	}
	zipPath := args[1]

	fs := flag.NewFlagSet("import notion", flag.ExitOnError)
	data := dataFlag(fs)
	board := fs.String("board", "노션에서 가져옴", "할 일이 들어갈 보드 이름 (없으면 만든다)")
	apply := fs.Bool("apply", false, "실제로 쓴다. 없으면 dry-run")
	fs.Parse(args[2:])

	ex, err := notion.Parse(zipPath)
	if err != nil {
		return err
	}

	// 칸반과 캘린더가 같은 데이터를 보는 뷰이므로 일정도 카드로 들어간다.
	// 노션의 '선택'(일정/할일)은 어느 컬럼에 놓을지에만 쓴다.
	var events, todos []notion.Item
	for _, it := range ex.Items {
		if it.Kind == notion.KindEvent {
			events = append(events, it)
		} else {
			todos = append(todos, it)
		}
	}

	fmt.Printf("%s\n\n", zipPath)
	fmt.Printf("  일정   %3d건 → 보드 %q (지난 것은 마지막 컬럼)\n", len(events), *board)
	fmt.Printf("  할 일  %3d건 → 보드 %q 의 첫 컬럼\n", len(todos), *board)
	withBody := 0
	for _, it := range ex.Items {
		if it.Body != "" {
			withBody++
		}
	}
	fmt.Printf("  본문   %3d건 (블록으로 변환)\n", withBody)

	if len(ex.Skipped) > 0 {
		fmt.Println("\n가져오지 않는 것:")
		keys := make([]string, 0, len(ex.Skipped))
		for k := range ex.Skipped {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %-36s %d\n", k, ex.Skipped[k])
		}
	}

	if !*apply {
		fmt.Println("\n(dry-run — 실제로 쓰려면 --apply)")
		return nil
	}

	st, err := store.Open(*data)
	if err != nil {
		return err
	}
	defer st.Close()
	ctx := context.Background()

	// 가져오기 전 스냅샷. 잘못되면 이 파일로 되돌린다.
	snap := fmt.Sprintf("%s/before-import.db", *data)
	if err := st.Backup(ctx, snap); err != nil {
		return fmt.Errorf("스냅샷 실패 (가져오기 중단): %w", err)
	}
	fmt.Printf("\n스냅샷: %s\n", snap)

	users, err := st.ListUsers(ctx)
	if err != nil {
		return err
	}
	if len(users) == 0 {
		return errors.New("사용자가 없다 — 먼저 planner user add <이름>")
	}
	by := users[0].ID // 가져온 항목의 작성자

	// 대상 보드: 같은 이름이 있으면 재사용한다(두 번 돌려도 보드가 안 늘어난다).
	boards, err := st.ListBoards(ctx)
	if err != nil {
		return err
	}
	var target store.Board
	for _, b := range boards {
		if b.Name == *board {
			target = b
			break
		}
	}
	if target.ID == 0 {
		target, err = st.CreateBoard(ctx, *board, by)
		if err != nil {
			return err
		}
		fmt.Printf("보드 생성: %s\n", target.Name)
	}
	detail, err := st.GetBoard(ctx, target.ID, store.SortManual)
	if err != nil {
		return err
	}
	if len(detail.Columns) == 0 {
		return errors.New("보드에 컬럼이 없다")
	}
	firstCol := detail.Columns[0].ID
	existing := map[string]bool{}
	for _, c := range detail.Columns {
		for _, card := range c.Cards {
			existing[card.Title] = true
		}
	}

	var okCards, okEvents, dup int
	for _, it := range todos {
		if existing[it.Title] {
			dup++
			continue
		}
		content := buildBody(it)
		in := store.CardInput{Title: &it.Title}
		if content != "" {
			in.Content = &content
		}
		if it.Start != "" {
			in.DueAt = &it.Start // 시각이 있으면 시각까지 보존한다
		}
		if it.Priority > 0 {
			p := it.Priority
			in.Priority = &p
		}
		if _, err := st.CreateCard(ctx, firstCol, in, by); err != nil {
			return fmt.Errorf("카드 %q: %w", it.Title, err)
		}
		okCards++
	}

	lastCol := detail.Columns[len(detail.Columns)-1].ID
	todayStr := time.Now().Format("2006-01-02")
	for _, it := range events {
		if existing[it.Title] {
			dup++
			continue
		}
		// 지난 일정을 첫 컬럼에 넣으면 '연체된 할 일'로 보인다 — 날짜로 가른다.
		col := firstCol
		if it.Start < todayStr {
			col = lastCol
		}
		content := buildBody(it)
		in := store.CardInput{Title: &it.Title, DueAt: &it.Start}
		if it.End != "" {
			in.EndAt = &it.End
		}
		if content != "" {
			in.Content = &content
		}
		if _, err := st.CreateCard(ctx, col, in, by); err != nil {
			return fmt.Errorf("일정 %q: %w", it.Title, err)
		}
		okEvents++
	}

	fmt.Printf("\n완료 — 할 일 %d, 일정 %d (모두 카드)", okCards, okEvents)
	if dup > 0 {
		fmt.Printf(", 이미 있어 건너뜀 %d", dup)
	}
	fmt.Println()
	return nil
}

// buildBody는 카드/일정의 본문을 만든다. 노션 본문 앞에, 우리 스키마에 자리가
// 없는 속성(분류·내용)을 콜아웃으로 남긴다 — 버리지 않고 눈에 보이게.
func buildBody(it notion.Item) string {
	var head []string
	if it.Category != "" {
		head = append(head, "분류: "+it.Category)
	}
	// 중요도는 priority 컬럼으로 들어간다 — 본문에 중복해 쓰지 않는다.
	if it.Note != "" {
		head = append(head, it.Note)
	}

	body := notion.MarkdownToBlocks(it.Body)
	if len(head) == 0 {
		return body
	}

	callout := map[string]any{
		"type":  "callout",
		"props": map[string]any{"emoji": "📥", "tone": "gray"},
		"content": []map[string]any{
			{"type": "text", "text": strings.Join(head, " · "), "styles": map[string]bool{}},
		},
		"children": []any{},
	}
	blocks := []any{callout}
	if body != "" {
		var rest []any
		if err := json.Unmarshal([]byte(body), &rest); err == nil {
			blocks = append(blocks, rest...)
		}
	}
	b, err := json.Marshal(blocks)
	if err != nil {
		return body
	}
	return string(b)
}
