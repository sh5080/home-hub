package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 없는 정적 파일이 index.html 로 떨어지면 아이콘·매니페스트가 깨져도
// 200으로 보여서 배포 후에야 드러난다.
func TestMissingAssetIs404NotHTML(t *testing.T) {
	srv := httptest.NewServer(Handler())
	defer srv.Close()

	for _, p := range []string{"/nope-123.png", "/missing.json", "/assets/gone.js"} {
		res, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("%s → %d, 404여야 한다", p, res.StatusCode)
		}
	}
	// SPA 경로(확장자 없음)는 계속 index.html 이어야 딥링크가 산다.
	res, err := http.Get(srv.URL + "/babyfood/stock")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("SPA 경로 = %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
}
