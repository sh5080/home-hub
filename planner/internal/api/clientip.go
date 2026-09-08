package api

import (
	"net"
	"net/http"
	"strings"
)

// clientIP 는 레이트리밋 키. tailscale serve/funnel 뒤에선 RemoteAddr 가 루프백이라
// X-Forwarded-For 를 쓰되, 위조 가능하니 RemoteAddr 가 루프백일 때만 믿는다.
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
	// 가장 왼쪽이 원 호출자(앞에는 신뢰하는 프록시 하나뿐).
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
