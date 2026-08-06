package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/sh5080/home-hub/planner/internal/store"
)

// cmdUser: planner user add|passwd|list [name] [--data DIR]
func cmdUser(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: planner user add|passwd|del|list [name] [--data DIR]")
	}
	sub := args[0]
	fs := flag.NewFlagSet("user "+sub, flag.ExitOnError)
	data := dataFlag(fs)

	// flag stops at the first positional, so re-parse past it: this accepts
	// `user add NAME --data DIR`, `user add --data DIR NAME`, and `user list --data DIR`.
	var name string
	remaining := args[1:]
	for {
		fs.Parse(remaining)
		if fs.NArg() == 0 {
			break
		}
		if name == "" {
			name = fs.Arg(0)
		}
		remaining = fs.Args()[1:]
	}

	st, err := store.Open(*data)
	if err != nil {
		return err
	}
	defer st.Close()
	ctx := context.Background()

	switch sub {
	case "add":
		if name == "" {
			return errors.New("usage: planner user add <name>")
		}
		pw, err := promptPassword(true)
		if err != nil {
			return err
		}
		u, err := st.CreateUser(ctx, name, pw)
		if errors.Is(err, store.ErrConflict) {
			return fmt.Errorf("user %q already exists", name)
		}
		if err != nil {
			return err
		}
		fmt.Printf("created user %q (id %d)\n", u.Name, u.ID)
		return nil

	case "passwd":
		if name == "" {
			return errors.New("usage: planner user passwd <name>")
		}
		pw, err := promptPassword(true)
		if err != nil {
			return err
		}
		if err := st.SetPassword(ctx, name, pw); errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("no user %q", name)
		} else if err != nil {
			return err
		}
		fmt.Printf("password updated for %q\n", name)
		return nil

	case "del":
		if name == "" {
			return errors.New("usage: planner user del <name>")
		}
		if err := st.DeleteUser(ctx, name); errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("no user %q", name)
		} else if err != nil {
			return err
		}
		fmt.Printf("deleted user %q\n", name)
		return nil

	case "list":
		users, err := st.ListUsers(ctx)
		if err != nil {
			return err
		}
		if len(users) == 0 {
			fmt.Println("(no users — run: planner user add <name>)")
			return nil
		}
		for _, u := range users {
			fmt.Printf("%3d  %-16s  %s\n", u.ID, u.Name, time.Unix(u.CreatedAt, 0).Format("2006-01-02"))
		}
		return nil

	default:
		return fmt.Errorf("unknown user subcommand %q (add|passwd|del|list)", sub)
	}
}

// promptPassword reads a password without echo when stdin is a terminal,
// falling back to a plain line read (pipes, CI). confirm asks twice.
func promptPassword(confirm bool) (string, error) {
	stdin := bufio.NewReader(os.Stdin) // one reader: a fresh one per read would swallow the 2nd line
	read := func(label string) (string, error) {
		fmt.Fprint(os.Stderr, label)
		if term.IsTerminal(int(syscall.Stdin)) {
			b, err := term.ReadPassword(int(syscall.Stdin))
			fmt.Fprintln(os.Stderr)
			return string(b), err
		}
		line, err := stdin.ReadString('\n')
		if err != nil && line == "" {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	pw, err := read("password: ")
	if err != nil {
		return "", err
	}
	if confirm {
		again, err := read("again: ")
		if err != nil {
			return "", err
		}
		if pw != again {
			return "", errors.New("passwords do not match")
		}
	}
	return pw, nil
}
