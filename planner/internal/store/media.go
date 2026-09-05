package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/image/draw"
)

// 파일 이름은 내용의 sha256 — 경로 탐색이 막히고 영구 캐시해도 된다.

// MediaMaxEdge 는 저장할 때 긴 변의 상한.
const MediaMaxEdge = 1600

// 디코딩 허용 상한. 24MP 를 넘으면 RGBA 가 100MB+ (Pi 메모리 921MB).
const mediaMaxPixels = 24 << 20

var mediaName = regexp.MustCompile(`^[a-f0-9]{64}\.jpg$`)

// Media 는 저장된 사진 한 장이다.
type Media struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Bytes     int64  `json:"bytes"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	CreatedBy *int64 `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
}

// MediaDir 는 사진이 놓이는 디렉터리다.
func (s *Store) MediaDir() string { return filepath.Join(filepath.Dir(s.path), "media") }

// MediaPath 는 이름이 우리가 만든 모양일 때만 경로를 준다(경로 탐색 방지).
func (s *Store) MediaPath(name string) (string, error) {
	if !mediaName.MatchString(name) {
		return "", ErrNotFound
	}
	return filepath.Join(s.MediaDir(), name), nil
}

// MediaPut 은 JPEG 로 정규화해 저장한다(EXIF 회전 굽기, 긴 변 축소). 이미 작은 JPEG 는 그대로 둔다.
func (s *Store) MediaPut(ctx context.Context, data []byte, userID *int64) (Media, error) {
	if len(data) == 0 {
		return Media{}, invalid("빈 파일이에요")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Media{}, invalid("사진 파일이 아니에요")
	}
	if cfg.Width*cfg.Height > mediaMaxPixels {
		return Media{}, invalid("사진이 너무 커요")
	}

	orient := 1
	if format == "jpeg" {
		orient = exifOrientation(data)
	}
	longEdge := max(cfg.Width, cfg.Height)
	keep := format == "jpeg" && orient == 1 && longEdge <= MediaMaxEdge

	out := data
	w, h := cfg.Width, cfg.Height
	if !keep {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return Media{}, invalid("사진을 읽지 못했어요")
		}
		// 줄인 뒤 돌린다 — 회전이 만지는 픽셀 수가 줄어든다.
		b := img.Bounds()
		w, h = b.Dx(), b.Dy()
		if max(w, h) > MediaMaxEdge {
			scale := float64(MediaMaxEdge) / float64(max(w, h))
			nw, nh := int(float64(w)*scale+0.5), int(float64(h)*scale+0.5)
			dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
			draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
			img, w, h = dst, nw, nh
		}
		img = applyOrientation(img, orient)
		b = img.Bounds()
		w, h = b.Dx(), b.Dy()
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			return Media{}, err
		}
		out = buf.Bytes()
	}

	sum := sha256.Sum256(out)
	id := hex.EncodeToString(sum[:])
	dir := s.MediaDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Media{}, err
	}
	path := filepath.Join(dir, id+".jpg")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		// 임시 이름으로 쓰고 rename — 반쪽 파일이 정식 이름으로 남지 않게.
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, out, 0o644); err != nil {
			return Media{}, err
		}
		if err := os.Rename(tmp, path); err != nil {
			os.Remove(tmp)
			return Media{}, err
		}
	}

	m := Media{ID: id, URL: "/media/" + id + ".jpg", Bytes: int64(len(out)), Width: w, Height: h, CreatedAt: time.Now().Unix()}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO media (id, ext, bytes, width, height, created_by, created_at) VALUES (?, 'jpg', ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO NOTHING`, id, m.Bytes, w, h, userID, m.CreatedAt); err != nil {
		return Media{}, err
	}
	if userID != nil {
		v := *userID
		m.CreatedBy = &v
	}
	return m, nil
}

// MediaBytes 는 저장된 사진의 총량이다. 저장 공간 계산에 쓴다.
func (s *Store) MediaBytes(ctx context.Context) (n int64, count int, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT COALESCE(sum(bytes), 0), count(*) FROM media`).Scan(&n, &count)
	return
}

// Go image/jpeg 는 EXIF 를 무시한다. Orientation(0x0112)만 직접 읽는다.

func exifOrientation(data []byte) int {
	// JPEG 는 FFD8 로 시작하고 세그먼트가 FFxx + 길이(2) 로 이어진다.
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 { // 이미지 데이터 시작 / 끝
			return 1
		}
		size := int(binary.BigEndian.Uint16(data[i+2:]))
		if size < 2 || i+2+size > len(data) {
			return 1
		}
		if marker == 0xE1 {
			return orientationFromAPP1(data[i+4 : i+2+size])
		}
		i += 2 + size
	}
	return 1
}

func orientationFromAPP1(seg []byte) int {
	if len(seg) < 14 || string(seg[:6]) != "Exif\x00\x00" {
		return 1
	}
	t := seg[6:] // TIFF 헤더부터
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	if len(t) < 8 {
		return 1
	}
	ifd := int(bo.Uint32(t[4:]))
	if ifd+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[ifd:]))
	for k := 0; k < n; k++ {
		e := ifd + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 {
			v := int(bo.Uint16(t[e+8:]))
			if v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// applyOrientation 은 EXIF 방향(1~8)대로 픽셀을 돌린다.
// At/Set 은 Pi 에서 분 단위라 Pix 를 직접 복사한다.
func applyOrientation(img image.Image, o int) image.Image {
	if o == 1 {
		return img
	}
	src, ok := img.(*image.RGBA)
	if !ok {
		src = image.NewRGBA(img.Bounds())
		draw.Draw(src, src.Bounds(), img, img.Bounds().Min, draw.Src)
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	var dst *image.RGBA
	if o >= 5 {
		dst = image.NewRGBA(image.Rect(0, 0, h, w))
	} else {
		dst = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	for y := 0; y < h; y++ {
		row := src.Pix[y*src.Stride : y*src.Stride+w*4]
		for x := 0; x < w; x++ {
			var nx, ny int
			switch o {
			case 2: // 좌우 반전
				nx, ny = w-1-x, y
			case 3: // 180도
				nx, ny = w-1-x, h-1-y
			case 4: // 상하 반전
				nx, ny = x, h-1-y
			case 5: // 대각 반전
				nx, ny = y, x
			case 6: // 시계 90도
				nx, ny = h-1-y, x
			case 7: // 반대 대각 반전
				nx, ny = h-1-y, w-1-x
			case 8: // 반시계 90도
				nx, ny = y, w-1-x
			}
			d := ny*dst.Stride + nx*4
			copy(dst.Pix[d:d+4], row[x*4:x*4+4])
		}
	}
	return dst
}

func (m Media) String() string {
	return fmt.Sprintf("%s (%dx%d, %d bytes)", m.ID[:8], m.Width, m.Height, m.Bytes)
}

// MediaDelete 는 어떤 본문도 가리키지 않을 때만 지운다(한 파일을 여러 글이 공유할 수 있다).
func (s *Store) MediaDelete(ctx context.Context, name string) error {
	path, err := s.MediaPath(name)
	if err != nil {
		return err
	}
	var n int
	like := "%/media/" + name + "%"
	if err := s.db.QueryRowContext(ctx,
		`SELECT (SELECT count(*) FROM diary WHERE content LIKE ?) + (SELECT count(*) FROM cards WHERE content LIKE ?)`,
		like, like).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrInUse
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM media WHERE id=?`, strings.TrimSuffix(name, ".jpg")); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ErrInUse 는 아직 어딘가에서 쓰는 사진을 지우려 할 때다.
var ErrInUse = errors.New("아직 쓰는 사진이에요")
