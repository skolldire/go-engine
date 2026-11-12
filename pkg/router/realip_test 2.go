package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// captureRemoteAddr runs trustedRealIP with the given trusted proxies and
// returns the r.RemoteAddr observed by the downstream handler.
func captureRemoteAddr(t *testing.T, trusted []string, remoteAddr string, headers map[string]string) string {
	t.Helper()

	var seen string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = r.RemoteAddr
	})

	mw := trustedRealIP(newTrustedProxySet(trusted))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	return seen
}

func TestTrustedRealIP_NoProxiesIgnoresHeaders(t *testing.T) {
	got := captureRemoteAddr(t, nil, "10.0.0.1:5555", map[string]string{
		"X-Forwarded-For": "1.2.3.4",
	})
	assert.Equal(t, "10.0.0.1:5555", got, "without trusted proxies the header must be ignored")
}

func TestTrustedRealIP_TrustedPeerUsesXFF(t *testing.T) {
	got := captureRemoteAddr(t, []string{"10.0.0.1"}, "10.0.0.1:5555", map[string]string{
		"X-Forwarded-For": "203.0.113.7",
	})
	assert.Equal(t, "203.0.113.7:5555", got)
}

func TestTrustedRealIP_UntrustedPeerKeepsRemoteAddr(t *testing.T) {
	got := captureRemoteAddr(t, []string{"10.0.0.1"}, "192.0.2.50:5555", map[string]string{
		"X-Forwarded-For": "203.0.113.7",
	})
	assert.Equal(t, "192.0.2.50:5555", got, "an untrusted peer must not be able to spoof via XFF")
}

func TestTrustedRealIP_CIDRTrust(t *testing.T) {
	got := captureRemoteAddr(t, []string{"10.0.0.0/8"}, "10.1.2.3:5555", map[string]string{
		"X-Forwarded-For": "203.0.113.9",
	})
	assert.Equal(t, "203.0.113.9:5555", got)
}

func TestTrustedRealIP_WalksPastTrustedProxiesInXFF(t *testing.T) {
	// Client -> edge(203.0.113.1) -> internal proxy(10.0.0.2) -> app(peer 10.0.0.1)
	// XFF: client, edge, internal-proxy. Both 10.x are trusted, so the real
	// client is the rightmost untrusted entry: 203.0.113.1 is the edge, the
	// actual client is 198.51.100.23.
	got := captureRemoteAddr(t, []string{"10.0.0.0/8"}, "10.0.0.1:5555", map[string]string{
		"X-Forwarded-For": "198.51.100.23, 203.0.113.1, 10.0.0.2",
	})
	assert.Equal(t, "203.0.113.1:5555", got)
}

func TestTrustedRealIP_FallsBackToXRealIP(t *testing.T) {
	got := captureRemoteAddr(t, []string{"10.0.0.1"}, "10.0.0.1:5555", map[string]string{
		"X-Real-IP": "203.0.113.20",
	})
	assert.Equal(t, "203.0.113.20:5555", got)
}

func TestConfigureBasicRoutes_PprofOptIn(t *testing.T) {
	// Disabled by default.
	off := NewService(Config{Port: "0"})
	rec := httptest.NewRecorder()
	off.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code, "pprof must be disabled by default")

	// Enabled opt-in (non-prod profile in tests).
	on := NewService(Config{Port: "0", EnablePprof: true})
	rec2 := httptest.NewRecorder()
	on.Router().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	assert.NotEqual(t, http.StatusNotFound, rec2.Code, "pprof must be reachable when enabled")
}
