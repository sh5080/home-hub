// Package health serves a minimal HTTP liveness endpoint so the service can be
// probed by systemd or an external monitor.
package health

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// Server exposes GET /healthz.
type Server struct {
	addr string
	log  *slog.Logger
}

// New builds a health server listening on addr (e.g. ":8086").
func New(addr string, log *slog.Logger) *Server {
	return &Server{addr: addr, log: log}
}

// Name identifies the component.
func (s *Server) Name() string { return "health" }

// Start serves until ctx is cancelled, then shuts down gracefully.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	srv := &http.Server{Addr: s.addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	s.log.Info("health endpoint listening", "addr", s.addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
