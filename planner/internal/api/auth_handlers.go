package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/sh5080/home-hub/planner/internal/auth"
	"github.com/sh5080/home-hub/planner/internal/store"
)

type loginReq struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

// POST /api/login
//
// 공개(funnel) 가능하니 로그인이 방어선이다. 레이트리밋 세 겹: 계정 연속 실패 잠금,
// IP 창당 시도 상한(bcrypt 한 번이 ~1초라 그 자체가 DoS 벡터), 없는 이름 반복 시 IP 차단.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if !decodeJSON(w, r, &req) {
		return
	}
	s.withBcryptSlot(w, r, func() { s.loginLocked(w, r, req) })
}

func (s *Server) loginLocked(w http.ResponseWriter, r *http.Request, req loginReq) {
	ip := clientIP(r)
	now := time.Now()

	gate, err := s.st.CheckLogin(r.Context(), req.Name, ip, now)
	if err != nil {
		s.log.Error("check login", "err", err)
		writeErr(w, http.StatusInternalServerError, "로그인 처리 중 오류")
		return
	}

	// 막혔어도 비밀번호 비교는 돌린다 — 응답 시간으로 잠금 상태가 드러나지 않게.
	u, ok, err := s.st.Authenticate(r.Context(), req.Name, req.Password)
	if err != nil {
		s.log.Error("authenticate", "err", err)
		writeErr(w, http.StatusInternalServerError, "로그인 처리 중 오류")
		return
	}

	if !gate.Allowed {
		s.log.Warn("login blocked", "name", req.Name, "ip", ip, "retry_s", int(gate.Retry.Seconds()))
		w.Header().Set("Retry-After", strconv.Itoa(int(gate.Retry.Seconds())+1))
		writeErr(w, http.StatusTooManyRequests, "시도가 너무 많아요. "+humanDuration(gate.Retry)+" 후에 다시 시도해주세요")
		return
	}

	outcome := store.AttemptWrongPass
	switch {
	case ok:
		outcome = store.AttemptSuccess
	case !s.st.UserExists(r.Context(), req.Name):
		outcome = store.AttemptUnknownUser
	}
	if err := s.st.RecordAttempt(r.Context(), req.Name, ip, outcome, now); err != nil {
		s.log.Warn("record attempt", "err", err)
	}

	if !ok {
		writeErr(w, http.StatusUnauthorized, "이름 또는 비밀번호가 틀렸어요")
		return
	}
	token, err := s.st.CreateSession(r.Context(), u.ID)
	if err != nil {
		s.log.Error("create session", "err", err)
		writeErr(w, http.StatusInternalServerError, "세션 생성 실패")
		return
	}
	auth.SetCookie(w, token, s.dev)
	writeJSON(w, http.StatusOK, u)
}

// POST /api/logout
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.CookieName); err == nil && c.Value != "" {
		_ = s.st.DeleteSession(r.Context(), c.Value)
	}
	auth.ClearCookie(w, s.dev)
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/me
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"id": u.ID, "name": u.Name})
}

// GET /api/users — assignee pickers, family list.
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.st.ListUsers(r.Context())
	if err != nil {
		s.log.Error("list users", "err", err)
		writeErr(w, http.StatusInternalServerError, "사용자 목록 조회 실패")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

// humanDuration 은 Retry-After 를 한국어로(분 올림, 초 기준).
func humanDuration(d time.Duration) string {
	sec := int(d.Seconds())
	if sec <= 0 {
		return "잠시"
	}
	min := (sec + 59) / 60
	if min < 60 {
		return strconv.Itoa(min) + "분"
	}
	return strconv.Itoa((min+59)/60) + "시간"
}

type passwordReq struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// POST /api/users/{id}/password — 가족끼리 재설정.
// 계정 탈취 경로라 행위자 본인의 현재 비밀번호를 요구한다. 성공하면 대상 세션을 전부 끊는다.
func (s *Server) setPassword(w http.ResponseWriter, r *http.Request) {
	targetID, ok := pathID(w, r)
	if !ok {
		return
	}
	var req passwordReq
	if !decodeJSON(w, r, &req) {
		return
	}
	// 검증과 해싱으로 bcrypt 를 두 번 돈다.
	s.withBcryptSlot(w, r, func() { s.setPasswordLocked(w, r, targetID, req) })
}

func (s *Server) setPasswordLocked(w http.ResponseWriter, r *http.Request, targetID int64, req passwordReq) {
	actor, _ := auth.UserFrom(r.Context())

	// 행위자 확인도 레이트리밋 대상이다.
	ip := clientIP(r)
	now := time.Now()
	gate, err := s.st.CheckLogin(r.Context(), actor.Name, ip, now)
	if err != nil {
		s.log.Error("check login", "err", err)
		writeErr(w, http.StatusInternalServerError, "처리 중 오류")
		return
	}
	_, verified, err := s.st.Authenticate(r.Context(), actor.Name, req.CurrentPassword)
	if err != nil {
		s.log.Error("verify actor", "err", err)
		writeErr(w, http.StatusInternalServerError, "처리 중 오류")
		return
	}
	if !gate.Allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(gate.Retry.Seconds())+1))
		writeErr(w, http.StatusTooManyRequests, "시도가 너무 많아요. "+humanDuration(gate.Retry)+" 후에 다시 시도해주세요")
		return
	}
	outcome := store.AttemptWrongPass
	if verified {
		outcome = store.AttemptSuccess
	}
	if err := s.st.RecordAttempt(r.Context(), actor.Name, ip, outcome, now); err != nil {
		s.log.Warn("record attempt", "err", err)
	}
	if !verified {
		writeErr(w, http.StatusForbidden, "본인 비밀번호가 맞지 않아요")
		return
	}

	target, err := s.st.SetPasswordByID(r.Context(), targetID, req.NewPassword)
	if s.storeErr(w, err, "set password") {
		return
	}
	s.log.Info("password reset", "actor", actor.Name, "target", target.Name)

	// 대상이 자기 자신이면 새 세션을 발급해 로그아웃되지 않게 한다.
	if target.ID == actor.ID {
		token, err := s.st.CreateSession(r.Context(), actor.ID)
		if err != nil {
			s.log.Error("recreate session", "err", err)
			writeErr(w, http.StatusInternalServerError, "세션 재발급 실패")
			return
		}
		auth.SetCookie(w, token, s.dev)
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/users — any logged-in family member can add another.
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if !decodeJSON(w, r, &req) {
		return
	}
	var u store.User
	var err error
	s.withBcryptSlot(w, r, func() { u, err = s.st.CreateUser(r.Context(), req.Name, req.Password) })
	switch {
	case errors.Is(err, store.ErrConflict):
		writeErr(w, http.StatusConflict, "이미 있는 이름이에요")
		return
	case err != nil:
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

type birthReq struct {
	BirthDate string `json:"birth_date"`
}

// PATCH /api/users/{id}/birthdate — 서로 고칠 수 있다(아기는 스스로 못 넣는다).
func (s *Server) setBirthDate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req birthReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, err := s.st.SetBirthDate(r.Context(), id, req.BirthDate)
	if s.storeErr(w, err, "set birth date") {
		return
	}
	writeJSON(w, http.StatusOK, u)
}
