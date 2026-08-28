// planner — 단아네 플래너 (칸반 · 캘린더 · 주간 루틴).
//
//	planner serve  [--listen 127.0.0.1:8090] [--data DIR] [--log LEVEL] [--dev]
//	planner user add <name>     [--data DIR]   비밀번호는 터미널에서 입력
//	planner user passwd <name>  [--data DIR]
//	planner user del <name>     [--data DIR]
//	planner user list           [--data DIR]
//	planner backup [--out FILE] [--data DIR]   VACUUM INTO 스냅샷
//	planner migrate status      [--data DIR]   적용/대기 마이그레이션 확인 (적용은 serve가 함)
//	planner import notion <zip> [--board N] [--apply]  노션 export 가져오기 (기본 dry-run)
//	planner babyfood import <파일.json> [--apply]      이유식 식단 데이터 넣기 (기본 dry-run)
//
// 데이터 디렉터리는 --data 또는 PLANNER_DATA. Pi에서 CLI는 서비스와 같은 사용자
// (sh5080)로 실행해야 -wal/-shm 파일 소유권이 꼬이지 않는다.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // static binary: carry zoneinfo so TZ=Asia/Seoul resolves without the OS db

	"github.com/sh5080/home-hub/planner/internal/api"
	"github.com/sh5080/home-hub/planner/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = cmdServe(os.Args[2:])
	case "user":
		err = cmdUser(os.Args[2:])
	case "backup":
		err = cmdBackup(os.Args[2:])
	case "migrate":
		err = cmdMigrate(os.Args[2:])
	case "import":
		err = cmdImport(os.Args[2:])
	case "babyfood":
		err = cmdBabyfood(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  planner serve  [--listen ADDR] [--data DIR] [--log LEVEL] [--dev]
  planner user   add|passwd|del|list [name] [--data DIR]
  planner backup [--out FILE] [--data DIR]
  planner migrate status [--data DIR]
  planner import notion <export.zip> [--board NAME] [--apply] [--data DIR]
  planner babyfood import <파일.json> [--track a,b] [--replace] [--apply] [--data DIR]
`)
}

// dataFlag registers --data with the PLANNER_DATA env fallback on fs.
func dataFlag(fs *flag.FlagSet) *string {
	def := os.Getenv("PLANNER_DATA")
	if def == "" {
		def = "./data"
	}
	return fs.String("data", def, "data directory (env PLANNER_DATA)")
}

func newLogger(level string) (*slog.Logger, error) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "info":
		l = slog.LevelInfo
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		return nil, fmt.Errorf("unknown log level %q", level)
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: l})), nil
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	listen := fs.String("listen", "127.0.0.1:8090", "listen address (keep on loopback; tailscale serve fronts it)")
	data := dataFlag(fs)
	logLevel := fs.String("log", "info", "log level: debug | info | warn | error")
	dev := fs.Bool("dev", false, "development mode: session cookie without Secure (for Vite over http://localhost)")
	quotaMB := fs.Int64("quota-mb", 512, "플래너가 쓸 수 있는 상한(MB). DB+백업 합계가 넘으면 새로 만들기를 막는다")
	fs.Parse(args)

	log, err := newLogger(*logLevel)
	if err != nil {
		return err
	}

	st, err := store.Open(*data)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	st.SetQuota(*quotaMB << 20)
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	handler, hub := api.NewWithHub(st, log, *dev)
	srv := &http.Server{
		Addr:              *listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		// slowloris 방지. bcrypt가 최대 ~3초(대기)+1초(해싱)이라 Write는 넉넉히 둔다.
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  90 * time.Second,
		// Request contexts derive from ctx so in-flight handlers observe SIGTERM.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	go func() {
		<-ctx.Done()
		// 스트림을 먼저 끊는다 — Shutdown 은 핸들러가 돌아오기를 기다리므로
		// 열린 SSE가 있으면 5초 마감을 그냥 다 쓴다.
		hub.Close()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	// Expired sessions only matter for table size; sweep hourly.
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				if n, err := st.PurgeExpiredSessions(ctx, now); err != nil {
					log.Warn("purge sessions", "err", err)
				} else if n > 0 {
					log.Debug("purged sessions", "n", n)
				}
				if n, err := st.PurgeLoginAttempts(ctx, now); err != nil {
					log.Warn("purge login attempts", "err", err)
				} else if n > 0 {
					log.Debug("purged login attempts", "n", n)
				}
			}
		}
	}()

	log.Info("planner started", "addr", *listen, "data", *data, "dev", *dev)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Info("planner stopped")
	return nil
}

func cmdBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	data := dataFlag(fs)
	out := fs.String("out", "", "output file (default: <data>/backup-YYYYMMDD-HHMMSS.db)")
	fs.Parse(args)

	st, err := store.Open(*data)
	if err != nil {
		return err
	}
	defer st.Close()

	dst := *out
	if dst == "" {
		dst = fmt.Sprintf("%s/backup-%s.db", *data, time.Now().Format("20060102-150405"))
	}
	if err := st.Backup(context.Background(), dst); err != nil {
		return err
	}
	fmt.Println("backup written:", dst)
	return nil
}
