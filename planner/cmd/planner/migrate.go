package main

import (
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/sh5080/home-hub/planner/internal/store"
)

// cmdMigrate: planner migrate status [--data DIR]
//
// `serve` applies pending migrations on startup; this only inspects. It's the
// thing to run before a deploy you're unsure about, and after one that failed.
func cmdMigrate(args []string) error {
	if len(args) < 1 || args[0] != "status" {
		return errors.New("usage: planner migrate status [--data DIR]")
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
