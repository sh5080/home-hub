package api

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sh5080/home-hub/planner/internal/auth"
	"github.com/sh5080/home-hub/planner/internal/store"
)

// 사진 올리기/내려받기. /media/ 는 /api/ 밖이지만 인증 뒤에 있다(Funnel 공개 + 아이 사진).

const mediaMaxUpload = 15 << 20

func (s *Server) registerMedia(m *http.ServeMux) {
	m.HandleFunc("POST /api/media", s.mediaUpload)
	m.HandleFunc("DELETE /api/media/{name}", s.mediaDelete)
	m.HandleFunc("GET /media/{name}", s.mediaGet)
}

// POST /api/media — multipart(file). decodeJSON 을 안 거친다(1MiB 상한).
func (s *Server) mediaUpload(w http.ResponseWriter, r *http.Request) {
	if s.quotaBlocked(w, r) {
		return
	}
	// ReadTimeout(15초)이 본문 전체에 걸린다. LTE 대비로 이 요청만 늘린다.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(2 * time.Minute))
	_ = rc.SetWriteDeadline(time.Now().Add(2 * time.Minute))

	r.Body = http.MaxBytesReader(w, r.Body, mediaMaxUpload)
	start := time.Now()
	f, _, err := r.FormFile("file")
	if err != nil {
		// 본문을 다 받기 전에 끊긴 경우(LTE, 배포 재시작)도 여기로 온다.
		s.log.Warn("media upload read", "err", err, "content_length", r.ContentLength, "ms", time.Since(start).Milliseconds())
		writeErr(w, http.StatusBadRequest, "사진이 너무 크거나(15MB) 형식이 맞지 않아요")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "사진을 읽지 못했어요")
		return
	}

	// 디코딩이 무거워(12MP=RGBA 50MB) 한 번에 하나만.
	s.mediaSem <- struct{}{}
	defer func() { <-s.mediaSem }()

	u, _ := auth.UserFrom(r.Context())
	uid := u.ID
	m, err := s.st.MediaPut(r.Context(), data, &uid)
	if err != nil {
		s.log.Warn("media put", "err", err, "in_bytes", len(data))
	}
	if s.storeErr(w, err, "media put") {
		return
	}
	// 성공 요청은 기본 로그에 안 남아서 사진만 따로 적는다.
	s.log.Info("media stored", "in_bytes", len(data), "out_bytes", m.Bytes, "w", m.Width, "h", m.Height, "ms", time.Since(start).Milliseconds())
	writeJSON(w, http.StatusCreated, m)
}

// GET /media/{name}
func (s *Server) mediaGet(w http.ResponseWriter, r *http.Request) {
	path, err := s.st.MediaPath(r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	// ServeFile 의 404 는 text/plain 이라 먼저 확인해 JSON 으로 답한다.
	if _, err := os.Stat(path); err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	// 이름이 내용 해시라 영구 캐시.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, filepath.Clean(path))
}

// DELETE /api/media/{name} — 아무 글도 쓰지 않을 때만 지운다.
func (s *Server) mediaDelete(w http.ResponseWriter, r *http.Request) {
	err := s.st.MediaDelete(r.Context(), r.PathValue("name"))
	if errors.Is(err, store.ErrInUse) {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	if s.storeErr(w, err, "media delete") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
