package api

import (
	"net"
	"net/http"
	"strings"
)

// clientIP는 레이트리밋 버킷 키로 쓸 호출자 주소다.
//
// tailscale serve/funnel 뒤에서는 프록시가 같은 기계의 루프백에서 붙으므로
// RemoteAddr가 항상 127.0.0.1이고, 진짜 주소는 X-Forwarded-For에 온다.
// 다만 XFF는 위조 가능한 헤더라 **RemoteAddr가 루프백일 때만** 믿는다.
// 앱이 루프백 밖에서 직접 노출되는 일이 생기면 그땐 RemoteAddr가 진실이다.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !isLoopback(host) {
		return host
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return host
	}
	// 가장 왼쪽이 원 호출자다. 우리 앞에는 신뢰하는 프록시 하나뿐이다.
	first := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	if ip := net.ParseIP(first); ip != nil {
		return ip.String()
	}
	return host
}

func isLoopback(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
