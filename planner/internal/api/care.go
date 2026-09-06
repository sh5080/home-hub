package api

import (
	"net/http"

	"github.com/sh5080/home-hub/planner/internal/auth"
	"github.com/sh5080/home-hub/planner/internal/store"
)

// 육아 기록. 아이는 ?child= 로 고른다.
func (s *Server) registerCare(m *http.ServeMux) {
	m.HandleFunc("GET /api/care/kinds", s.careKinds)
	m.HandleFunc("GET /api/care", s.careList)
	m.HandleFunc("GET /api/care/last", s.careLast)
	m.HandleFunc("POST /api/care", s.careAdd)
	m.HandleFunc("PATCH /api/care/{id}", s.carePatch)
	m.HandleFunc("DELETE /api/care/{id}", s.careDelete)
}

func (s *Server) careKinds(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, store.CareKinds)
}

// GET /api/care?child=&date=YYYY-MM-DD
func (s *Server) careList(w http.ResponseWriter, r *http.Request) {
	c, ok := s.child(w, r)
	if !ok {
		return
	}
	day, err := s.st.CareList(r.Context(), c.UserID, r.URL.Query().Get("date"))
	if s.storeErr(w, err, "care list") {
		return
	}
	writeJSON(w, http.StatusOK, day)
}

func (s *Server) careLast(w http.ResponseWriter, r *http.Request) {
	c, ok := s.child(w, r)
	if !ok {
		return
	}
	last, err := s.st.CareLast(r.Context(), c.UserID)
	if s.storeErr(w, err, "care last") {
		return
	}
	writeJSON(w, http.StatusOK, last)
}

type careReq struct {
	Kind     *string `json:"kind"`
	At       *string `json:"at"`
	Minutes  *int    `json:"minutes"`
	AmountML *int    `json:"amount_ml"`
	Detail   *string `json:"detail"`
	Note     *string `json:"note"`
	// 이유식: 재료 이름들, 식단 끼니(0 이면 풂)
	Ingredients *[]string `json:"ingredients"`
	MealID      *int64    `json:"meal_id"`
}

func (r careReq) input() store.CareInput {
	return store.CareInput{Kind: r.Kind, At: r.At, Minutes: r.Minutes, AmountML: r.AmountML, Detail: r.Detail, Note: r.Note,
		Ingredients: r.Ingredients, MealID: r.MealID}
}

func (s *Server) careAdd(w http.ResponseWriter, r *http.Request) {
	if s.quotaBlocked(w, r) {
		return
	}
	c, ok := s.child(w, r)
	if !ok {
		return
	}
	var req careReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	l, err := s.st.CareAdd(r.Context(), c.UserID, req.input(), u.ID)
	if s.storeErr(w, err, "care add") {
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (s *Server) carePatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req careReq
	if !decodeJSON(w, r, &req) {
		return
	}
	l, err := s.st.CareUpdate(r.Context(), id, req.input())
	if s.storeErr(w, err, "care update") {
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) careDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.storeErr(w, s.st.CareDelete(r.Context(), id), "care delete") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
