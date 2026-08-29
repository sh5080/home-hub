package api

import (
	"net/http"
	"strconv"

	"github.com/sh5080/home-hub/planner/internal/auth"
	"github.com/sh5080/home-hub/planner/internal/store"
)

func (s *Server) registerBabyfood(m *http.ServeMux) {
	m.HandleFunc("GET /api/babyfood", s.babyfoodRange)
	m.HandleFunc("GET /api/babyfood/profile", s.babyfoodProfile)
	m.HandleFunc("PATCH /api/babyfood/profile", s.babyfoodSetProfile)
	m.HandleFunc("PATCH /api/babyfood/meals/{id}", s.babyfoodPatchMeal)
	m.HandleFunc("PATCH /api/babyfood/days/{dday}", s.babyfoodPatchDay)
	m.HandleFunc("GET /api/babyfood/foods", s.babyfoodFoods)
	m.HandleFunc("POST /api/babyfood/foods/tag", s.babyfoodTagFood)
	m.HandleFunc("GET /api/babyfood/stock", s.babyfoodStock)
	m.HandleFunc("POST /api/babyfood/stock/count", s.babyfoodCount)
	m.HandleFunc("POST /api/babyfood/stock/batch", s.babyfoodBatch)
	m.HandleFunc("POST /api/babyfood/stock/shopping", s.babyfoodShopping)
}

// GET /api/babyfood?from=YYYY-MM-DD&to=YYYY-MM-DD
// 날짜로 묻고 날짜로 답한다. D+n 은 저장 형식일 뿐 화면에서 쓰지 않는다.
func (s *Server) babyfoodRange(w http.ResponseWriter, r *http.Request) {
	p, err := s.st.BFProfileGet(r.Context())
	if s.storeErr(w, err, "babyfood profile") {
		return
	}
	if p.BirthDate == nil {
		// 생일이 없으면 날짜로 물을 수가 없다. 화면이 설정으로 안내하도록
		// 빈 목록과 프로필만 준다.
		writeJSON(w, http.StatusOK, map[string]any{"profile": p, "days": []store.BFDay{}})
		return
	}
	from, ok := s.ddayParam(w, r, "from", p)
	if !ok {
		return
	}
	to, ok := s.ddayParam(w, r, "to", p)
	if !ok {
		return
	}
	days, err := s.st.BFRange(r.Context(), from, to)
	if s.storeErr(w, err, "babyfood range") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": p, "days": days})
}

// ddayParam은 ?from=/?to= 날짜를 D+n 으로 바꾼다.
func (s *Server) ddayParam(w http.ResponseWriter, r *http.Request, key string, p store.BFProfile) (int, bool) {
	v := r.URL.Query().Get(key)
	if v == "" {
		writeErr(w, http.StatusBadRequest, key+" is required")
		return 0, false
	}
	d, err := store.BFDDayOf(*p.BirthDate, v)
	if err != nil {
		s.storeErr(w, err, "babyfood date")
		return 0, false
	}
	return d, true
}

