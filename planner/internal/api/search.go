package api

import (
	"net/http"
	"strings"
)

// GET /api/search?q= — 카드·루틴·재료·일기를 한 번에 찾는다.
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) > 100 {
		writeErr(w, http.StatusBadRequest, "검색어가 너무 길어요")
		return
	}
	res, err := s.st.SearchAll(r.Context(), q)
	if s.storeErr(w, err, "search") {
		return
	}
	writeJSON(w, http.StatusOK, res)
}
