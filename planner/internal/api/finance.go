package api

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/sh5080/home-hub/planner/internal/auth"
	"github.com/sh5080/home-hub/planner/internal/store"
)

func (s *Server) registerFinance(m *http.ServeMux) {
	m.HandleFunc("POST /api/finance/import", s.financeImport)
	m.HandleFunc("GET /api/finance/format", s.financeFormat)
	m.HandleFunc("GET /api/finance/overview", s.financeOverview)
	m.HandleFunc("POST /api/finance/settings", s.financeSettings)
	m.HandleFunc("POST /api/finance/fixed", s.financeFixed)
	m.HandleFunc("POST /api/finance/income", s.financeIncome)
	m.HandleFunc("POST /api/finance/spend", s.financeSpend)
	m.HandleFunc("POST /api/finance/tags", s.financeCreateTag)
	m.HandleFunc("PATCH /api/finance/tags/{id}", s.financeUpdateTag)
	m.HandleFunc("DELETE /api/finance/tags/{id}", s.financeDeleteTag)
	m.HandleFunc("PUT /api/finance/tag-links", s.financeSetTags)
	m.HandleFunc("PUT /api/finance/labels", s.financeSetLabel)
	m.HandleFunc("GET /api/finance/detail", s.financeDetail)
	m.HandleFunc("POST /api/finance/goals", s.financeSaveGoal)
	m.HandleFunc("DELETE /api/finance/goals/{id}", s.financeDeleteGoal)
}

// POST /api/finance/import — multipart: file(xlsx), owner(가족 id), apply(1 이면 저장, 아니면 미리보기).
// 파일은 메모리에서 읽고 버린다(이름·신용점수가 들어 있다).
func (s *Server) financeImport(w http.ResponseWriter, r *http.Request) {
	if s.quotaBlocked(w, r) {
		return
	}
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(2 * time.Minute))
	r.Body = http.MaxBytesReader(w, r.Body, 15<<20)
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "파일이 너무 크거나(15MB) 형식이 맞지 않아요")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "파일을 읽지 못했어요")
		return
	}
	u, _ := auth.UserFrom(r.Context())
	owner := u.ID
	if v := r.FormValue("owner"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || !s.isUser(r, id) {
			writeErr(w, http.StatusBadRequest, "가족이 아니에요")
			return
		}
		owner = id
	}
	p, err := store.ParseFinanceExport(data, hdr.Filename, s.st.FinFormatNow())
	if s.storeErr(w, err, "finance parse") {
		return
	}
	res, err := s.st.FinanceImport(r.Context(), owner, p, r.FormValue("apply") == "1", u.ID)
	if s.storeErr(w, err, "finance import") {
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) isUser(r *http.Request, id int64) bool {
	us, err := s.st.ListUsers(r.Context())
	if err != nil {
		return false
	}
	for _, u := range us {
		if u.ID == id {
			return true
		}
	}
	return false
}

// GET /api/finance/overview?owner=(0|비움=가족 전체)&month=YYYY-MM
func (s *Server) financeOverview(w http.ResponseWriter, r *http.Request) {
	owner, _ := strconv.ParseInt(r.URL.Query().Get("owner"), 10, 64)
	ov, err := s.st.FinanceOverview(r.Context(), owner, r.URL.Query().Get("month"), time.Now())
	if s.storeErr(w, err, "finance overview") {
		return
	}
	writeJSON(w, http.StatusOK, ov)
}

type finSettingsReq struct {
	Income       *int64 `json:"income"`
	BufferMonths *int   `json:"buffer_months"`
}

