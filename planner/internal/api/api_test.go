package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

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
	if _, err := st.CreateUser(context.Background(), "엄마", "pass1234"); err != nil {
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
	c.must("POST", "/api/login", map[string]string{"name": "엄마", "password": "wrong"}, nil, 401)
	c.must("POST", "/api/login", map[string]string{"name": "엄마", "password": "pass1234"}, &me, 200)
	if me["name"] != "엄마" {
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

	// create two cards, move the first into 진행 중
	var a, b store.Card
	c.must("POST", "/api/columns/"+itoa(todo)+"/cards", map[string]any{"title": "장보기", "due_date": "2026-09-25"}, &a, 201)
	c.must("POST", "/api/columns/"+itoa(todo)+"/cards", map[string]any{"title": "청소"}, &b, 201)
	c.must("POST", "/api/columns/"+itoa(todo)+"/cards", map[string]any{"title": ""}, nil, 400)
	c.must("POST", "/api/columns/"+itoa(todo)+"/cards", map[string]any{"title": "x", "due_date": "25/09"}, nil, 400)

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

	// calendar sees the due card
	var cal struct {
		Events   []store.Event `json:"events"`
		DueCards []store.Card  `json:"due_cards"`
	}
	c.must("GET", "/api/calendar?from=2026-09-01&to=2026-10-01", nil, &cal, 200)
	if len(cal.DueCards) != 1 || cal.DueCards[0].Title != "장보기" {
		t.Fatalf("due cards = %+v", cal.DueCards)
	}

	// event validation + range
	var ev store.Event
	c.must("POST", "/api/events", map[string]any{"title": "병원", "start_at": "2026-09-22T10:30"}, &ev, 201)
	c.must("POST", "/api/events", map[string]any{"title": "여행", "start_at": "2026-09-27", "end_at": "2026-09-29", "all_day": true}, nil, 201)
	c.must("POST", "/api/events", map[string]any{"title": "bad", "start_at": "2026-09-27", "all_day": false}, nil, 400) // needs HH:MM
	c.must("POST", "/api/events", map[string]any{"title": "bad", "start_at": "2026-09-29T10:00", "end_at": "2026-09-28T10:00"}, nil, 400)
	c.must("GET", "/api/calendar?from=2026-09-22&to=2026-09-23", nil, &cal, 200)
	if len(cal.Events) != 1 || cal.Events[0].Title != "병원" {
		t.Fatalf("events on 22nd = %+v", cal.Events)
	}
	// multi-day event overlaps a window that starts mid-event
	c.must("GET", "/api/calendar?from=2026-09-28&to=2026-09-29", nil, &cal, 200)
	if len(cal.Events) != 1 || cal.Events[0].Title != "여행" {
		t.Fatalf("events on 28th = %+v", cal.Events)
	}

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

func itoa(i int64) string {
	b, _ := json.Marshal(i)
	return string(b)
}
