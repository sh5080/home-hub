package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/sh5080/home-hub/planner/internal/auth"
	"github.com/sh5080/home-hub/planner/internal/notify"
	"github.com/sh5080/home-hub/planner/internal/store"
)

func (s *Server) registerNotify(m *http.ServeMux) {
	m.HandleFunc("GET /api/push/key", s.pushKey)
	m.HandleFunc("POST /api/push/subscribe", s.pushSubscribe)
	m.HandleFunc("POST /api/push/unsubscribe", s.pushUnsubscribe)
	m.HandleFunc("POST /api/push/test", s.pushTest)
	m.HandleFunc("GET /api/notify/kinds", s.notifyKinds)
	m.HandleFunc("GET /api/notify/rules", s.notifyRules)
	m.HandleFunc("POST /api/notify/rules", s.notifySave)
	m.HandleFunc("DELETE /api/notify/rules/{id}", s.notifyDelete)
}

func (s *Server) pushKey(w http.ResponseWriter, r *http.Request) {
	pub, _, err := s.push.Keys(r.Context())
	if s.storeErr(w, err, "push key") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"public_key": pub})
}

type pushSubReq struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	Device string `json:"device"`
}

func (s *Server) pushSubscribe(w http.ResponseWriter, r *http.Request) {
	var req pushSubReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	if s.storeErr(w, s.st.PushSubscribe(r.Context(), store.PushSub{
		Endpoint: req.Endpoint, UserID: u.ID, P256dh: req.Keys.P256dh, Auth: req.Keys.Auth, Device: strings.TrimSpace(req.Device),
	}), "push subscribe") {
		return
	}
	// 푸시 서버는 보내는 쪽 연락처(sub)를 요구한다. 코드에 박지 않고 구독 요청의 호스트를 쓴다.
	if host := r.Host; host != "" && !strings.HasPrefix(host, "127.") && !strings.HasPrefix(host, "localhost") {
		_ = s.st.KVSet(r.Context(), "push_subject", "https://"+host)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) pushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var req pushSubReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.storeErr(w, s.st.PushUnsubscribe(r.Context(), req.Endpoint), "push unsubscribe") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/push/test — 누른 사람의 기기로 하나 보낸다.
func (s *Server) pushTest(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	n, err := s.push.Send(r.Context(), []int64{u.ID}, notify.Payload{
		Title: "알림이 잘 와요", Body: u.Name + "님 기기로 보낸 시험 알림이에요.", URL: "/", Tag: "test",
	})
	if errors.Is(err, notify.ErrNoDevice) {
		writeErr(w, http.StatusBadRequest, "이 계정에 알림을 켠 기기가 없어요")
		return
	}
	if err != nil {
		s.log.Warn("push test", "err", err)
		writeErr(w, http.StatusBadGateway, "보내지 못했어요: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"devices": n})
}

func (s *Server) notifyKinds(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sources": store.NotifySources, "conds": store.NotifyConds})
}

func (s *Server) notifyRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.st.NotifyRules(r.Context())
	if s.storeErr(w, err, "notify rules") {
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

func (s *Server) notifySave(w http.ResponseWriter, r *http.Request) {
	var req store.NotifyRule
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	rule, err := s.st.NotifySave(r.Context(), req, u.ID)
	if s.storeErr(w, err, "notify save") {
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) notifyDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.storeErr(w, s.st.NotifyDelete(r.Context(), id), "notify delete") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
