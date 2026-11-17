package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestOrdersAndOperations(t *testing.T) {
	app, e := build("127.0.0.1:0", "127.0.0.1:0", io.Discard)
	if e != nil {
		t.Fatal(e)
	}
	for _, tt := range []struct {
		method, path, body string
		status             int
	}{{"POST", "/orders", `{"id":"1","item":"book","amount_cents":200}`, 201}, {"GET", "/orders/1", "", 200}, {"GET", "/orders/missing", "", 404}, {"POST", "/orders", `{"id":""}`, 400}} {
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, httptest.NewRequest(tt.method, "http://localhost"+tt.path, strings.NewReader(tt.body)))
		if w.Code != tt.status || w.Header().Get("X-Request-ID") == "" {
			t.Fatal(tt.path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/ping", "/live", "/ready", "/deps"} {
		w := httptest.NewRecorder()
		app.OperationsHandler().ServeHTTP(w, httptest.NewRequest("GET", "http://localhost"+path, nil))
		if w.Code != 200 {
			t.Fatal(path, w.Code)
		}
		for _, s := range []string{"Err", "Value", "Stack"} {
			if strings.Contains(w.Body.String(), s) {
				t.Fatal(w.Body.String())
			}
		}
	}
	w := httptest.NewRecorder()
	app.OperationsHandler().ServeHTTP(w, httptest.NewRequest("GET", "http://localhost/debug/pprof/", nil))
	if w.Code != 404 {
		t.Fatal("pprof implicitly enabled")
	}
}

type failingStore struct{ panicSave bool }

func (f failingStore) save(context.Context, order) error {
	if f.panicSave {
		panic("private-panic-value")
	}
	return errors.New("private-storage-cause")
}
func (f failingStore) find(context.Context, string) (order, error) {
	return order{}, errors.New("private-storage-cause")
}
func TestFailureAndPanicPrivacy(t *testing.T) {
	for _, panicSave := range []bool{false, true} {
		var logs bytes.Buffer
		app, e := buildWithStore("127.0.0.1:0", "127.0.0.1:0", &logs, failingStore{panicSave})
		if e != nil {
			t.Fatal(e)
		}
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, httptest.NewRequest("POST", "http://localhost/orders", strings.NewReader(`{"id":"1","item":"book"}`)))
		if w.Code != 500 || strings.Contains(w.Body.String(), "private") {
			t.Fatal(w.Code, w.Body.String())
		}
		var problem map[string]any
		if e = json.Unmarshal(w.Body.Bytes(), &problem); e != nil || problem["request_id"] != w.Header().Get("X-Request-ID") {
			t.Fatal(w.Body.String())
		}
	}
}
func TestConcurrentStore(t *testing.T) {
	m := &memoryStore{orders: make(map[string]order)}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 32 {
				if e := m.save(context.Background(), order{ID: "same", Item: "book"}); e != nil {
					t.Error(e)
				}
				if _, e := m.find(context.Background(), "same"); e != nil {
					t.Error(e)
				}
			}
		})
	}
	wg.Wait()
}
func TestLifecycle(t *testing.T) {
	app, e := build("127.0.0.1:0", "127.0.0.1:0", io.Discard)
	if e != nil {
		t.Fatal(e)
	}
	api, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	ops, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		_ = api.Close()
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.RunListeners(ctx, api, ops) }()
	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr}
	for _, addr := range []string{api.Addr().String(), ops.Addr().String()} {
		resp, e := client.Get("http://" + addr + "/live")
		if e != nil {
			t.Error(e)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	tr.CloseIdleConnections()
	cancel()
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if _, e := api.Accept(); e == nil {
		t.Fatal("API listener retained")
	}
	if _, e := ops.Accept(); e == nil {
		t.Fatal("operations listener retained")
	}
}
