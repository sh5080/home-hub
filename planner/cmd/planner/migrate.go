package main

import (
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/sh5080/home-hub/planner/internal/store"
)

// planner migrate status — 적용 없이 확인만(적용은 serve 가 한다).
// planner migrate accept NAME — 적용된 파일의 주석만 고쳤을 때 기록된 체크섬을 맞춘다.
func cmdMigrate(args []string) error {
	if len(args) >= 2 && args[0] == "accept" {
		fs := flag.NewFlagSet("migrate accept", flag.ExitOnError)
		data := dataFlag(fs)
		fs.Parse(args[2:])
		old, now, err := store.AcceptChecksum(*data, args[1])
		if err != nil {
			return err
		}
		fmt.Printf("%s: %s → %s\n", args[1], old[:8], now[:8])
		return nil
	}
	if len(args) < 1 || args[0] != "status" {
		return errors.New("usage: planner migrate status|accept NAME [--data DIR]")
	}
	fs := flag.NewFlagSet("migrate status", flag.ExitOnError)
	data := dataFlag(fs)
	fs.Parse(args[1:])

	applied, pending, err := store.Status(*data)
	if err != nil {
		return err
	}
	problem := false
	fmt.Printf("database: %s/planner.db\n\n", *data)
	fmt.Println("applied:")
	if len(applied) == 0 {
		fmt.Println("  (none)")
	}
	for _, a := range applied {
		flag := ""
		switch {
		case a.Modified:
			flag = "  !! MODIFIED after apply — serve will refuse to start"
			problem = true
		case a.Missing:
			flag = "  ?? not in this binary (db is newer than this build)"
		}
		fmt.Printf("  %04d  %-28s %s  %s%s\n", a.Version, a.Name, a.Checksum[:8], time.Unix(a.AppliedAt, 0).Format("2006-01-02 15:04"), flag)
	}
	fmt.Println("\npending:")
	if len(pending) == 0 {
		fmt.Println("  (none — schema is current)")
	}
	for _, p := range pending {
		fmt.Printf("  %04d  %-28s %s\n", p.Version, p.Name, p.Checksum[:8])
	}
	if problem {
		return errors.New("schema problem detected")
	}
	return nil
}
