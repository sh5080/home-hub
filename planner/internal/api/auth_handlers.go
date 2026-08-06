package api

import (
	"errors"
	"net/http"

	"github.com/sh5080/home-hub/planner/internal/auth"
	"github.com/sh5080/home-hub/planner/internal/store"
)

type loginReq struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

// POST /api/login
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, ok, err := s.st.Authenticate(r.Context(), req.Name, req.Password)
	if err != nil {
		s.log.Error("authenticate", "err", err)
		writeErr(w, http.StatusInternalServerError, "로그인 처리 중 오류")
		return
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

// POST /api/users — any logged-in family member can add another.
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, err := s.st.CreateUser(r.Context(), req.Name, req.Password)
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
