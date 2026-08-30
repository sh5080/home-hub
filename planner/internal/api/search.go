package api

import (
	"net/http"
	"strings"
)

// GET /api/search?q=... — 제목과 본문에서 카드를 찾는다.
// 읽기라 변경 알림(SSE)을 내지 않는다.
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) > 100 {
		writeErr(w, http.StatusBadRequest, "검색어가 너무 길어요")
		return
	}
	res, err := s.st.SearchCards(r.Context(), q)
	if s.storeErr(w, err, "search") {
		return
	}
	writeJSON(w, http.StatusOK, res)
}
