package api

import (
	"net/http"
)

// securityHeaders는 공개(tailscale funnel) 노출을 전제로 한 기본 방어선이다.
// 전부 정적 헤더라 비용이 없고, 없을 때만 문제가 된다.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// 클릭재킹: 다른 사이트가 우리 앱을 iframe에 넣지 못하게.
		h.Set("X-Frame-Options", "DENY")
		// MIME 스니핑으로 업로드/에러 본문이 스크립트로 해석되는 걸 막는다.
		h.Set("X-Content-Type-Options", "nosniff")
		// 외부로 나갈 때 경로를 흘리지 않는다.
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		// 쓰지 않는 브라우저 기능을 끈다.
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), interest-cohort=()")
		// CSP: 번들·스타일·폰트가 전부 자기 출처에서 온다. 외부 로드가 없으므로
		// self만 허용하면 된다. Vite 개발 서버는 인라인 스크립트와 HMR용 웹소켓이
		// 필요해 dev에서는 느슨하게 둔다.
		h.Set("Content-Security-Policy", cspValue(r))
		next.ServeHTTP(w, r)
	})
}

// devCSP는 --dev에서만 쓰인다. 패키지 변수로 둔 건 요청마다 계산하지 않기 위해서다.
var devCSP bool

func cspValue(*http.Request) string {
	if devCSP {
		return "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; " +
			"style-src 'self' 'unsafe-inline'; connect-src 'self' ws: wss:; img-src 'self' data:; " +
			"frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
	}
	return "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"connect-src 'self'; img-src 'self' data:; font-src 'self'; object-src 'none'; " +
		"frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
}
