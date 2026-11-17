// Command composed validates a request body in an HTTP handler and answers
// 422 through router/errors, listing the failing paths.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	routererrors "github.com/skolldire/go-engine/router/errors"
	"github.com/skolldire/go-engine/validation"
)

type signup struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=12"`
	Plan     string `json:"plan" validate:"oneof=free pro"`
}

var errBadJSON = errors.New("request body is not valid JSON")

// classify is the recipe: validation.Errors becomes a 422 whose detail is
// the list of paths and rules. validation knows nothing about HTTP, and
// the response cannot contain a value because the error never had one.
func classify(err error) routererrors.Problem {
	var failed validation.Errors
	switch {
	case errors.As(err, &failed):
		parts := make([]string, 0, len(failed))
		for _, fe := range failed {
			parts = append(parts, fe.Path+": "+fe.Rule)
		}
		return routererrors.Problem{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Title: "Unprocessable Entity", Details: strings.Join(parts, "; ")}
	case errors.Is(err, errBadJSON):
		return routererrors.Problem{Status: http.StatusBadRequest, Code: "bad_request", Title: "Bad Request"}
	}
	// ErrInvalidCall and anything else is a server-side mistake.
	return routererrors.Problem{Status: http.StatusInternalServerError, Code: "internal_error", Title: "Internal Server Error"}
}

func handler(v *validation.Validator, respond *routererrors.Responder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body signup
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			_ = respond.Write(r.Context(), w, errBadJSON)
			return
		}
		if err := v.Struct(r.Context(), &body); err != nil {
			_ = respond.Write(r.Context(), w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
}

func post(h http.Handler, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/signup", strings.NewReader(body)))
	return w
}

func run() error {
	v, err := validation.New(validation.DefaultConfig(), validation.Dependencies{})
	if err != nil {
		return err
	}
	respond, err := routererrors.NewResponder(routererrors.DefaultConfig(), routererrors.Dependencies{Classify: classify})
	if err != nil {
		return err
	}
	h := handler(v, respond)

	if w := post(h, `{"email":"ana@example.com","password":"correct-horse-battery","plan":"pro"}`); w.Code != http.StatusCreated {
		return fmt.Errorf("valid body answered %d", w.Code)
	}

	const secret = "hunter2"
	w := post(h, `{"email":"nope","password":"`+secret+`","plan":"gold"}`)
	if w.Code != http.StatusUnprocessableEntity {
		return fmt.Errorf("invalid body answered %d", w.Code)
	}
	var problem map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
		return err
	}
	text := w.Body.String()
	for _, path := range []string{"email: email", "password: min", "plan: oneof"} {
		if !strings.Contains(text, path) {
			return fmt.Errorf("response does not name %q: %s", path, text)
		}
	}
	if problem["code"] != "validation_failed" || strings.Contains(text, secret) || strings.Contains(text, "gold") {
		return fmt.Errorf("unsafe or wrong response: %s", text)
	}

	if w := post(h, `{"email":`); w.Code != http.StatusBadRequest {
		return fmt.Errorf("malformed JSON answered %d", w.Code)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("validation composed: handler validates the body and answers 422 with paths, no values passed")
}
