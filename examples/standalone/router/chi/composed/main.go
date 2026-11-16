package main

import (
	"fmt"
	"github.com/skolldire/go-engine/router"
	adapter "github.com/skolldire/go-engine/router/chi"
	"net/http"
	"net/http/httptest"
)

func run() error {
	a, e := adapter.New(router.Config{AllowedHosts: []string{"example.com"}})
	if e != nil {
		return e
	}
	a.Native().Get("/native", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(201) })
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "http://example.com/native", nil))
	if w.Code != 201 {
		return fmt.Errorf("native route failed")
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		panic(e)
	}
	fmt.Println("chi consumer passed")
}
