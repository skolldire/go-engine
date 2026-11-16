package router

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type CORS struct {
	Origins        []string      `mapstructure:"origins" json:"origins"`
	Methods        []string      `mapstructure:"methods" json:"methods"`
	Headers        []string      `mapstructure:"headers" json:"headers"`
	ExposedHeaders []string      `mapstructure:"exposed_headers" json:"exposed_headers"`
	Credentials    bool          `mapstructure:"credentials" json:"credentials"`
	MaxAge         time.Duration `mapstructure:"max_age" json:"max_age"`
}
type Config struct {
	AllowedHosts   []string `mapstructure:"allowed_hosts" json:"allowed_hosts"`
	TrustedProxies []string `mapstructure:"trusted_proxies" json:"trusted_proxies"`
	CORS           *CORS    `mapstructure:"cors" json:"cors"`
}
type Middleware func(http.Handler) http.Handler
type Policy struct {
	hosts   []string
	proxies []netip.Prefix
	cors    *CORS
}
type clientKey struct{}

func ClientIP(ctx context.Context) (netip.Addr, bool) {
	if ctx == nil {
		return netip.Addr{}, false
	}
	ip, ok := ctx.Value(clientKey{}).(netip.Addr)
	return ip, ok
}

func hostName(raw string) (string, error) {
	s := strings.ToLower(raw)
	if s == "" {
		return "", fmt.Errorf("empty host")
	}
	if strings.HasPrefix(s, "[") {
		h, p, e := net.SplitHostPort(s)
		if e == nil {
			if _, e = strconv.ParseUint(p, 10, 16); e != nil {
				return "", e
			}
			s = h
		} else if strings.HasSuffix(s, "]") {
			s = s[1 : len(s)-1]
		} else {
			return "", e
		}
	} else if strings.Count(s, ":") == 1 {
		h, p, e := net.SplitHostPort(s)
		if e != nil {
			return "", e
		}
		if _, e = strconv.ParseUint(p, 10, 16); e != nil {
			return "", e
		}
		s = h
	}
	if ip, e := netip.ParseAddr(s); e == nil {
		if ip.Zone() != "" {
			return "", fmt.Errorf("zone")
		}
		return ip.Unmap().String(), nil
	}
	s = strings.TrimSuffix(s, ".")
	if len(s) == 0 || len(s) > 253 {
		return "", fmt.Errorf("host length")
	}
	for _, label := range strings.Split(s, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("label")
		}
		for _, c := range label {
			valid := c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-'
			if !valid {
				return "", fmt.Errorf("label")
			}
		}
	}
	return s, nil
}
func configHost(raw string) (string, error) {
	if strings.HasPrefix(raw, "*.") {
		h, e := hostName(raw[2:])
		if e != nil {
			return "", e
		}
		if _, e := netip.ParseAddr(h); e == nil {
			return "", fmt.Errorf("wildcard IP")
		}
		return "*." + h, nil
	}
	return hostName(raw)
}
func hostMatches(rule, h string) bool {
	if !strings.HasPrefix(rule, "*.") {
		return rule == h
	}
	suffix := rule[1:]
	if !strings.HasSuffix(h, suffix) {
		return false
	}
	left := strings.TrimSuffix(h, suffix)
	return left != "" && !strings.Contains(left, ".")
}
func token(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		valid := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)
		if !valid {
			return false
		}
	}
	return true
}
func NewPolicy(cfg Config) (*Policy, error) {
	p := &Policy{}
	if len(cfg.AllowedHosts) == 0 {
		return nil, &ConfigError{"AllowedHosts", "is required"}
	}
	for _, raw := range cfg.AllowedHosts {
		h, e := configHost(raw)
		if e != nil {
			return nil, &ConfigError{"AllowedHosts", "invalid host"}
		}
		p.hosts = append(p.hosts, h)
	}
	for _, raw := range cfg.TrustedProxies {
		prefix, e := netip.ParsePrefix(raw)
		if e != nil {
			ip, err := netip.ParseAddr(raw)
			if err != nil || ip.Zone() != "" {
				return nil, &ConfigError{"TrustedProxies", "invalid IP or CIDR"}
			}
			prefix = netip.PrefixFrom(ip, ip.BitLen())
		}
		p.proxies = append(p.proxies, prefix.Masked())
	}
	if cfg.CORS != nil {
		c := *cfg.CORS
		c.Origins = append([]string(nil), c.Origins...)
		c.Methods = append([]string(nil), c.Methods...)
		c.Headers = append([]string(nil), c.Headers...)
		c.ExposedHeaders = append([]string(nil), c.ExposedHeaders...)
		for i, o := range c.Origins {
			if o == "*" {
				if c.Credentials || len(c.Origins) != 1 {
					return nil, &ConfigError{"CORS.Origins", "wildcard must be alone without credentials"}
				}
				continue
			}
			normalized, err := originName(o)
			if err != nil {
				return nil, &ConfigError{"CORS.Origins", "invalid origin"}
			}
			c.Origins[i] = normalized
		}
		if len(c.Methods) == 0 {
			c.Methods = []string{"GET", "HEAD", "POST"}
		}
		if len(c.Headers) == 0 {
			c.Headers = []string{"Accept", "Accept-Language", "Content-Language", "Content-Type", "Range"}
		}
		if c.Credentials && contains(c.Headers, "*", false) {
			return nil, &ConfigError{"CORS.Headers", "wildcard with credentials"}
		}

		for _, list := range [][]string{c.Methods, c.Headers, c.ExposedHeaders} {
			for _, s := range list {
				if !token(s) {
					return nil, &ConfigError{"CORS", "invalid HTTP token"}
				}
			}
		}
		if c.MaxAge < 0 {
			return nil, &ConfigError{"CORS.MaxAge", "must be nonnegative"}
		}
		p.cors = &c
	}
	return p, nil
}
func (p *Policy) trusts(ip netip.Addr) bool {
	for _, prefix := range p.proxies {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}
func peerAddr(raw string) netip.Addr {
	h, _, e := net.SplitHostPort(raw)
	if e == nil {
		raw = h
	}
	ip, _ := netip.ParseAddr(raw)
	return ip.Unmap()
}
func (p *Policy) client(r *http.Request) netip.Addr {
	peer := peerAddr(r.RemoteAddr)
	if !p.trusts(peer) {
		return peer
	}
	if raw := strings.Join(r.Header.Values("X-Forwarded-For"), ","); raw != "" {
		parts := strings.Split(raw, ",")
		client := peer
		for i := len(parts) - 1; i >= 0; i-- {
			ip, e := netip.ParseAddr(strings.TrimSpace(parts[i]))
			if e != nil {
				return peer
			}
			ip = ip.Unmap()
			client = ip
			if !p.trusts(ip) {
				return ip
			}
		}
		return client
	}
	if ip, e := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-Real-IP"))); e == nil {
		return ip.Unmap()
	}
	return peer
}
func (p *Policy) Wrap(next http.Handler) http.Handler {
	if nilPointer(next) {
		return nil
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, e := hostName(r.Host)
		if strings.Count(r.Host, ":") > 1 && !strings.HasPrefix(r.Host, "[") {
			e = fmt.Errorf("unbracketed IPv6 authority")
		}
		allowed := false
		if e == nil {
			for _, rule := range p.hosts {
				if hostMatches(rule, h) {
					allowed = true
					break
				}
			}
		}
		if !allowed {
			w.WriteHeader(http.StatusMisdirectedRequest)
			return
		}
		if ip := p.client(r); ip.IsValid() {
			r = r.WithContext(context.WithValue(r.Context(), clientKey{}, ip))
		}
		if p.applyCORS(w, r) {
			return
		}
		next.ServeHTTP(w, r)
	})
}
func contains(list []string, s string, fold bool) bool {
	for _, v := range list {
		if v == "*" || v == s || fold && strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
func originName(raw string) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme == "" || u.Host == "" || u.Opaque != "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(raw, "\r\n\t ?#") {
		return "", fmt.Errorf("origin")
	}
	if _, e := hostName(u.Host); e != nil {
		return "", e
	}
	return strings.ToLower(raw), nil
}
func (p *Policy) applyCORS(w http.ResponseWriter, r *http.Request) bool {
	c := p.cors
	if c == nil {
		return false
	}
	w.Header().Add("Vary", "Origin")
	origin := r.Header.Get("Origin")
	pre := r.Method == "OPTIONS" && r.Header.Get("Access-Control-Request-Method") != ""
	if origin == "" && !pre {
		return false
	}
	normalized, e := originName(origin)
	allowed := e == nil && contains(c.Origins, normalized, false)
	if pre {
		w.Header().Add("Vary", "Access-Control-Request-Method")
		w.Header().Add("Vary", "Access-Control-Request-Headers")
		allowed = allowed && contains(c.Methods, r.Header.Get("Access-Control-Request-Method"), false)
		for _, h := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
			if h = strings.TrimSpace(h); h != "" && (!token(h) || !contains(c.Headers, h, true)) {
				allowed = false
			}
		}
		if !allowed {
			w.WriteHeader(403)
			return true
		}
	} else if !allowed {
		return false
	}
	out := origin
	if len(c.Origins) == 1 && c.Origins[0] == "*" {
		out = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", out)
	if c.Credentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	if pre {
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(c.Methods, ", "))
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(c.Headers, ", "))
		if seconds := c.MaxAge / time.Second; seconds > 0 {
			w.Header().Set("Access-Control-Max-Age", strconv.FormatInt(int64(seconds), 10))
		}
		w.WriteHeader(204)
		return true
	}
	if len(c.ExposedHeaders) > 0 {
		w.Header().Set("Access-Control-Expose-Headers", strings.Join(c.ExposedHeaders, ", "))
	}
	return false
}
