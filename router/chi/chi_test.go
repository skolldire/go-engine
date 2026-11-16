package chi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	native "github.com/go-chi/chi/v5"
	"github.com/skolldire/go-engine/router"
)

func TestAdapter(t *testing.T) {
	a, e := New(router.Config{AllowedHosts: []string{"example.com"}})
	if e != nil {
		t.Fatal(e)
	}
	a.Native().Get("/item/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Request-ID") != "" {
			t.Fatal("implicit middleware")
		}
		_, _ = fmt.Fprint(w, native.URLParam(r, "id")) //nolint:gosec // Local route fixture; not exposed on a network listener.
	})
	for _, host := range []string{"example.com", "evil.com"} {
		r := httptest.NewRequest("GET", "http://"+host+"/item/7", nil)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if host == "example.com" && w.Body.String() != "7" || host == "evil.com" && w.Code != 421 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if _, e := New(router.Config{}); e == nil {
		t.Fatal("invalid policy accepted")
	}
}
func FuzzHost(f *testing.F) {
	f.Add("example.com")
	f.Fuzz(func(t *testing.T, host string) {
		if len(host) > 1024 {
			return
		}
		a, _ := New(router.Config{AllowedHosts: []string{"example.com"}})
		a.Native().Get("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(201) })
		r := httptest.NewRequest("GET", "http://example.com/", nil)
		r.Host = host
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != 201 && w.Code != 421 {
			t.Fatal(w.Code)
		}
	})
}
func BenchmarkHandler(b *testing.B) {
	for _, wrapped := range []bool{false, true} {
		b.Run(fmt.Sprint(wrapped), func(b *testing.B) {
			var h http.Handler
			if wrapped {
				a, _ := New(router.Config{AllowedHosts: []string{"example.com"}})
				a.Native().Get("/", func(http.ResponseWriter, *http.Request) {})
				h = a.Handler()
			} else {
				m := native.NewMux()
				m.Get("/", func(http.ResponseWriter, *http.Request) {})
				h = m
			}
			r := httptest.NewRequest("GET", "http://example.com/", nil)
			w := httptest.NewRecorder()
			b.ReportAllocs()
			for b.Loop() {
				h.ServeHTTP(w, r)
			}
		})
	}
}
