// Package api wires HTTP routes to the store.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/sh5080/home-hub/planner/internal/auth"
	"github.com/sh5080/home-hub/planner/internal/store"
	"github.com/sh5080/home-hub/planner/internal/webui"
)

// Server holds handler dependencies.
type Server struct {
	st  *store.Store
	log *slog.Logger
	dev bool // drops the Secure cookie flag so Vite's http://localhost works
	// bcrypt는 Pi에서 한 번에 ~1초를 쓴다. 동시 실행을 묶어 CPU 고갈을 막는다.
	bcrypt bcryptLimiter
}

// New builds the full handler: /api/* routes, then the SPA. Route order matters:
// the /api/ catch-all 404 is registered so an unknown API path can never fall
// through to index.html.
func New(st *store.Store, log *slog.Logger, dev bool) http.Handler {
	s := &Server{st: st, log: log, dev: dev, bcrypt: newBcryptLimiter()}
	devCSP = dev
	mux := http.NewServeMux()

	// --- public ---
	mux.HandleFunc("GET /api/healthz", s.healthz)
	mux.HandleFunc("POST /api/login", s.login)

	// --- authenticated ---
	// Auth-gated routes live on their own mux so one middleware covers them
	// all; the outer mux forwards every /api/ path we register here.
	authed := http.NewServeMux()
	authed.HandleFunc("POST /api/logout", s.logout)
	authed.HandleFunc("GET /api/me", s.me)
	authed.HandleFunc("GET /api/users", s.listUsers)
	authed.HandleFunc("POST /api/users", s.createUser)
	authed.HandleFunc("POST /api/users/{id}/password", s.setPassword)
	s.registerBoards(authed)
	s.registerEvents(authed)
	s.registerRoutines(authed)
	authed.HandleFunc("/", notFound) // inner mux must also answer JSON, never the stdlib HTML 404
	gated := auth.Middleware(st, authed)
	for _, p := range []string{
		"/api/logout", "/api/me", "/api/users", "/api/users/",
		"/api/boards", "/api/boards/", "/api/columns/", "/api/cards/",
		"/api/events", "/api/events/", "/api/routines", "/api/routines/", "/api/calendar", "/api/today",
	} {
		mux.Handle(p, gated)
	}

	// --- catch-all: unknown API path is JSON 404, never HTML ---
	mux.HandleFunc("/api/", notFound)

	// --- SPA (must be last) ---
	mux.Handle("/", webui.Handler())

	return securityHeaders(s.logRequests(mux))
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

// logRequests is deliberately terse — the Pi journal is capped at 50 MB.
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