func (s *Server) financeSettings(w http.ResponseWriter, r *http.Request) {
	var req finSettingsReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.storeErr(w, s.st.FinanceSetSettings(r.Context(), req.Income, req.BufferMonths), "finance settings") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type finFixedReq struct {
	Key   string `json:"key"`
	Fixed *bool  `json:"fixed"` // null 이면 자동 판정으로 되돌림
}

func (s *Server) financeFixed(w http.ResponseWriter, r *http.Request) {
	var req finFixedReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.storeErr(w, s.st.FinanceSetFixed(r.Context(), req.Key, req.Fixed), "finance fixed") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) financeSaveGoal(w http.ResponseWriter, r *http.Request) {
	var g store.FinGoal
	if !decodeJSON(w, r, &g) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	out, err := s.st.FinanceSaveGoal(r.Context(), g, u.ID)
	if s.storeErr(w, err, "finance goal") {
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) financeDeleteGoal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.storeErr(w, s.st.FinanceDeleteGoal(r.Context(), id), "finance goal delete") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/finance/format — 화면 안내용. 설정이 없으면 configured=false.
func (s *Server) financeFormat(w http.ResponseWriter, _ *http.Request) {
	f := s.st.FinFormatNow()
	if f == nil {
		writeJSON(w, http.StatusOK, map[string]any{"configured": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": true, "source_name": f.SourceName, "how_to": f.HowTo,
		"summary_sheet": f.SummarySheet, "tx_sheet": f.TxSheet,
		"sections":  []string{f.BalanceSection, f.InvestSection, f.LoanSection},
		"tx_header": f.TxHeader,
	})
}

type finIncomeReq struct {
	Key    string `json:"key"`
	Income *bool  `json:"income"`
}

// POST /api/finance/income {key, income|null} — null 이면 자동 판정으로.
func (s *Server) financeIncome(w http.ResponseWriter, r *http.Request) {
	var req finIncomeReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.storeErr(w, s.st.FinanceSetIncome(r.Context(), req.Key, req.Income), "finance income") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type finTagReq struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (s *Server) financeCreateTag(w http.ResponseWriter, r *http.Request) {
	var req finTagReq
	if !decodeJSON(w, r, &req) {
		return
	}
	t, err := s.st.FinanceCreateTag(r.Context(), req.Name, req.Color)
	if s.storeErr(w, err, "finance tag create") {
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) financeUpdateTag(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req finTagReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.storeErr(w, s.st.FinanceUpdateTag(r.Context(), id, req.Name, req.Color), "finance tag update") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) financeDeleteTag(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.storeErr(w, s.st.FinanceDeleteTag(r.Context(), id), "finance tag delete") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type finTagLinksReq struct {
	Key    string  `json:"key"`
	TagIDs []int64 `json:"tag_ids"`
}

// PUT /api/finance/tag-links {key, tag_ids} — 항목의 태그를 통째로 바꾼다.
func (s *Server) financeSetTags(w http.ResponseWriter, r *http.Request) {
	var req finTagLinksReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.storeErr(w, s.st.FinanceSetTags(r.Context(), req.Key, req.TagIDs), "finance tags") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type finLabelReq struct {
	Keys  []string `json:"keys"`
	Label string   `json:"label"`
	Cat1  string   `json:"cat1"`
}

// PUT /api/finance/labels {keys, label, cat1} — 둘 다 비우면 원래대로.
func (s *Server) financeSetLabel(w http.ResponseWriter, r *http.Request) {
	var req finLabelReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.storeErr(w, s.st.FinanceSetLabel(r.Context(), req.Keys, req.Label, req.Cat1), "finance label") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/finance/detail?owner=&key=a&key=b — 항목의 최근 거래.
func (s *Server) financeDetail(w http.ResponseWriter, r *http.Request) {
	owner, _ := strconv.ParseInt(r.URL.Query().Get("owner"), 10, 64)
	keys := r.URL.Query()["key"]
	if len(keys) == 0 || len(keys) > 50 {
		writeErr(w, http.StatusBadRequest, "key 가 필요해요")
		return
	}
	d, err := s.st.FinanceDetail(r.Context(), owner, keys, time.Now())
	if s.storeErr(w, err, "finance detail") {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

type finSpendReq struct {
	Key   string `json:"key"`
	Spend bool   `json:"spend"`
}

// POST /api/finance/spend {key, spend} — spend=false 면 '지출 아님'.
func (s *Server) financeSpend(w http.ResponseWriter, r *http.Request) {
	var req finSpendReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.storeErr(w, s.st.FinanceSetSpend(r.Context(), req.Key, req.Spend), "finance spend") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
