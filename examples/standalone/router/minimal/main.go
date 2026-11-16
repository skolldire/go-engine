package main

import (
	"fmt"
	"github.com/skolldire/go-engine/router"
	"github.com/skolldire/go-engine/router/middleware"
	"net/http"
	"net/http/httptest"
)

func run() error {
	m, e := router.New(router.Config{AllowedHosts: []string{"a.example.com", "b.example.com"}})
	if e != nil {
		return e
	}
	id, e := middleware.RequestID(middleware.RequestIDConfig{Generate: func() string { return "local-id" }})
	if e != nil {
		return e
	}
	if e = m.Use(id); e != nil {
		return e
	}
	for _, host := range []string{"a.example.com", "b.example.com"} {
		if e = m.Handle("GET", host, "/item/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, r.Host+":"+r.PathValue("id")) })); e != nil {
			return e
		}
	}
	if e = m.Freeze(); e != nil {
		return e
	}
	for _, host := range []string{"a.example.com", "b.example.com"} {
		w := httptest.NewRecorder()
		m.ServeHTTP(w, httptest.NewRequest("GET", "http://"+host+"/item/7", nil))
		if w.Body.String() != host+":7" || w.Header().Get("X-Request-ID") != "local-id" {
			return fmt.Errorf("routing failed")
		}
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		panic(e)
	}
	fmt.Println("router minimal passed")
}
