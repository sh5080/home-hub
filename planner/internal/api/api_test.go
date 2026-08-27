package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sh5080/home-hub/planner/internal/store"
)

type client struct {
	t   *testing.T
	srv *httptest.Server
	jar []*http.Cookie
}

func newClient(t *testing.T) *client {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.CreateUser(context.Background(), "테스트1", "pass1234"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser(context.Background(), "테스트2", "pass5678"); err != nil {
		t.Fatal(err)
	}
	h := New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), true)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &client{t: t, srv: srv}
}

// do sends a JSON request with the session cookie and decodes into out (if non-nil).
func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.srv.URL+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, ck := range c.jar {
		req.AddCookie(ck)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	if cs := res.Cookies(); len(cs) > 0 {
		c.jar = cs
	}
	raw, _ := io.ReadAll(res.Body)
	if out != nil && len(raw) > 0 {
		// Unmarshal leaves fields absent from the JSON untouched, so a reused
		// target would carry stale values. Zero it first.
		v := reflect.ValueOf(out).Elem()
		v.Set(reflect.Zero(v.Type()))
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("%s %s: bad JSON %q: %v", method, path, raw, err)
		}
	}
	return res.StatusCode
}

func (c *client) must(method, path string, body any, out any, want int) {
	c.t.Helper()
	if got := c.do(method, path, body, out); got != want {
		c.t.Fatalf("%s %s: status %d, want %d", method, path, got, want)
	}
}

