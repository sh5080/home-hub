package api

import (
	"net/http"

	"github.com/sh5080/home-hub/planner/internal/auth"
	"github.com/sh5080/home-hub/planner/internal/store"
)

func (s *Server) registerRoutines(m *http.ServeMux) {
	m.HandleFunc("GET /api/routines", s.listRoutines)
	m.HandleFunc("POST /api/routines", s.createRoutine)
	m.HandleFunc("PATCH /api/routines/{id}", s.patchRoutine)
	m.HandleFunc("DELETE /api/routines/{id}", s.deleteRoutine)
	m.HandleFunc("PUT /api/routines/{id}/checks/{date}", s.checkRoutine)
	m.HandleFunc("DELETE /api/routines/{id}/checks/{date}", s.uncheckRoutine)
	m.HandleFunc("GET /api/routines/checks", s.routineChecks)
	m.HandleFunc("GET /api/today", s.today)
}

// GET /api/today?date=D → {routines, cards}: the home screen's unified list.
// Routines are those scheduled on D with D's check state; cards are every
// board's first-column ("할 일") cards.
func (s *Server) today(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		writeErr(w, http.StatusBadRequest, "date is required")
		return
	}
	rs, err := s.st.RoutinesForDate(r.Context(), date)
	if s.storeErr(w, err, "routines for date") {
		return
	}
	cards, err := s.st.TodoCards(r.Context(), store.ParseSort(r.URL.Query().Get("sort")), store.ParseOrder(r.URL.Query().Get("order")))
	if s.storeErr(w, err, "todo cards") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"routines": rs, "cards": cards})
}

type routineReq struct {
	Title        *string `json:"title"`
	WeekdaysMask *int    `json:"weekdays_mask"`
	TimeOfDay    *string `json:"time_of_day"`
	AssigneeID   *int64  `json:"assignee_id"`
	Active       *bool   `json:"active"`
}

func (r routineReq) input() store.RoutineInput {
	return store.RoutineInput{Title: r.Title, WeekdaysMask: r.WeekdaysMask, TimeOfDay: r.TimeOfDay, AssigneeID: r.AssigneeID, Active: r.Active}
}

// GET /api/routines            → all routines, no check state (management view)
// GET /api/routines?date=D     → active routines scheduled on D with D's check state (dashboard)
func (s *Server) listRoutines(w http.ResponseWriter, r *http.Request) {
	if date := r.URL.Query().Get("date"); date != "" {
		rs, err := s.st.RoutinesForDate(r.Context(), date)
		if s.storeErr(w, err, "routines for date") {
			return
		}
		writeJSON(w, http.StatusOK, rs)
		return
	}
	rs, err := s.st.ListRoutines(r.Context())
	if s.storeErr(w, err, "list routines") {
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

// GET /api/routines/checks?from&to → {routine_id: [dates...]} for the weekly grid.
func (s *Server) routineChecks(w http.ResponseWriter, r *http.Request) {
	from, to, ok := rangeParams(w, r)
	if !ok {
		return
	}
	m, err := s.st.RoutineChecksInRange(r.Context(), from, to)
	if s.storeErr(w, err, "routine checks") {
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) createRoutine(w http.ResponseWriter, r *http.Request) {
	if s.quotaBlocked(w, r) {
		return
	}
	var req routineReq
	if !decodeJSON(w, r, &req) {
		return
	}
	rt, err := s.st.CreateRoutine(r.Context(), req.input())
	if s.storeErr(w, err, "create routine") {
		return
	}
	writeJSON(w, http.StatusCreated, rt)
}

func (s *Server) patchRoutine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req routineReq
	if !decodeJSON(w, r, &req) {
		return
	}
	rt, err := s.st.UpdateRoutine(r.Context(), id, req.input())
	if s.storeErr(w, err, "update routine") {
		return
	}
	writeJSON(w, http.StatusOK, rt)
}

func (s *Server) deleteRoutine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.storeErr(w, s.st.DeleteRoutine(r.Context(), id), "delete routine") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) checkRoutine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	if s.storeErr(w, s.st.SetRoutineCheck(r.Context(), id, r.PathValue("date"), u.ID), "check routine") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) uncheckRoutine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.storeErr(w, s.st.ClearRoutineCheck(r.Context(), id, r.PathValue("date")), "uncheck routine") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
