package api

import (
	"net/http"
	"strconv"

	"github.com/sh5080/home-hub/planner/internal/auth"
	"github.com/sh5080/home-hub/planner/internal/store"
)

func (s *Server) registerBabyfood(m *http.ServeMux) {
	m.HandleFunc("GET /api/babyfood/children", s.babyfoodChildren)
	m.HandleFunc("POST /api/babyfood/children", s.babyfoodAddChild)
	m.HandleFunc("DELETE /api/babyfood/children/{id}", s.babyfoodRemoveChild)
	m.HandleFunc("POST /api/babyfood/mealtimes", s.babyfoodMealTimes)
	m.HandleFunc("GET /api/babyfood", s.babyfoodRange)
	m.HandleFunc("PATCH /api/babyfood/meals/{id}", s.babyfoodPatchMeal)
	m.HandleFunc("PATCH /api/babyfood/days/{dday}", s.babyfoodPatchDay)
	m.HandleFunc("GET /api/babyfood/foods", s.babyfoodFoods)
	m.HandleFunc("POST /api/babyfood/logs", s.babyfoodLog)
	m.HandleFunc("GET /api/babyfood/stock", s.babyfoodStock)
	m.HandleFunc("POST /api/babyfood/stock/count", s.babyfoodCount)
	m.HandleFunc("POST /api/babyfood/stock/batch", s.babyfoodBatch)
	m.HandleFunc("POST /api/babyfood/stock/shopping", s.babyfoodShopping)
}

// child 는 ?child=<가족 id> 를 읽는다. 아이가 하나면 생략 가능.
func (s *Server) child(w http.ResponseWriter, r *http.Request) (store.BFChild, bool) {
	kids, err := s.st.BFChildren(r.Context())
	if s.storeErr(w, err, "babyfood children") {
		return store.BFChild{}, false
	}
	if len(kids) == 0 {
		writeErr(w, http.StatusBadRequest, "이유식 대상을 먼저 정해주세요")
		return store.BFChild{}, false
	}
	v := r.URL.Query().Get("child")
	if v == "" {
		if len(kids) == 1 {
			return kids[0], true
		}
		writeErr(w, http.StatusBadRequest, "어느 아이인지 알려주세요")
		return store.BFChild{}, false
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid child")
		return store.BFChild{}, false
	}
	for _, k := range kids {
		if k.UserID == id {
			return k, true
		}
	}
	writeErr(w, http.StatusNotFound, "그 아이는 이유식 대상이 아니에요")
	return store.BFChild{}, false
}

func (s *Server) babyfoodChildren(w http.ResponseWriter, r *http.Request) {
	kids, err := s.st.BFChildren(r.Context())
	if s.storeErr(w, err, "babyfood children") {
		return
	}
	writeJSON(w, http.StatusOK, kids)
}

type bfChildReq struct {
	UserID      int64 `json:"user_id"`
	HorizonDays int   `json:"horizon_days"`
}

func (s *Server) babyfoodAddChild(w http.ResponseWriter, r *http.Request) {
	var req bfChildReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.storeErr(w, s.st.BFAddChild(r.Context(), req.UserID, req.HorizonDays), "babyfood add child") {
		return
	}
	kids, err := s.st.BFChildren(r.Context())
	if s.storeErr(w, err, "babyfood children") {
		return
	}
	writeJSON(w, http.StatusOK, kids)
}

func (s *Server) babyfoodRemoveChild(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.storeErr(w, s.st.BFRemoveChild(r.Context(), id), "babyfood remove child") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type bfTimesReq struct {
	N     int      `json:"n"`
	Times []string `json:"times"`
}

// POST /api/babyfood/mealtimes — 하루 n끼일 때의 기본 시각.
func (s *Server) babyfoodMealTimes(w http.ResponseWriter, r *http.Request) {
	c, ok := s.child(w, r)
	if !ok {
		return
	}
	var req bfTimesReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.storeErr(w, s.st.BFSetMealTimes(r.Context(), c.UserID, req.N, req.Times), "babyfood meal times") {
		return
	}
	kids, err := s.st.BFChildren(r.Context())
	if s.storeErr(w, err, "babyfood children") {
		return
	}
	writeJSON(w, http.StatusOK, kids)
}

// GET /api/babyfood?child=&from=YYYY-MM-DD&to=YYYY-MM-DD (날짜로 묻고 답한다. D+n 은 저장 형식일 뿐)
func (s *Server) babyfoodRange(w http.ResponseWriter, r *http.Request) {
	kids, err := s.st.BFChildren(r.Context())
	if s.storeErr(w, err, "babyfood children") {
		return
	}
	if len(kids) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"children": kids, "days": []store.BFDay{}})
		return
	}
	c, ok := s.child(w, r)
	if !ok {
		return
	}
	from, ok := s.ddayParam(w, r, "from", c.BirthDate)
	if !ok {
		return
	}
	to, ok := s.ddayParam(w, r, "to", c.BirthDate)
	if !ok {
		return
	}
	days, err := s.st.BFRange(r.Context(), c.UserID, from, to)
	if s.storeErr(w, err, "babyfood range") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"children": kids, "child": c, "days": days})
}

