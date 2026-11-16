package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"
)

func TestRequestID(t *testing.T) {
	for _, accept := range []bool{false, true} {
		mw, e := RequestID(RequestIDConfig{AcceptIncoming: accept, Generate: func() string { return "generated" }})
		if e != nil {
			t.Fatal(e)
		}
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("X-Request-ID", "incoming")
		w := httptest.NewRecorder()
		mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := RequestIDFrom(r.Context())
			want := "generated"
			if accept {
				want = "incoming"
			}
			if !ok || id != want || w.Header().Get("X-Request-ID") != want {
				t.Fatal(id)
			}
		})).ServeHTTP(w, r)
	}
	mw, _ := RequestID(RequestIDConfig{})
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := RequestIDFrom(r.Context())
		if len(id) != 32 {
			t.Fatal(id)
		}
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if _, e := RequestID(RequestIDConfig{Header: "bad\nheader"}); e == nil {
		t.Fatal("invalid header")
	}
}
func TestRecover(t *testing.T) {
	if _, e := Recover(nil); e == nil {
		t.Fatal("nil callback accepted")
	}
	for _, abort := range []bool{false, true} {
		func() {
			called := false
			w := httptest.NewRecorder()
			mw, e := Recover(func(w http.ResponseWriter, r *http.Request, v any) { called = true; w.WriteHeader(500) })
			if e != nil {
				t.Fatal(e)
			}
			defer func() {
				v := recover()
				if abort && v != http.ErrAbortHandler {
					t.Fatal("abort not propagated")
				}
				if !abort && v != nil {
					t.Fatal(v)
				}
				if called == abort || w.Body.Len() != 0 {
					t.Fatal(called, w.Body.String())
				}
			}()
			mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				if abort {
					panic(http.ErrAbortHandler)
				}
				panic("secret")
			})).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		}()
	}
}

type testValueKey struct{}

func TestTimeout(t *testing.T) {
	for _, d := range []time.Duration{0, -1} {
		if _, e := Timeout(d); e == nil {
			t.Fatal("invalid timeout accepted")
		}
	}
	synctest.Test(t, func(t *testing.T) {
		ctx := context.WithValue(context.Background(), testValueKey{}, "value")
		r := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
		mw, e := Timeout(time.Second)
		if e != nil {
			t.Fatal(e)
		}
		w := httptest.NewRecorder()
		mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := r.Context().Deadline(); !ok {
				t.Fatal("missing deadline")
			}
			if r.Context().Value(testValueKey{}) != "value" {
				t.Fatal("lost context")
			}
			<-r.Context().Done()
			w.WriteHeader(201)
		})).ServeHTTP(w, r)
		if w.Code != 201 {
			t.Fatal("middleware replaced response")
		}
	})
}
