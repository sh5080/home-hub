package api

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// hub 는 '뭔가 바뀌었다'만 알린다(버전 번호). 클라이언트는 다시 불러온다.
type hub struct {
	version atomic.Uint64
	mu      sync.Mutex
	subs    map[uint64]chan uint64
	nextID  uint64
	closed  bool
}

// perUserSubs 는 사용자당 스트림 수 상한(멈춘 클라이언트의 고루틴 누적 방지).
const perUserSubs = 5

func newHub() *hub { return &hub{subs: map[uint64]chan uint64{}} }

// Broadcast 는 버전을 올리고 구독자를 깨운다. 채널이 막혀 있으면 건너뛴다.
func (h *hub) Broadcast() {
	v := h.version.Add(1)
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subs {
		select {
		case ch <- v:
		default:
		}
	}
}

func (h *hub) subscribe() (id uint64, ch chan uint64, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return 0, nil, false
	}
	h.nextID++
	id = h.nextID
	ch = make(chan uint64, 1)
	h.subs[id] = ch
	return id, ch, true
}

func (h *hub) unsubscribe(id uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.subs[id]; ok {
		delete(h.subs, id)
		close(ch)
	}
}

// Close 는 모든 스트림을 끝낸다. Shutdown 전에 불러야 5초 마감에 안 걸린다.
func (h *hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for id, ch := range h.subs {
		delete(h.subs, id)
		close(ch)
	}
}

// GET /api/stream — 변경 알림(SSE).
// 서버 WriteTimeout(30초)을 이 연결에서만 푼다.
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		s.log.Error("stream: clear write deadline", "err", err)
		writeErr(w, http.StatusInternalServerError, "스트림을 열 수 없어요")
		return
	}

	id, ch, ok := s.hub.subscribe()
	if !ok {
		writeErr(w, http.StatusServiceUnavailable, "종료 중이에요")
		return
	}
	defer s.hub.unsubscribe(id)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	// 프록시(tailscale serve/funnel)가 버퍼링하지 않도록.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// EventSource 는 서버가 정상 종료하면 재연결하지 않는다 — retry 를 알리고 클라이언트도 직접 재연결한다.
	fmt.Fprint(w, "retry: 3000\n\n")
	_ = rc.Flush()

	// 유휴 연결은 인그레스에서 1분쯤 뒤 끊긴다. 25초마다 핑.
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case v, open := <-ch:
			if !open {
				return
			}
			if _, err := fmt.Fprintf(w, "event: changed\ndata: %d\n\n", v); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}

// broadcastOnWrite 는 성공한 변경 요청 뒤에 알린다(새 라우트도 자동 포함).
func (s *Server) broadcastOnWrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec, ok := w.(*statusRecorder)
		if !ok {
			rec = &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			w = rec
		}
		next.ServeHTTP(w, r)
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return
		}
		if rec.status < 400 {
			s.hub.Broadcast()
		}
	})
}
