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

// planner babyfood import <파일.json> [--track a,b] [--from-dday N] [--replace] [--apply] (기본 dry-run)
func cmdBabyfood(args []string) error {
	if len(args) >= 2 && args[0] == "adopt" {
		return cmdBabyfoodAdopt(args[1:])
	}
	if len(args) < 3 || args[0] != "import" {
		return errors.New("usage:\n" +
			"  planner babyfood import <아이 이름> <파일.json> [--track a,b] [--from-dday N] [--replace] [--apply] [--data DIR]\n" +
			"  planner babyfood adopt  <아이 이름> [--apply] [--data DIR]   주인 없는 기존 자료를 이 아이에게 연결")
	}
	childName := args[1]
	path := args[2]

	fs := flag.NewFlagSet("babyfood import", flag.ExitOnError)
	data := dataFlag(fs)
	track := fs.String("track", "", "쓸 구간 id를 쉼표로 (비우면 파일의 기본 구성)")
	replace := fs.Bool("replace", false, "이미 들어 있는 날도 덮어쓴다 (손으로 고친 내용이 날아간다)")
	from := fs.Int("from-dday", 0, "이 생후 일수부터만 다룬다 (구성을 도중에 바꿀 때)")
	apply := fs.Bool("apply", false, "실제로 쓴다. 없으면 dry-run")
	fs.Parse(args[3:])

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
	child, err := findChild(ctx, st, childName)
	if err != nil {
		return err
	}
	plan, err := st.BFImport(ctx, child.UserID, f, ids, *apply, *replace, *from)
	if err != nil {
		return err
	}

	fmt.Printf("%s → %s\n\n", path, child.Name)
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

// findChild 는 이름으로 이유식 대상 아이를 찾는다.
func findChild(ctx context.Context, st *store.Store, name string) (store.BFChild, error) {
	kids, err := st.BFChildren(ctx)
	if err != nil {
		return store.BFChild{}, err
	}
	var names []string
	for _, k := range kids {
		if k.Name == name {
			return k, nil
		}
		names = append(names, k.Name)
	}
	if len(names) == 0 {
		return store.BFChild{}, errors.New("이유식 대상이 없습니다. 앱의 설정 → 이유식에서 먼저 지정하세요")
	}
	return store.BFChild{}, fmt.Errorf("%q 는 이유식 대상이 아닙니다. 현재 대상: %s", name, strings.Join(names, ", "))
}

// cmdBabyfoodAdopt 는 child_id 없는 자료를 한 아이에게 붙인다.
func cmdBabyfoodAdopt(args []string) error {
	fs := flag.NewFlagSet("babyfood adopt", flag.ExitOnError)
	data := dataFlag(fs)
	apply := fs.Bool("apply", false, "실제로 연결한다. 없으면 확인만")
	fs.Parse(args[1:])

	st, err := store.Open(*data)
	if err != nil {
		return err
	}
	defer st.Close()
	ctx := context.Background()
	child, err := findChild(ctx, st, args[0])
	if err != nil {
		return err
	}
	if !*apply {
		fmt.Printf("주인 없는 자료를 %s 에게 연결합니다. 실제로 하려면 --apply 를 붙이세요.\n", child.Name)
		return nil
	}
	n, err := st.BFAdoptOrphans(ctx, child.UserID)
	if err != nil {
		return err
	}
	fmt.Printf("%s 에게 %d일치 식단을 연결했습니다.\n", child.Name, n)
	return nil
}