func TestSmoke(t *testing.T) {
	c := newClient(t)

	// unauthenticated
	c.must("GET", "/api/me", nil, nil, 401)
	c.must("GET", "/api/boards", nil, nil, 401)

	// login
	var me map[string]any
	c.must("POST", "/api/login", map[string]string{"name": "테스트1", "password": "wrong"}, nil, 401)
	c.must("POST", "/api/login", map[string]string{"name": "테스트1", "password": "pass1234"}, &me, 200)
	if me["name"] != "테스트1" {
		t.Fatalf("me = %v", me)
	}
	c.must("GET", "/api/me", nil, &me, 200)

	// seed board exists
	var boards []store.Board
	c.must("GET", "/api/boards", nil, &boards, 200)
	if len(boards) != 1 || boards[0].Name != "할 일" {
		t.Fatalf("boards = %+v", boards)
	}

	// board detail with 3 columns
	var d store.BoardDetail
	c.must("GET", "/api/boards/1", nil, &d, 200)
	if len(d.Columns) != 3 {
		t.Fatalf("columns = %d", len(d.Columns))
	}
	todo, doing := d.Columns[0].ID, d.Columns[1].ID

	// create two cards, move the first into 진행 중 (content는 블록 문서)
	var a, b store.Card
	c.must("POST", "/api/columns/"+itoa(todo)+"/cards", map[string]any{"title": "장보기", "due_at": "2026-09-25"}, &a, 201)
	c.must("POST", "/api/columns/"+itoa(todo)+"/cards", map[string]any{"title": "청소"}, &b, 201)
	c.must("POST", "/api/columns/"+itoa(todo)+"/cards", map[string]any{"title": ""}, nil, 400)
	c.must("POST", "/api/columns/"+itoa(todo)+"/cards", map[string]any{"title": "x", "due_at": "25/09"}, nil, 400)

	var moved store.Card
	c.must("PATCH", "/api/cards/"+itoa(a.ID), map[string]any{"column_id": doing, "position": 0}, &moved, 200)
	if moved.ColumnID != doing || moved.Position != 0 {
		t.Fatalf("moved = %+v", moved)
	}
	c.must("PATCH", "/api/cards/"+itoa(a.ID), map[string]any{"column_id": doing}, nil, 400) // position missing

	c.must("GET", "/api/boards/1", nil, &d, 200)
	if len(d.Columns[0].Cards) != 1 || d.Columns[0].Cards[0].Title != "청소" || d.Columns[0].Cards[0].Position != 0 {
		t.Fatalf("todo column after move = %+v", d.Columns[0].Cards)
	}
	if len(d.Columns[1].Cards) != 1 || d.Columns[1].Cards[0].Title != "장보기" {
		t.Fatalf("doing column after move = %+v", d.Columns[1].Cards)
	}

	// 캘린더는 별도 테이블이 아니라 날짜가 있는 카드를 본다.
	var cal struct {
		Cards []store.Card `json:"cards"`
	}
	c.must("GET", "/api/calendar?from=2026-09-01&to=2026-10-01", nil, &cal, 200)
	if len(cal.Cards) != 1 || cal.Cards[0].Title != "장보기" {
		t.Fatalf("calendar cards = %+v", cal.Cards)
	}

	// 여러 날 항목: 창 중간에서 시작해도 겹치면 잡힌다.
	var trip store.Card
	c.must("POST", "/api/columns/"+itoa(todo)+"/cards",
		map[string]any{"title": "여행", "due_at": "2026-09-27", "end_at": "2026-09-29"}, &trip, 201)
	c.must("GET", "/api/calendar?from=2026-09-28&to=2026-09-29", nil, &cal, 200)
	if len(cal.Cards) != 1 || cal.Cards[0].Title != "여행" {
		t.Fatalf("multi-day overlap = %+v", cal.Cards)
	}
	// 끝이 시작보다 빠르면 거절.
	c.must("POST", "/api/columns/"+itoa(todo)+"/cards",
		map[string]any{"title": "bad", "due_at": "2026-09-29", "end_at": "2026-09-28"}, nil, 400)
	// 시작 없이 끝만 있으면 거절.
	c.must("POST", "/api/columns/"+itoa(todo)+"/cards",
		map[string]any{"title": "bad", "end_at": "2026-09-28"}, nil, 400)

	// routines: Mon/Wed/Fri = bits 0,2,4 = 0b10101 = 21. 2026-09-21 is a Monday.
	var rt store.Routine
	c.must("POST", "/api/routines", map[string]any{"title": "운동", "weekdays_mask": 21, "time_of_day": "07:00"}, &rt, 201)
	c.must("POST", "/api/routines", map[string]any{"title": "x", "weekdays_mask": 0}, nil, 400)
	c.must("POST", "/api/routines", map[string]any{"title": "x", "time_of_day": "7am"}, nil, 400)

	var today []store.Routine
	c.must("GET", "/api/routines?date=2026-09-21", nil, &today, 200) // Mon → scheduled
	if len(today) != 1 || today[0].CheckedBy != nil {
		t.Fatalf("monday routines = %+v", today)
	}
	c.must("GET", "/api/routines?date=2026-09-22", nil, &today, 200) // Tue → not scheduled
	if len(today) != 0 {
		t.Fatalf("tuesday routines = %+v", today)
	}

	c.must("PUT", "/api/routines/"+itoa(rt.ID)+"/checks/2026-09-21", nil, nil, 204)
	c.must("PUT", "/api/routines/"+itoa(rt.ID)+"/checks/2026-09-21", nil, nil, 204) // idempotent
	c.must("GET", "/api/routines?date=2026-09-21", nil, &today, 200)
	if today[0].CheckedBy == nil || *today[0].CheckedBy != 1 {
		t.Fatalf("check not recorded: %+v", today[0])
	}
	var checks map[string][]string
	c.must("GET", "/api/routines/checks?from=2026-09-21&to=2026-09-28", nil, &checks, 200)
	if len(checks[itoa(rt.ID)]) != 1 {
		t.Fatalf("checks = %v", checks)
	}
	c.must("DELETE", "/api/routines/"+itoa(rt.ID)+"/checks/2026-09-21", nil, nil, 204)
	c.must("GET", "/api/routines?date=2026-09-21", nil, &today, 200)
	if today[0].CheckedBy != nil {
		t.Fatalf("check not cleared: %+v", today[0])
	}

	// unknown API path is JSON, not HTML
	c.must("GET", "/api/whatever", nil, nil, 404)
	c.must("GET", "/api/boards/1/whatever", nil, nil, 404)

	// logout kills the session
	c.must("POST", "/api/logout", nil, nil, 204)
	c.must("GET", "/api/me", nil, nil, 401)
}

