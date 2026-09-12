package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/sh5080/home-hub/planner/internal/store"
)

// planner finance tag (--name 무신사 | --cat 식사) --tags 생활비,의류 [--apply]
// 내용에 이름이 든(또는 분류가 같은) 거래에 태그를 더한다(있던 태그는 둔다). --apply 가 없으면 찾기만.
func cmdFinance(args []string) error {
	if len(args) == 0 || args[0] != "tag" {
		return errors.New("usage: planner finance tag (--name <이름> | --cat <분류>) --tags <태그,태그> [--apply] [--data DIR]")
	}
	fs := flag.NewFlagSet("finance tag", flag.ExitOnError)
	data := dataFlag(fs)
	name := fs.String("name", "", "거래 내용에 들어 있는 글자(대소문자 무시)")
	cat := fs.String("cat", "", "분류(대분류)가 이것인 거래")
	tags := fs.String("tags", "", "더할 태그 이름, 쉼표로")
	apply := fs.Bool("apply", false, "실제로 붙인다. 없으면 찾기만")
	fs.Parse(args[1:])
	st, err := store.Open(*data)
	if err != nil {
		return err
	}
	defer st.Close()
	found, picked, err := st.FinanceTagByName(context.Background(), *name, *cat, strings.Split(*tags, ","), *apply)
	if err != nil {
		return err
	}
	var sum int64
	for _, m := range found {
		fmt.Printf("%s  %10d  %s\n", m.At, -m.Amount, m.Content)
		sum += -m.Amount
	}
	var names []string
	for _, t := range picked {
		names = append(names, t.Name)
	}
	verb := "찾음(붙이지 않음 — --apply)"
	if *apply {
		verb = "태그 붙임"
	}
	fmt.Printf("\n%d건, 합 %d원 → [%s] %s\n", len(found), sum, strings.Join(names, ", "), verb)
	return nil
}
