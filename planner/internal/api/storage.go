package api

import "net/http"

// GET /api/storage — 저장 공간 현황.
func (s *Server) storage(w http.ResponseWriter, r *http.Request) {
	st, err := s.st.Stat(r.Context())
	if s.storeErr(w, err, "storage") {
		return
	}
	writeJSON(w, http.StatusOK, st)
}
