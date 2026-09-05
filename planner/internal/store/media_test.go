package store

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"
)

// 세로로 긴 사진을 만든다. 왼쪽 위 모서리만 빨갛게 칠해 방향을 알아볼 수 있게.
func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{200, 200, 200, 255}
			if x < w/4 && y < h/4 {
				c = color.RGBA{255, 0, 0, 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMediaPutResizesAndDedupes(t *testing.T) {
	s, _ := newDiaryStore(t)
	ctx := context.Background()

	big := testJPEG(t, 3000, 4000)
	m, err := s.MediaPut(ctx, big, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Width != 1200 || m.Height != 1600 {
		t.Fatalf("긴 변이 1600 으로 줄어야 하는데 %dx%d", m.Width, m.Height)
	}
	p, err := s.MediaPath(m.ID + ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("파일이 없다: %v", err)
	}

	// 같은 걸 다시 올리면 같은 이름, 표에도 한 줄.
	m2, err := s.MediaPut(ctx, big, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m2.ID != m.ID {
		t.Fatal("같은 사진인데 이름이 다르다")
	}
	if n, c, _ := s.MediaBytes(ctx); c != 1 || n != m.Bytes {
		t.Fatalf("표에 %d줄 %d바이트 — 한 줄이어야 한다", c, n)
	}

	// 이미 작은 JPEG 는 바이트 그대로 둔다.
	small := testJPEG(t, 800, 600)
	m3, err := s.MediaPut(ctx, small, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m3.Bytes != int64(len(small)) {
		t.Fatal("작은 사진을 다시 인코딩했다")
	}

	// 이름 검사: 우리가 만든 모양이 아니면 경로를 주지 않는다.
	for _, bad := range []string{"../planner.db", "abc.jpg", m.ID + ".png", m.ID + "/x.jpg"} {
		if _, err := s.MediaPath(bad); err == nil {
			t.Fatalf("%q 에 경로를 줬다", bad)
		}
	}
	if _, err := s.MediaPut(ctx, []byte("not an image"), nil); err == nil {
		t.Fatal("사진이 아닌 걸 받았다")
	}
}

// EXIF 로 "시계 방향 90도" 라고 적힌 가로 사진은 세로로 저장되어야 한다.
func TestMediaPutBakesExifOrientation(t *testing.T) {
	s, _ := newDiaryStore(t)
	// 가로 2000x1000 + Orientation=6 → 저장은 세로(높이가 더 큼).
	src := testJPEG(t, 2000, 1000)
	withExif := injectOrientation(t, src, 6)
	if got := exifOrientation(withExif); got != 6 {
		t.Fatalf("EXIF 읽기 = %d, want 6", got)
	}
	m, err := s.MediaPut(context.Background(), withExif, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Height <= m.Width {
		t.Fatalf("회전이 안 구워졌다: %dx%d", m.Width, m.Height)
	}
	if m.Height != 1600 || m.Width != 800 {
		t.Fatalf("회전 후 축소가 이상하다: %dx%d", m.Width, m.Height)
	}
}

// APP1(Exif) 세그먼트를 SOI 바로 뒤에 끼워 넣는다. IFD0 에 Orientation 하나.
func injectOrientation(t *testing.T, jpg []byte, o int) []byte {
	t.Helper()
	tiff := []byte{
		'M', 'M', 0, 0x2A, 0, 0, 0, 8, // 빅엔디언, IFD0 at 8
		0, 1, // 엔트리 1개
		0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, byte(o), 0, 0, // Orientation SHORT
		0, 0, 0, 0, // next IFD
	}
	payload := append([]byte("Exif\x00\x00"), tiff...)
	size := len(payload) + 2
	seg := append([]byte{0xFF, 0xE1, byte(size >> 8), byte(size)}, payload...)
	out := append([]byte{0xFF, 0xD8}, seg...)
	return append(out, jpg[2:]...)
}

func TestMediaDeleteOnlyWhenUnused(t *testing.T) {
	s, uid := newDiaryStore(t)
	ctx := context.Background()
	m, err := s.MediaPut(ctx, testJPEG(t, 800, 400), nil)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := s.DiaryCreate(ctx, "2026-01-01", "", uid)
	body := `[{"type":"image","props":{"url":"` + m.URL + `"},"children":[]}]`
	if _, err := s.DiaryUpdate(ctx, e.ID, DiaryInput{Content: &body}); err != nil {
		t.Fatal(err)
	}
	if err := s.MediaDelete(ctx, m.ID+".jpg"); err != ErrInUse {
		t.Fatalf("글이 쓰는 사진을 지웠다: %v", err)
	}
	empty := `[]`
	_, _ = s.DiaryUpdate(ctx, e.ID, DiaryInput{Content: &empty})
	if err := s.MediaDelete(ctx, m.ID+".jpg"); err != nil {
		t.Fatal(err)
	}
	if n, c, _ := s.MediaBytes(ctx); c != 0 || n != 0 {
		t.Fatal("표에 남았다")
	}
}
