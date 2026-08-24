package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/sh5080/home-hub/planner/internal/auth"
	"github.com/sh5080/home-hub/planner/internal/store"
)

func (s *Server) registerBoards(m *http.ServeMux) {
	m.HandleFunc("GET /api/boards", s.listBoards)
	m.HandleFunc("POST /api/boards", s.createBoard)
	m.HandleFunc("GET /api/boards/{id}", s.getBoard)
	m.HandleFunc("PATCH /api/boards/{id}", s.renameBoard)
	m.HandleFunc("DELETE /api/boards/{id}", s.deleteBoard)
	m.HandleFunc("POST /api/boards/{id}/columns", s.addColumn)
	m.HandleFunc("PATCH /api/columns/{id}", s.renameColumn)
	m.HandleFunc("DELETE /api/columns/{id}", s.deleteColumn)
	m.HandleFunc("POST /api/columns/{id}/cards", s.createCard)
	m.HandleFunc("GET /api/cards/{id}", s.getCard)
	m.HandleFunc("PATCH /api/cards/{id}", s.patchCard)
	m.HandleFunc("DELETE /api/cards/{id}", s.deleteCard)
}

// pathID parses the {id} wildcard; writes 400 and returns ok=false on garbage.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

// storeErr maps store errors to HTTP. Returns true if it wrote a response.
func (s *Server) storeErr(w http.ResponseWriter, err error, what string) bool {
	var ve *store.ValidationError
	switch {
	case err == nil:
		return false
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrConflict):
		writeErr(w, http.StatusConflict, "already exists")
	case errors.As(err, &ve):
		writeErr(w, http.StatusBadRequest, ve.Msg)
	default:
		s.log.Error(what, "err", err)
		writeErr(w, http.StatusInternalServerError, what+" failed")
	}
	return true
}

type nameReq struct {
	Name string `json:"name"`
}

func (s *Server) listBoards(w http.ResponseWriter, r *http.Request) {
	boards, err := s.st.ListBoards(r.Context())
	if s.storeErr(w, err, "list boards") {
		return
	}
	writeJSON(w, http.StatusOK, boards)
}

func (s *Server) createBoard(w http.ResponseWriter, r *http.Request) {
	var req nameReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	b, err := s.st.CreateBoard(r.Context(), req.Name, u.ID)
	if s.storeErr(w, err, "create board") {
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) getBoard(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	d, err := s.st.GetBoard(r.Context(), id, store.ParseSort(r.URL.Query().Get("sort")))
	if s.storeErr(w, err, "get board") {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) renameBoard(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req nameReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.storeErr(w, s.st.RenameBoard(r.Context(), id, req.Name), "rename board") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteBoard(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.storeErr(w, s.st.DeleteBoard(r.Context(), id), "delete board") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) addColumn(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req nameReq
	if !decodeJSON(w, r, &req) {
		return
	}
	c, err := s.st.AddColumn(r.Context(), id, req.Name)
	if s.storeErr(w, err, "add column") {
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) renameColumn(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req nameReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.storeErr(w, s.st.RenameColumn(r.Context(), id, req.Name), "rename column") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteColumn(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.storeErr(w, s.st.DeleteColumn(r.Context(), id), "delete column") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// cardReq covers both create and patch. For patch, absent fields are left
// alone; column_id+position together mean "move".
type cardReq struct {
	Title      *string `json:"title"`
	Content    *string `json:"content"` // 블록 문서 JSON. description은 서버가 파생한다.
	DueAt      *string `json:"due_at"`  // 'YYYY-MM-DD' 또는 'YYYY-MM-DDTHH:MM'
	EndAt      *string `json:"end_at"`  // 여러 날 항목의 끝
	Priority   *int    `json:"priority"`
	AssigneeID *int64  `json:"assignee_id"`
	ColumnID   *int64  `json:"column_id"`
	Position   *int    `json:"position"`
}

func (r cardReq) input() store.CardInput {
	return store.CardInput{Title: r.Title, Content: r.Content, DueAt: r.DueAt, EndAt: r.EndAt, AssigneeID: r.AssigneeID, Priority: r.Priority}
}

func (s *Server) createCard(w http.ResponseWriter, r *http.Request) {
	col, ok := pathID(w, r)
	if !ok {
		return
	}
	var req cardReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	c, err := s.st.CreateCard(r.Context(), col, req.input(), u.ID)
	if s.storeErr(w, err, "create card") {
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) getCard(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	d, err := s.st.GetCardDetail(r.Context(), id)
	if s.storeErr(w, err, "get card") {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) patchCard(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req cardReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if (req.ColumnID == nil) != (req.Position == nil) {
		writeErr(w, http.StatusBadRequest, "column_id and position must be given together")
		return
	}
	// Field edits first, then the move, so a combined request is one round-trip.
	c, err := s.st.UpdateCard(r.Context(), id, req.input())
	if s.storeErr(w, err, "update card") {
		return
	}
	if req.ColumnID != nil {
		c, err = s.st.MoveCard(r.Context(), id, *req.ColumnID, *req.Position)
		if s.storeErr(w, err, "move card") {
			return
		}
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) deleteCard(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.storeErr(w, s.st.DeleteCard(r.Context(), id), "delete card") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
