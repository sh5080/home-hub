package api

import (
	"net/http"
	"time"

	"github.com/sh5080/home-hub/planner/internal/auth"
)

func (s *Server) registerFamily(m *http.ServeMux) {
	m.HandleFunc("GET /api/family", s.family)
	m.HandleFunc("PUT /api/family/reward", s.familyReward)
	m.HandleFunc("POST /api/family/seen", s.familySeen)
	m.HandleFunc("PUT /api/family/jar-goal", s.familyJarGoal)
	m.HandleFunc("POST /api/family/wishes", s.familySaveWish)
	m.HandleFunc("PATCH /api/family/wishes/{id}", s.familySaveWish)
	m.HandleFunc("DELETE /api/family/wishes/{id}", s.familyDeleteWish)
	m.HandleFunc("POST /api/family/grow-next", s.familyGrowNext)
	m.HandleFunc("POST /api/family/wishes/{id}/buy", s.familyBuyWish)
	m.HandleFunc("PUT /api/family/plant-name", s.familyRenamePlant)
}

// GET /api/family — 가족 퀘스트·연속 기록·저금통·마일스톤.
func (s *Server) family(w http.ResponseWriter, r *http.Request) {
	b, err := s.st.Family(r.Context(), time.Now())
	if s.storeErr(w, err, "family") {
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// PUT /api/family/reward {week, reward} — 이번 주 퀘스트를 다 채우면 할 보상.
func (s *Server) familyReward(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Week   string `json:"week"`
		Reward string `json:"reward"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	if s.storeErr(w, s.st.FamilySetReward(r.Context(), req.Week, req.Reward, u.ID), "family reward") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/family/seen {keys} — 새 마일스톤을 확인했다.
func (s *Server) familySeen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Keys []string `json:"keys"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.storeErr(w, s.st.FamilySeen(r.Context(), req.Keys), "family seen") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /api/family/jar-goal {goal_id} — 저금통을 이을 목표(0 이면 없음).
func (s *Server) familyJarGoal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GoalID int64 `json:"goal_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.storeErr(w, s.st.FamilySetJarGoal(r.Context(), req.GoalID), "family jar goal") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type familyWishReq struct {
	Title string `json:"title"`
	Price int64  `json:"price"`
	Note  string `json:"note"`
}

// POST /api/family/wishes, PATCH /api/family/wishes/{id} — 위시리스트는 언제든 넣고 고친다.
func (s *Server) familySaveWish(w http.ResponseWriter, r *http.Request) {
	var id int64
	if r.PathValue("id") != "" {
		var ok bool
		if id, ok = pathID(w, r); !ok {
			return
		}
	}
	var req familyWishReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	nid, err := s.st.FamilySaveWish(r.Context(), id, req.Title, req.Note, req.Price, u.ID)
	if s.storeErr(w, err, "family wish") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"id": nid})
}

func (s *Server) familyDeleteWish(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.storeErr(w, s.st.FamilyDeleteWish(r.Context(), id), "family wish delete") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/family/grow-next — 다 키운 식물을 마치고(구매권 +1) 새 씨앗을 심는다.
func (s *Server) familyGrowNext(w http.ResponseWriter, r *http.Request) {
	if s.storeErr(w, s.st.FamilyGrowNext(r.Context(), time.Now()), "family grow next") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/family/wishes/{id}/buy — 구매권 하나로 위시를 산다.
func (s *Server) familyBuyWish(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.storeErr(w, s.st.FamilyBuyWish(r.Context(), id, time.Now()), "family buy wish") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /api/family/plant-name {name}
func (s *Server) familyRenamePlant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.storeErr(w, s.st.FamilyRenamePlant(r.Context(), req.Name, time.Now()), "family plant name") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