// ddayParam은 ?from=/?to= 날짜를 D+n 으로 바꾼다.
func (s *Server) ddayParam(w http.ResponseWriter, r *http.Request, key, birth string) (int, bool) {
	v := r.URL.Query().Get(key)
	if v == "" {
		writeErr(w, http.StatusBadRequest, key+" is required")
		return 0, false
	}
	d, err := store.BFDDayOf(birth, v)
	if err != nil {
		s.storeErr(w, err, "babyfood date")
		return 0, false
	}
	return d, true
}

type bfMealReq struct {
	Base     *string   `json:"base"`
	Toppings *[]string `json:"toppings"`
	Snack    *string   `json:"snack"`
	EatenG   *int      `json:"eaten_g"`
	ServedG  *int      `json:"served_g"`
	Skipped  *bool     `json:"skipped"`
	// At은 'HH:MM'. ""면 설정의 기본 시각으로 되돌린다.
	At *string `json:"at"`
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
		EatenG: req.EatenG, ServedG: req.ServedG, Skipped: req.Skipped,
		At: req.At, Reset: req.Reset,
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
	c, ok := s.child(w, r)
	if !ok {
		return
	}
	var req bfDayReq
	if !decodeJSON(w, r, &req) {
		return
	}
	day, err := s.st.BFUpdateDay(r.Context(), c.UserID, dday, store.BFDayInput{
		Note: req.Note, NewItem: req.NewItem,
	})
	if s.storeErr(w, err, "babyfood update day") {
		return
	}
	writeJSON(w, http.StatusOK, day)
}

// GET /api/babyfood/foods
func (s *Server) babyfoodFoods(w http.ResponseWriter, r *http.Request) {
	c, ok := s.child(w, r)
	if !ok {
		return
	}
	foods, err := s.st.BFFoods(r.Context(), c.UserID)
	if s.storeErr(w, err, "babyfood foods") {
		return
	}
	writeJSON(w, http.StatusOK, foods)
}

type bfLogReq struct {
	Date     string `json:"date"`
	Name     string `json:"name"`
	Reaction *bool  `json:"reaction"`
	Liked    *bool  `json:"liked"`
	Disliked *bool  `json:"disliked"`
}

// POST /api/babyfood/logs — 그날 그 재료에 반응 / 좋아함 / 싫어함을 남긴다.
func (s *Server) babyfoodLog(w http.ResponseWriter, r *http.Request) {
	c, ok := s.child(w, r)
	if !ok {
		return
	}
	var req bfLogReq
	if !decodeJSON(w, r, &req) {
		return
	}
	dday, err := store.BFDDayOf(c.BirthDate, req.Date)
	if err != nil {
		s.storeErr(w, err, "babyfood date")
		return
	}
	u, _ := auth.UserFrom(r.Context())
	day, err := s.st.BFSetLog(r.Context(), c.UserID, dday, req.Name,
		store.BFFoodTag{Reaction: req.Reaction, Liked: req.Liked, Disliked: req.Disliked}, u.ID)
	if s.storeErr(w, err, "babyfood log") {
		return
	}
	writeJSON(w, http.StatusOK, day)
}

// GET /api/babyfood/stock[?days=N] — N을 주면 그 기간으로 한 번만 계산한다.
func (s *Server) babyfoodStock(w http.ResponseWriter, r *http.Request) {
	c, ok := s.child(w, r)
	if !ok {
		return
	}
	days := 0
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid days")
			return
		}
		days = n
	}
	view, err := s.st.BFStockList(r.Context(), c.UserID, days)
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
	c, ok := s.child(w, r)
	if !ok {
		return
	}
	var req bfCountReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	if s.storeErr(w, s.st.BFCount(r.Context(), c.UserID, req.Name, req.Qty, u.ID), "babyfood count") {
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
	c, ok := s.child(w, r)
	if !ok {
		return
	}
	var req bfBatchReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	if s.storeErr(w, s.st.BFAddBatch(r.Context(), c.UserID, req.Name, req.Qty, req.Note, u.ID), "babyfood batch") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type bfShoppingReq struct {
	Days int `json:"days"`
}

// POST /api/babyfood/stock/shopping — '제조 필요'를 장보기 카드로.
func (s *Server) babyfoodShopping(w http.ResponseWriter, r *http.Request) {
	if s.quotaBlocked(w, r) {
		return
	}
	c, ok := s.child(w, r)
	if !ok {
		return
	}
	var req bfShoppingReq
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	card, err := s.st.BFShoppingCard(r.Context(), c.UserID, req.Days, u.ID)
	if s.storeErr(w, err, "babyfood shopping card") {
		return
	}
	writeJSON(w, http.StatusCreated, card)
}
