package api

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// hub는 "뭔가 바뀌었다"를 구독자에게 알린다.
//
// 무엇이 바뀌었는지는 보내지 않는다 — 클라이언트는 그냥 다시 불러오면 되고,
// 변경 종류를 프로토콜에 넣으면 핸들러를 추가할 때마다 양쪽을 고쳐야 한다.
// 버전 번호만 올린다.
type hub struct {
	version atomic.Uint64
	mu      sync.Mutex
	subs    map[uint64]chan uint64
	nextID  uint64
	closed  bool
}

// perUserSubs는 한 사용자가 열 수 있는 스트림 수다. 탭 여러 개는 정상이지만
// 멈춘 클라이언트가 고루틴을 무한정 쌓는 건 막는다.
const perUserSubs = 5

func newHub() *hub { return &hub{subs: map[uint64]chan uint64{}} }

// Broadcast는 버전을 올리고 구독자를 깨운다. 채널이 막혀 있으면 건너뛴다 —
// 어차피 다음 알림이 따라잡는다.
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

// Close는 모든 스트림을 끝낸다. srv.Shutdown 이 핸들러가 돌아오기를 기다리므로
// 그 전에 불러야 종료가 5초 마감에 걸리지 않는다.
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
//
// 30초마다 끊기던 이유: http.Server 의 WriteTimeout 이 slowloris 대비로 30초다.
// 그 값을 전역에서 풀면 다른 라우트가 노출되므로, 이 연결의 마감만 해제한다.
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

	// EventSource 는 서버가 정상 종료하면 자동 재연결하지 않는다 — 재시도
	// 간격을 알려두고, 클라이언트도 직접 재연결한다.
	fmt.Fprint(w, "retry: 3000\n\n")
	_ = rc.Flush()

	// 유휴 TCP 연결은 인그레스에서 1분쯤 뒤에 끊긴다. 25초마다 주석을 보낸다.
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

// broadcastOnWrite는 성공한 변경 요청 뒤에 알림을 보낸다.
//
// 핸들러마다 호출을 넣으면 새 핸들러에서 빠뜨린다. 미들웨어 한 곳에 두면
// 앞으로 추가되는 라우트도 자동으로 포함된다.
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
