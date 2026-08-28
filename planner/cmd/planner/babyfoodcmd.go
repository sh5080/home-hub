package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sh5080/home-hub/planner/internal/store"
)

// cmdBabyfood: planner babyfood import <파일.json> [--track a,b] [--from-dday N] [--replace] [--apply]
//
// 식단 데이터는 파일로만 들어온다. 저장소에 두지 않으므로 Pi에서 파일을
// 올린 뒤 한 번 실행한다. 기본은 dry-run — 무엇이 들어가는지 먼저 보여준다.
func cmdBabyfood(args []string) error {
	if len(args) < 2 || args[0] != "import" {
		return errors.New("usage: planner babyfood import <파일.json> [--track a,b] [--from-dday N] [--replace] [--apply] [--data DIR]")
	}
	path := args[1]

	fs := flag.NewFlagSet("babyfood import", flag.ExitOnError)
	data := dataFlag(fs)
	track := fs.String("track", "", "쓸 구간 id를 쉼표로 (비우면 파일의 기본 구성)")
	replace := fs.Bool("replace", false, "이미 들어 있는 날도 덮어쓴다 (손으로 고친 내용이 날아간다)")
	from := fs.Int("from-dday", 0, "이 생후 일수부터만 다룬다 (구성을 도중에 바꿀 때)")
	apply := fs.Bool("apply", false, "실제로 쓴다. 없으면 dry-run")
	fs.Parse(args[2:])

	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	f, err := store.ParseBFPlan(raw)
	if err != nil {
		return err
	}

	var ids []string
	if *track != "" {
		for _, s := range strings.Split(*track, ",") {
			if s = strings.TrimSpace(s); s != "" {
				ids = append(ids, s)
			}
		}
	}

	st, err := store.Open(*data)
	if err != nil {
		return err
	}
	defer st.Close()

	ctx := context.Background()
	plan, err := st.BFImport(ctx, f, ids, *apply, *replace, *from)
	if err != nil {
		return err
	}

	fmt.Printf("%s\n\n", path)
	for _, s := range plan.Sections {
		line := fmt.Sprintf("  %-18s D+%d~D+%d  %3d일", s.Label, s.From, s.To, s.Days)
		if s.Skip > 0 {
			line += fmt.Sprintf("  (%d일은 이미 있어서 건너뜀)", s.Skip)
		}
		fmt.Println(line)
	}
	fmt.Printf("\n  새로 넣을 날  %3d일 / %d끼\n", plan.NewDays, plan.NewMeals)
	fmt.Printf("  재료 분류    %3d종\n", plan.Ingredients)
	if plan.Existing > 0 && !*replace {
		fmt.Printf("  건너뛴 날    %3d일  (--replace 를 주면 덮어쓴다)\n", plan.Existing)
	}
	if !*apply {
		fmt.Println("\ndry-run 입니다. 실제로 넣으려면 --apply 를 붙이세요.")
		return nil
	}
	fmt.Println("\n넣었습니다. 아이 생일은 앱의 설정에서 입력하세요.")
	return nil
}
