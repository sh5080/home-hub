// Package api wires HTTP routes to the store.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/sh5080/home-hub/planner/internal/auth"
	"github.com/sh5080/home-hub/planner/internal/notify"
	"github.com/sh5080/home-hub/planner/internal/store"
	"github.com/sh5080/home-hub/planner/internal/webui"
)

// Server holds handler dependencies.
type Server struct {
	st  *store.Store
	log *slog.Logger
	dev bool // drops the Secure cookie flag so Vite's http://localhost works
	// bcrypt 는 Pi 에서 ~1초. 동시 실행을 묶는다.
	bcrypt bcryptLimiter
	hub    *hub
	// 사진 디코딩은 한 번에 하나(메모리).
	mediaSem chan struct{}
	// 웹푸시. 1분 알림 루프도 이것이 돈다.
	push *notify.Sender
}

// New 는 /api/* 다음 SPA 순서로 묶는다. /api/ catch-all 404 가 먼저라 모르는 API 경로가 index.html 로 가지 않는다.
func New(st *store.Store, log *slog.Logger, dev bool) http.Handler {
	h, _ := NewWithHub(st, log, dev)
	return h
}

// NewWithHub 는 알림 허브를 같이 준다. 종료 시 스트림을 먼저 끊어야 Shutdown 마감에 안 걸린다.
func NewWithHub(st *store.Store, log *slog.Logger, dev bool) (http.Handler, interface{ Close() }) {
	s := &Server{st: st, log: log, dev: dev, bcrypt: newBcryptLimiter(), hub: newHub(), mediaSem: make(chan struct{}, 1), push: notify.New(st, log)}
	devCSP = dev
	mux := http.NewServeMux()

	// --- public ---
	mux.HandleFunc("GET /api/healthz", s.healthz)
	mux.HandleFunc("POST /api/login", s.login)

	// --- authenticated --- (인증 mux 하나에 모으고 바깥 mux 가 경로를 넘긴다)
	authed := http.NewServeMux()
	authed.HandleFunc("POST /api/logout", s.logout)
	authed.HandleFunc("GET /api/me", s.me)
	authed.HandleFunc("GET /api/users", s.listUsers)
	authed.HandleFunc("POST /api/users", s.createUser)
	authed.HandleFunc("POST /api/users/{id}/password", s.setPassword)
	authed.HandleFunc("PATCH /api/users/{id}/birthdate", s.setBirthDate)
	s.registerBoards(authed)
	s.registerCalendar(authed)
	s.registerRoutines(authed)
	s.registerBabyfood(authed)
	s.registerDiary(authed)
	s.registerMedia(authed)
	s.registerCare(authed)
	s.registerNotify(authed)
	s.registerFinance(authed)
	authed.HandleFunc("GET /api/stream", s.stream)
	authed.HandleFunc("GET /api/storage", s.storage)
	authed.HandleFunc("GET /api/search", s.search)
	authed.HandleFunc("/", notFound) // inner mux must also answer JSON, never the stdlib HTML 404
	// 변경 알림은 미들웨어 한 곳에서.
	gated := auth.Middleware(st, s.broadcastOnWrite(authed))
	// 바깥 mux 에도 경로를 얹어야 인증 mux 까지 간다. 빠뜨리면 catch-all 404 가 먼저 잡는다.
	for _, p := range []string{
		"/api/logout", "/api/me", "/api/users", "/api/users/",
		"/api/boards", "/api/boards/", "/api/columns/", "/api/cards/",
		"/api/routines", "/api/routines/", "/api/calendar", "/api/today", "/api/stream", "/api/storage", "/api/search",
		"/api/babyfood", "/api/babyfood/",
		"/api/diary", "/api/diary/", "/api/media", "/api/media/", "/api/care", "/api/care/",
		"/api/push/", "/api/notify/", "/api/finance/",
		// /api/ 밖의 유일한 인증 경로. SPA 핸들러보다 먼저 잡혀야 한다.
		"/media/",
	} {
		mux.Handle(p, gated)
	}

	// --- catch-all: unknown API path is JSON 404, never HTML ---
	mux.HandleFunc("/api/", notFound)

	// --- SPA (must be last) ---
	mux.Handle("/", webui.Handler())

	// 알림 루프. Close 때 같이 멈춘다.
	ctx, cancel := context.WithCancel(context.Background())
	go s.push.Run(ctx)
	return securityHeaders(s.logRequests(mux)), closers{s.hub, closeFunc(cancel)}
}

type closeFunc func()

func (f closeFunc) Close() { f() }

type closers []interface{ Close() }

func (cs closers) Close() {
	for _, c := range cs {
		c.Close()
	}
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeErr(w, http.StatusNotFound, "not found")
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.st.Ping(ctx); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "db: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// logRequests 는 일부러 짧다 — Pi 저널이 50MB 로 묶여 있다.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if rec.status >= 400 || s.log.Enabled(r.Context(), slog.LevelDebug) {
			s.log.Log(r.Context(), levelFor(rec.status), "http",
				"method", r.Method, "path", r.URL.Path, "status", rec.status,
				"ms", time.Since(start).Milliseconds())
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach Flush/Hijack on the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func levelFor(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelDebug
	}
}
