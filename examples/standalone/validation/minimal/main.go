// Command minimal uses validation alone: a rule of its own, and errors
// that name what failed without repeating what was sent.
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/skolldire/go-engine/validation"
)

type account struct {
	Email    string   `json:"email" validate:"required,email"`
	Password string   `json:"password" validate:"required,min=12"`
	Handle   string   `json:"handle" validate:"not_blank,max=20"`
	Tenant   string   `json:"tenant" validate:"same_tenant"`
	Contacts []person `json:"contacts" validate:"dive"`
}

type person struct {
	Name string `json:"name" validate:"required"`
}

type tenantKey struct{}

func run() error {
	// Rules are injected into the instance. There is no global validator
	// and no built-in "not_empty": a rule the application wants is one it
	// registers, here under its own name.
	v, err := validation.New(validation.DefaultConfig(), validation.Dependencies{Rules: map[string]validation.Rule{
		"not_blank": func(_ context.Context, value any, _ string) bool {
			text, ok := value.(string)
			return ok && strings.TrimSpace(text) != ""
		},
		// A rule receives the context given to Struct.
		"same_tenant": func(ctx context.Context, value any, _ string) bool {
			return value == ctx.Value(tenantKey{})
		},
	}})
	if err != nil {
		return err
	}
	// A name that is already a built-in rule is refused at construction.
	_, err = validation.New(validation.DefaultConfig(), validation.Dependencies{Rules: map[string]validation.Rule{
		"email": func(context.Context, any, string) bool { return true },
	}})
	if !errors.Is(err, validation.ErrInvalidRule) {
		return fmt.Errorf("built-in collision accepted: %w", err)
	}

	ctx := context.WithValue(context.Background(), tenantKey{}, "acme")
	good := account{Email: "ana@example.com", Password: "correct-horse-battery", Handle: "ana", Tenant: "acme", Contacts: []person{{Name: "Leo"}}}
	if err := v.Struct(ctx, &good); err != nil {
		return fmt.Errorf("valid account rejected: %w", err)
	}

	const secret = "hunter2"
	bad := account{Email: "not-an-email", Password: secret, Handle: "   ", Tenant: "other", Contacts: []person{{Name: "Leo"}, {}}}
	err = v.Struct(ctx, &bad)
	var failed validation.Errors
	if !errors.As(err, &failed) {
		return fmt.Errorf("want validation.Errors, got %w", err)
	}
	want := validation.Errors{
		{Path: "email", Rule: "email"},
		{Path: "password", Rule: "min", Param: "12"},
		{Path: "handle", Rule: "not_blank"},
		{Path: "tenant", Rule: "same_tenant"},
		{Path: "contacts[1].name", Rule: "required"},
	}
	if len(failed) != len(want) {
		return fmt.Errorf("got %d failures: %w", len(failed), err)
	}
	for i := range want {
		if failed[i] != want[i] {
			return fmt.Errorf("failure %d is %+v", i, failed[i])
		}
	}
	// The password failed and is named; its value is nowhere in the error.
	if text := err.Error(); strings.Contains(text, secret) || strings.Contains(text, "not-an-email") {
		return errors.New("a field value reached the error text")
	}

	// A misuse of the API is a different error from a failed rule.
	if err := v.Struct(ctx, "not a struct"); !errors.Is(err, validation.ErrInvalidCall) {
		return fmt.Errorf("non-struct accepted: %w", err)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("validation minimal: injected rules, context in rules and errors without values passed")
}
