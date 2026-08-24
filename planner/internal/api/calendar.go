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

// GET /api/calendar?from&to
//
// 칸반과 캘린더는 같은 데이터를 다르게 보는 뷰다. 여기서는 날짜가 있는 카드를
// 기간으로 걸러 돌려줄 뿐이고, 별도의 '일정' 테이블은 없다.
func (s *Server) calendar(w http.ResponseWriter, r *http.Request) {
	from, to, ok := rangeParams(w, r)
	if !ok {
		return
	}
	cards, err := s.st.CalendarCards(r.Context(), from, to)
	if s.storeErr(w, err, "calendar") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cards": cards})
}

var _ = store.SortManual // keep the store import meaningful if handlers shrink