func (s *Server) babyfoodProfile(w http.ResponseWriter, r *http.Request) {
	p, err := s.st.BFProfileGet(r.Context())
	if s.storeErr(w, err, "babyfood profile") {
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type bfProfileReq struct {
	Name        *string `json:"name"`
	BirthDate   *string `json:"birth_date"`
	HorizonDays *int    `json:"horizon_days"`
}

func (s *Server) babyfoodSetProfile(w http.ResponseWriter, r *http.Request) {
	var req bfProfileReq
	if !decodeJSON(w, r, &req) {
		return
	}
	p, err := s.st.BFProfileSet(r.Context(), store.BFProfileInput{
		Name: req.Name, BirthDate: req.BirthDate, HorizonDays: req.HorizonDays,
	})
	if s.storeErr(w, err, "babyfood set profile") {
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type bfMealReq struct {
	Base     *string   `json:"base"`
	Toppings *[]string `json:"toppings"`
	Snack    *string   `json:"snack"`
	EatenG   *int      `json:"eaten_g"`
	ServedG  *int      `json:"served_g"`
	Skipped  *bool     `json:"skipped"`
	// reset=true면 시드 원본으로 되돌린다.
	Reset bool `json:"reset"`
}

func (s *Server) babyfoodPatchMeal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req bfMealReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	day, err := s.st.BFUpdateMeal(r.Context(), id, store.BFMealInput{
		Base: req.Base, Toppings: req.Toppings, Snack: req.Snack,
		EatenG: req.EatenG, ServedG: req.ServedG, Skipped: req.Skipped, Reset: req.Reset,
	}, u.ID)
	if s.storeErr(w, err, "babyfood update meal") {
		return
	}
	writeJSON(w, http.StatusOK, day)
}

type bfDayReq struct {
	Note    *string `json:"note"`
	NewItem *string `json:"new_item"`
}

func (s *Server) babyfoodPatchDay(w http.ResponseWriter, r *http.Request) {
	dday, err := strconv.Atoi(r.PathValue("dday"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid dday")
		return
	}
	var req bfDayReq
	if !decodeJSON(w, r, &req) {
		return
	}
	day, err := s.st.BFUpdateDay(r.Context(), dday, store.BFDayInput{
		Note: req.Note, NewItem: req.NewItem,
	})
	if s.storeErr(w, err, "babyfood update day") {
		return
	}
	writeJSON(w, http.StatusOK, day)
}

// GET /api/babyfood/foods — 재료 전체와 거기 달린 표시.
func (s *Server) babyfoodFoods(w http.ResponseWriter, r *http.Request) {
	foods, err := s.st.BFFoods(r.Context())
	if s.storeErr(w, err, "babyfood foods") {
		return
	}
	writeJSON(w, http.StatusOK, foods)
}

type bfTagReq struct {
	Name     string `json:"name"`
	Reaction *bool  `json:"reaction"`
	Liked    *bool  `json:"liked"`
}

// POST /api/babyfood/foods/tag — 알레르기 반응 / 좋아함을 켜고 끈다.
func (s *Server) babyfoodTagFood(w http.ResponseWriter, r *http.Request) {
	var req bfTagReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	f, err := s.st.BFTagFood(r.Context(), req.Name, store.BFFoodTag{Reaction: req.Reaction, Liked: req.Liked}, u.ID)
	if s.storeErr(w, err, "babyfood tag food") {
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// GET /api/babyfood/stock[?days=N] — N을 주면 그 기간으로 한 번만 계산한다
// (설정을 바꾸지 않고 "4주면 얼마나?" 를 볼 수 있게).
func (s *Server) babyfoodStock(w http.ResponseWriter, r *http.Request) {
	days := 0
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid days")
			return
		}
		days = n
	}
	view, err := s.st.BFStockList(r.Context(), days)
	if s.storeErr(w, err, "babyfood stock") {
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type bfCountReq struct {
	Name string `json:"name"`
	Qty  int    `json:"qty"`
}

// POST /api/babyfood/stock/count — 실사. 지금 냉동실에 있는 개수로 맞춘다.
func (s *Server) babyfoodCount(w http.ResponseWriter, r *http.Request) {
	var req bfCountReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	if s.storeErr(w, s.st.BFCount(r.Context(), req.Name, req.Qty, u.ID), "babyfood count") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type bfBatchReq struct {
	Name string `json:"name"`
	Qty  int    `json:"qty"`
	Note string `json:"note"`
}

// POST /api/babyfood/stock/batch — 큐브 제조 기록.
func (s *Server) babyfoodBatch(w http.ResponseWriter, r *http.Request) {
	if s.quotaBlocked(w, r) {
		return
	}
	var req bfBatchReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	if s.storeErr(w, s.st.BFAddBatch(r.Context(), req.Name, req.Qty, req.Note, u.ID), "babyfood batch") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type bfShoppingReq struct {
	Days int `json:"days"`
}

// POST /api/babyfood/stock/shopping — '제조 필요'를 장보기 카드로 만든다.
// 재고 화면에서 끝나지 않고 칸반·홈·캘린더로 이어지게 하는 지점이다.
func (s *Server) babyfoodShopping(w http.ResponseWriter, r *http.Request) {
	if s.quotaBlocked(w, r) {
		return
	}
	var req bfShoppingReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	card, err := s.st.BFShoppingCard(r.Context(), req.Days, u.ID)
	if s.storeErr(w, err, "babyfood shopping card") {
		return
	}
	writeJSON(w, http.StatusCreated, card)
}
