package api

import (
	"context"
	"net/http"
	"time"
)

// bcryptSlots는 동시에 돌릴 수 있는 비밀번호 비교 수다.
//
// 왜 필요한가: 레이트리밋은 '확인 → bcrypt → 기록' 순서라 동시 요청이 전부
// 확인 단계를 통과한 뒤에야 기록이 남는다. 즉 순차 요청만 막힌다. Pi 3B에서
// bcrypt cost 10은 한 번에 ~1초를 쓰므로, 병렬 20개면 4코어가 통째로 묶인다.
// 세마포어로 동시 실행을 제한해 CPU를 지킨다 — 레이트리밋이 막지 못하는
// 유일한 축이다.
const bcryptSlots = 2

// bcryptWait는 슬롯을 기다리는 최대 시간. 넘으면 503으로 빠르게 거절해
// 큐가 무한정 길어지지 않게 한다.
const bcryptWait = 3 * time.Second

type bcryptLimiter chan struct{}

func newBcryptLimiter() bcryptLimiter {
	return make(bcryptLimiter, bcryptSlots)
}

// acquire는 슬롯을 잡는다. 실패하면 호출자가 503을 쓰고 끝내야 한다.
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
