package router

import (
	"net"
	"net/http"
	"strings"
)

// trustedProxySet holds the parsed trusted-proxy IPs and CIDR ranges. Only when
// the immediate peer (r.RemoteAddr) is a trusted proxy are the forwarding
// headers believed; otherwise they are ignored to prevent client IP spoofing.
type trustedProxySet struct {
	ips   map[string]struct{}
	cidrs []*net.IPNet
}

func newTrustedProxySet(entries []string) *trustedProxySet {
	s := &trustedProxySet{ips: make(map[string]struct{})}
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if _, ipnet, err := net.ParseCIDR(e); err == nil {
			s.cidrs = append(s.cidrs, ipnet)
			continue
		}
		if ip := net.ParseIP(e); ip != nil {
			s.ips[ip.String()] = struct{}{}
		}
	}
	return s
}

func (s *trustedProxySet) empty() bool {
	return len(s.ips) == 0 && len(s.cidrs) == 0
}

func (s *trustedProxySet) trusts(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if _, ok := s.ips[ip.String()]; ok {
		return true
	}
	for _, c := range s.cidrs {
		if c.Contains(ip) {
			return true
		}
	}
	return false
}

// trustedRealIP returns middleware that rewrites r.RemoteAddr to the real client
// IP only when the request arrives through a trusted proxy. It reads
// X-Forwarded-For (walking from the rightmost entry, skipping trusted proxies)
// and falls back to X-Real-IP. When no proxies are configured, or the immediate
// peer is not trusted, r.RemoteAddr is left untouched so forged headers cannot
// spoof the client IP.
func trustedRealIP(trusted *trustedProxySet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if trusted == nil || trusted.empty() {
				next.ServeHTTP(w, r)
				return
			}

			peerIP := remoteIP(r.RemoteAddr)
			if !trusted.trusts(peerIP) {
				// The direct peer is not a trusted proxy: do not believe headers.
				next.ServeHTTP(w, r)
				return
			}

			if clientIP := clientIPFromHeaders(r, trusted); clientIP != "" {
				if _, port, err := net.SplitHostPort(r.RemoteAddr); err == nil {
					r.RemoteAddr = net.JoinHostPort(clientIP, port)
				} else {
					r.RemoteAddr = clientIP
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIPFromHeaders extracts the client IP from X-Forwarded-For by walking the
// list from right to left and returning the first address that is not itself a
// trusted proxy. It falls back to X-Real-IP.
func clientIPFromHeaders(r *http.Request, trusted *trustedProxySet) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			ipStr := strings.TrimSpace(parts[i])
			ip := net.ParseIP(ipStr)
			if ip == nil {
				continue
			}
			if trusted.trusts(ip) {
				continue
			}
			return ip.String()
		}
	}
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		if ip := net.ParseIP(xrip); ip != nil {
			return ip.String()
		}
	}
	return ""
}

func remoteIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return net.ParseIP(host)
}