// 비밀번호 재설정: 행위자 본인 확인이 게이트이고, 성공하면 대상의 세션이 끊긴다.
func TestPasswordReset(t *testing.T) {
	c := newClient(t)
	c.must("POST", "/api/login", map[string]string{"name": "테스트1", "password": "pass1234"}, nil, 200)

	// 테스트2(id 2)를 재설정하려면 테스트1 본인 비밀번호가 필요하다.
	c.must("POST", "/api/users/2/password",
		map[string]string{"current_password": "틀린비번", "new_password": "newpass123"}, nil, 403)
	// 8자 미만은 거부.
	c.must("POST", "/api/users/2/password",
		map[string]string{"current_password": "pass1234", "new_password": "short"}, nil, 400)
	// 정상 재설정.
	c.must("POST", "/api/users/2/password",
		map[string]string{"current_password": "pass1234", "new_password": "newpass123"}, nil, 204)

	// 테스트1 세션은 그대로다 (대상이 테스트2였으므로).
	c.must("GET", "/api/me", nil, nil, 200)

	// 테스트2는 새 비밀번호로만 들어간다.
	d := newClientSharing(t, c)
	d.must("POST", "/api/login", map[string]string{"name": "테스트2", "password": "pass5678"}, nil, 401)
	d.must("POST", "/api/login", map[string]string{"name": "테스트2", "password": "newpass123"}, nil, 200)
}

// 본인 재설정은 쿠키를 새로 발급해 로그아웃되지 않는다.
func TestSelfPasswordChangeKeepsSession(t *testing.T) {
	c := newClient(t)
	c.must("POST", "/api/login", map[string]string{"name": "테스트1", "password": "pass1234"}, nil, 200)
	c.must("POST", "/api/users/1/password",
		map[string]string{"current_password": "pass1234", "new_password": "brandnew123"}, nil, 204)
	c.must("GET", "/api/me", nil, nil, 200) // 새 쿠키로 계속 유효
}

// 로그인 레이트리밋이 HTTP 경로에서도 동작하고 Retry-After를 준다.
func TestLoginRateLimit(t *testing.T) {
	c := newClient(t)
	for i := 0; i < 5; i++ {
		c.must("POST", "/api/login", map[string]string{"name": "테스트1", "password": "wrong"}, nil, 401)
	}
	res := c.raw("POST", "/api/login", map[string]string{"name": "테스트1", "password": "pass1234"})
	if res.StatusCode != 429 {
		t.Fatalf("status %d, want 429 after repeated failures", res.StatusCode)
	}
	if res.Header.Get("Retry-After") == "" {
		t.Fatal("429 must carry Retry-After")
	}
}

// newClientSharing은 같은 서버를 쓰되 쿠키 병을 따로 가진 클라이언트다.
func newClientSharing(t *testing.T, c *client) *client {
	return &client{t: t, srv: c.srv}
}

// raw는 상태 코드와 헤더를 봐야 할 때 쓴다.
func (c *client) raw(method, path string, body any) *http.Response {
	c.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, c.srv.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for _, ck := range c.jar {
		req.AddCookie(ck)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	res.Body.Close()
	return res
}

func itoa(i int64) string {
	b, _ := json.Marshal(i)
	return string(b)
}

// 동시 로그인 폭주가 bcrypt를 병렬로 태우지 못하는지. 레이트리밋은 '확인 →
// bcrypt → 기록' 순서라 동시 요청을 못 막는다 — 세마포어가 그 축을 맡는다.
func TestConcurrentLoginsAreThrottled(t *testing.T) {
	c := newClient(t)
	const n = 12
	var wg sync.WaitGroup
	codes := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d := &client{t: t, srv: c.srv}
			codes[i] = d.do("POST", "/api/login", map[string]string{"name": "테스트1", "password": "wrong"}, nil)
		}(i)
	}
	wg.Wait()
	// 전부 처리되긴 해야 한다(429/503/401 중 하나) — 무응답이나 500은 안 된다.
	for i, code := range codes {
		switch code {
		case 401, 429, 503:
		default:
			t.Fatalf("request %d: unexpected status %d", i, code)
		}
	}
}

