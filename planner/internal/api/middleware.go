package api

import (
	"net/http"
)

// securityHeaders 는 공개(funnel) 노출 전제의 기본 방어선.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), interest-cohort=()")
		// CSP: 전부 자기 출처. dev(Vite)만 인라인 스크립트·HMR 웹소켓을 허용한다.
		h.Set("Content-Security-Policy", cspValue(r))
		next.ServeHTTP(w, r)
	})
}

var devCSP bool

func cspValue(*http.Request) string {
	if devCSP {
		return "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; " +
			"style-src 'self' 'unsafe-inline'; connect-src 'self' ws: wss:; img-src 'self' data: blob:; " +
			"frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
	}
	// img-src blob: — 올리기 전 사진을 폰 안의 임시 주소로 먼저 보여준다.
	return "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"connect-src 'self'; img-src 'self' data: blob:; font-src 'self'; object-src 'none'; " +
		"frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
}
