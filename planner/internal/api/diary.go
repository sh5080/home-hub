package api

import (
	"net/http"

	"github.com/sh5080/home-hub/planner/internal/auth"
	"github.com/sh5080/home-hub/planner/internal/store"
)

func (s *Server) registerDiary(m *http.ServeMux) {
	m.HandleFunc("GET /api/diary", s.diaryList)
	m.HandleFunc("POST /api/diary", s.diaryCreate)
	m.HandleFunc("GET /api/diary/{id}", s.diaryGet)
	m.HandleFunc("PATCH /api/diary/{id}", s.diaryPatch)
	m.HandleFunc("DELETE /api/diary/{id}", s.diaryDelete)
}

// GET /api/diary?from=&to=&limit=&before_date=&before_id= (before_* 는 더 보기 커서)
func (s *Server) diaryList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list, err := s.st.DiaryListPage(r.Context(), store.DiaryPage{
		From: q.Get("from"), To: q.Get("to"), Limit: atoiDefault(q.Get("limit"), 0),
		BeforeDate: q.Get("before_date"), BeforeID: int64(atoiDefault(q.Get("before_id"), 0)),
	})
	if s.storeErr(w, err, "diary list") {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type diaryNewReq struct {
	Date  string `json:"date"`
	Title string `json:"title"`
}

func (s *Server) diaryCreate(w http.ResponseWriter, r *http.Request) {
	if s.quotaBlocked(w, r) {
		return
	}
	var req diaryNewReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	e, err := s.st.DiaryCreate(r.Context(), req.Date, req.Title, u.ID)
	if s.storeErr(w, err, "diary create") {
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) diaryGet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	e, err := s.st.DiaryGet(r.Context(), id)
	if s.storeErr(w, err, "diary get") {
		return
	}
	writeJSON(w, http.StatusOK, e)
}

type diaryPatchReq struct {
	Date    *string           `json:"date"`
	Title   *string           `json:"title"`
	Content *string           `json:"content"`
	Refs    *[]store.DiaryRef `json:"refs"`
}

func (s *Server) diaryPatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req diaryPatchReq
	if !decodeJSON(w, r, &req) {
		return
	}
	e, err := s.st.DiaryUpdate(r.Context(), id, store.DiaryInput{
		Date: req.Date, Title: req.Title, Content: req.Content, Refs: req.Refs,
	})
	if s.storeErr(w, err, "diary update") {
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) diaryDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.storeErr(w, s.st.DiaryDelete(r.Context(), id), "diary delete") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
