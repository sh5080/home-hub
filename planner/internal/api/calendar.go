package api

import (
	"net/http"

	"github.com/sh5080/home-hub/planner/internal/store"
)

func (s *Server) registerCalendar(m *http.ServeMux) {
	m.HandleFunc("GET /api/calendar", s.calendar)
}

// rangeParams reads ?from=&to= (date or datetime strings). Missing → 400.
func rangeParams(w http.ResponseWriter, r *http.Request) (from, to string, ok bool) {
	q := r.URL.Query()
	from, to = q.Get("from"), q.Get("to")
	if from == "" || to == "" || len(from) > 16 || len(to) > 16 || from >= to {
		writeErr(w, http.StatusBadRequest, "from and to are required, from < to")
		return "", "", false
	}
	return from, to, true
}

// GET /api/calendar?from&to — 마감이 있는 카드가 곧 일정이다(별도 표 없음).
func (s *Server) calendar(w http.ResponseWriter, r *http.Request) {
	from, to, ok := rangeParams(w, r)
	if !ok {
		return
	}
	cards, err := s.st.CalendarCards(r.Context(), from, to)
	if s.storeErr(w, err, "calendar") {
		return
	}
	diary, err := s.st.DiaryDates(r.Context(), from[:10], to[:10])
	if s.storeErr(w, err, "calendar diary") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cards": cards, "diary_dates": diary})
}

var _ = store.SortManual // keep the store import meaningful if handlers shrink