// 보안 헤더가 모든 응답에 붙는지.
func TestSecurityHeaders(t *testing.T) {
	c := newClient(t)
	res := c.raw("POST", "/api/login", map[string]string{"name": "x", "password": "y"})
	for _, h := range []string{"X-Frame-Options", "X-Content-Type-Options", "Content-Security-Policy", "Referrer-Policy"} {
		if res.Header.Get(h) == "" {
			t.Errorf("missing security header %s", h)
		}
	}
	if got := res.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", got)
	}
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP missing frame-ancestors: %q", csp)
	}
}

// 카드 상세 + 본문(블록 문서) 왕복. 서버가 description을 파생하는지,
// 허용하지 않는 블록을 거절하는지가 핵심이다.
func TestCardDetailAndContent(t *testing.T) {
	c := newClient(t)
	c.must("POST", "/api/login", map[string]string{"name": "테스트1", "password": "pass1234"}, nil, 200)

	var d store.BoardDetail
	c.must("GET", "/api/boards/1", nil, &d, 200)
	todo := d.Columns[0].ID

	var card store.Card
	c.must("POST", "/api/columns/"+itoa(todo)+"/cards", map[string]any{"title": "장보기"}, &card, 201)

	// 상세는 카드 + 보드 + 컬럼 선택지를 한 번에 준다.
	var detail store.CardDetail
	c.must("GET", "/api/cards/"+itoa(card.ID), nil, &detail, 200)
	if detail.Card.ID != card.ID || detail.Board.ID != 1 || len(detail.Columns) != 3 {
		t.Fatalf("detail = %+v", detail)
	}

	// 본문 저장 → description이 파생된다.
	doc := `[{"type":"heading","props":{"level":2},"content":[{"type":"text","text":"살 것"}],"children":[]},` +
		`{"type":"checkListItem","props":{"checked":false},"content":[{"type":"text","text":"우유"}],"children":[]}]`
	var saved store.Card
	c.must("PATCH", "/api/cards/"+itoa(card.ID), map[string]any{"content": doc}, &saved, 200)
	if saved.Content == nil || *saved.Content != doc {
		t.Fatalf("content not stored verbatim")
	}
	if !strings.Contains(saved.Description, "살 것") || !strings.Contains(saved.Description, "우유") {
		t.Fatalf("description should be derived from content, got %q", saved.Description)
	}

	// 모르는 블록 타입은 400이고, 타입 이름을 알려준다.
	var errBody map[string]string
	c.must("PATCH", "/api/cards/"+itoa(card.ID),
		map[string]any{"content": `[{"type":"evilScript","content":[],"children":[]}]`}, &errBody, 400)
	if !strings.Contains(errBody["error"], "evilScript") {
		t.Fatalf("error should name the rejected block type: %q", errBody["error"])
	}

	// 깨진 JSON도 400.
	c.must("PATCH", "/api/cards/"+itoa(card.ID), map[string]any{"content": "not json"}, nil, 400)

	// 거절된 뒤에도 원래 본문이 남아 있다.
	c.must("GET", "/api/cards/"+itoa(card.ID), nil, &detail, 200)
	if detail.Card.Content == nil || *detail.Card.Content != doc {
		t.Fatal("rejected write must not clobber stored content")
	}

	// 없는 카드는 404 JSON.
	c.must("GET", "/api/cards/99999", nil, nil, 404)
}

