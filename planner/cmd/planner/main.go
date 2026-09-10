// planner — 단아네 플래너. 서브커맨드는 usage() 참고.
//
// 데이터 디렉터리는 --data 또는 PLANNER_DATA. Pi 에서 CLI 는 서비스와 같은 사용자(sh5080)로
// 실행해야 -wal/-shm 소유권이 꼬이지 않는다.
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
	"path/filepath"
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
	case "diary":
		err = cmdDiary(os.Args[2:])
	case "care":
		err = cmdCare(os.Args[2:])
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
  planner diary import <폴더>... --as <이름> [--child <이름>] [--apply] [--data DIR]   베이비타임 일기
  planner care import <폴더>... --child <이름> --as <이름> [--apply] [--data DIR]    베이비타임 활동 기록
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
	// 재정 파일 규격(서비스 이름·시트 이름 등)은 저장소 밖 설정 파일에 둔다.
	finPath := os.Getenv("PLANNER_FIN_FORMAT")
	if finPath == "" {
		finPath = filepath.Join(*data, "finance-format.json")
	}
	ff, err := store.LoadFinFormat(finPath)
	if err != nil {
		return fmt.Errorf("재정 규격: %w", err)
	}
	if ff == nil {
		log.Info("finance format not configured; import disabled", "path", finPath)
	}
	st.SetFinFormat(ff)
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	handler, hub := api.NewWithHub(st, log, *dev)
	srv := &http.Server{
		Addr:              *listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		// slowloris 방지. bcrypt 가 최대 ~4초라 Write 는 넉넉히.
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  90 * time.Second,
		BaseContext:  func(net.Listener) context.Context { return ctx },
	}

	go func() {
		<-ctx.Done()
		// 스트림을 먼저 끊는다 — 열린 SSE 가 있으면 Shutdown 이 5초 마감을 다 쓴다.
		hub.Close()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

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
