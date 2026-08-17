package api

import (
	"net/http"

	"github.com/sh5080/home-hub/planner/internal/auth"
	"github.com/sh5080/home-hub/planner/internal/store"
)

func (s *Server) registerEvents(m *http.ServeMux) {
	m.HandleFunc("GET /api/events", s.listEvents)
	m.HandleFunc("POST /api/events", s.createEvent)
	m.HandleFunc("PATCH /api/events/{id}", s.patchEvent)
	m.HandleFunc("DELETE /api/events/{id}", s.deleteEvent)
	m.HandleFunc("GET /api/calendar", s.calendar)
}

type eventReq struct {
	Title   *string `json:"title"`
	StartAt *string `json:"start_at"`
	EndAt   *string `json:"end_at"`
	AllDay  *bool   `json:"all_day"`
	Content *string `json:"content"` // 블록 문서 JSON. description은 서버가 파생한다.
}

func (r eventReq) input() store.EventInput {
	return store.EventInput{Title: r.Title, StartAt: r.StartAt, EndAt: r.EndAt, AllDay: r.AllDay, Content: r.Content}
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

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	from, to, ok := rangeParams(w, r)
	if !ok {
		return
	}
	evs, err := s.st.ListEvents(r.Context(), from, to)
	if s.storeErr(w, err, "list events") {
		return
	}
	writeJSON(w, http.StatusOK, evs)
}

func (s *Server) createEvent(w http.ResponseWriter, r *http.Request) {
	var req eventReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	e, err := s.st.CreateEvent(r.Context(), req.input(), u.ID)
	if s.storeErr(w, err, "create event") {
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) patchEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req eventReq
	if !decodeJSON(w, r, &req) {
		return
	}
	e, err := s.st.UpdateEvent(r.Context(), id, req.input())
	if s.storeErr(w, err, "update event") {
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) deleteEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.storeErr(w, s.st.DeleteEvent(r.Context(), id), "delete event") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// calendar merges what the month view needs: events overlapping the range and
// cards due in it. Two queries, assembled here — no SQL UNION across shapes.
func (s *Server) calendar(w http.ResponseWriter, r *http.Request) {
	from, to, ok := rangeParams(w, r)
	if !ok {
		return
	}
	evs, err := s.st.ListEvents(r.Context(), from, to)
	if s.storeErr(w, err, "list events") {
		return
	}
	// due_at is a bare date; compare against the date portion of the bounds.
	due, err := s.st.DueCards(r.Context(), from[:10], to[:10])
	if s.storeErr(w, err, "due cards") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": evs, "due_cards": due})
}