// 변경이 SSE로 흘러나오는지. 이게 깨지면 화면이 조용히 30초 낡은 채로 남는다.
func TestStreamNotifiesOnWrite(t *testing.T) {
	c := newClient(t)
	c.must("POST", "/api/login", map[string]string{"name": "테스트1", "password": "pass1234"}, nil, 200)

	req, _ := http.NewRequest("GET", c.srv.URL+"/api/stream", nil)
	for _, ck := range c.jar {
		req.AddCookie(ck)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("stream status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type %q", ct)
	}

	lines := make(chan string, 8)
	go func() {
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()

	// 서두에 retry 가 온다 — EventSource 가 정상 종료 후 재연결하도록.
	if got := waitLine(t, lines, "retry:"); got == "" {
		t.Fatal("retry 서두가 없다")
	}

	// 읽기 요청은 알리지 않는다.
	var d store.BoardDetail
	c.must("GET", "/api/boards/1", nil, &d, 200)

	// 쓰기 요청은 알린다.
	var card store.Card
	c.must("POST", "/api/columns/"+itoa(d.Columns[0].ID)+"/cards", map[string]any{"title": "새 카드"}, &card, 201)
	if got := waitLine(t, lines, "event: changed"); got == "" {
		t.Fatal("쓰기 후 changed 이벤트가 오지 않았다")
	}
}

// waitLine은 prefix로 시작하는 줄을 1초 안에 기다린다.
func waitLine(t *testing.T, lines chan string, prefix string) string {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case ln, ok := <-lines:
			if !ok {
				return ""
			}
			if strings.HasPrefix(ln, prefix) {
				return ln
			}
		case <-deadline:
			return ""
		}
	}
}

// GET만으로는 버전이 오르지 않는다.
func TestReadsDoNotBroadcast(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	h := newHub()
	before := h.version.Load()
	s := &Server{st: st, log: slog.New(slog.NewTextHandler(io.Discard, nil)), hub: h}
	rec := httptest.NewRecorder()
	s.broadcastOnWrite(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})).ServeHTTP(rec, httptest.NewRequest("GET", "/api/boards", nil))
	if h.version.Load() != before {
		t.Fatal("GET이 알림을 보냈다")
	}
	// 실패한 쓰기도 알리지 않는다.
	s.broadcastOnWrite(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(400)
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/boards", nil))
	if h.version.Load() != before {
		t.Fatal("실패한 쓰기가 알림을 보냈다")
	}
}

// 한도를 넘으면 생성은 507, 삭제는 계속 동작.
func TestQuotaEnforcement(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if _, err := st.CreateUser(ctx, "테스트1", "pass1234"); err != nil {
		t.Fatal(err)
	}
	h := New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), true)
	srv := httptest.NewServer(h)
	defer srv.Close()
	c := &client{t: t, srv: srv}
	c.must("POST", "/api/login", map[string]string{"name": "테스트1", "password": "pass1234"}, nil, 200)

	var d store.BoardDetail
	c.must("GET", "/api/boards/1", nil, &d, 200)
	col := d.Columns[0].ID

	var card store.Card
	c.must("POST", "/api/columns/"+itoa(col)+"/cards", map[string]any{"title": "여유 있을 때"}, &card, 201)

	st.SetQuota(1) // 초과 상태로 만든다
	c.must("POST", "/api/columns/"+itoa(col)+"/cards", map[string]any{"title": "막혀야 함"}, nil, 507)
	c.must("POST", "/api/boards", map[string]string{"name": "막혀야 함"}, nil, 507)
	// 수정과 삭제는 계속 된다 — 공간을 비울 수 있어야 한다.
	c.must("PATCH", "/api/cards/"+itoa(card.ID), map[string]any{"title": "고쳐짐"}, nil, 200)
	c.must("DELETE", "/api/cards/"+itoa(card.ID), nil, nil, 204)

	st.SetQuota(store.DefaultQuota)
	c.must("POST", "/api/columns/"+itoa(col)+"/cards", map[string]any{"title": "다시 됨"}, nil, 201)
}
