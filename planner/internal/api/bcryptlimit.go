package api

import (
	"context"
	"net/http"
	"time"
)

// bcryptSlots 는 동시 bcrypt 수. 레이트리밋은 '확인→bcrypt→기록' 순서라 병렬 요청을 못 막는다.
// Pi 에서 bcrypt 한 번이 ~1초라 세마포어로 CPU 를 지킨다.
const bcryptSlots = 2

// bcryptWait 를 넘으면 503 으로 빠르게 거절한다.
const bcryptWait = 3 * time.Second

type bcryptLimiter chan struct{}

func newBcryptLimiter() bcryptLimiter {
	return make(bcryptLimiter, bcryptSlots)
}

// acquire 가 실패하면 호출자가 503 을 쓰고 끝낸다.
func (l bcryptLimiter) acquire(ctx context.Context) bool {
	t := time.NewTimer(bcryptWait)
	defer t.Stop()
	select {
	case l <- struct{}{}:
		return true
	case <-t.C:
		return false
	case <-ctx.Done():
		return false
	}
}

func (l bcryptLimiter) release() { <-l }

// withBcryptSlot은 핸들러 본문을 슬롯 안에서 돌린다.
func (s *Server) withBcryptSlot(w http.ResponseWriter, r *http.Request, fn func()) {
	if !s.bcrypt.acquire(r.Context()) {
		w.Header().Set("Retry-After", "5")
		writeErr(w, http.StatusServiceUnavailable, "서버가 잠시 바빠요. 다시 시도해주세요")
		return
	}
	defer s.bcrypt.release()
	fn()
}
