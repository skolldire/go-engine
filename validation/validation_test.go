package validation

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/go-playground/validator/v10"
)

type address struct {
	City string `json:"city" validate:"required"`
	Zip  string `json:"zip,omitempty" validate:"len=5"`
}

type line struct {
	SKU   string `json:"sku" validate:"required"`
	Email string `json:"email" validate:"omitempty,email"`
}

type signup struct {
	Name     string            `json:"name" validate:"required,min=3"`
	Password string            `json:"password" validate:"required,min=12"`
	Confirm  string            `json:"confirm" validate:"eqfield=Password"`
	Age      int               `json:"age" validate:"gte=18"`
	Address  address           `json:"address"`
	Billing  *address          `json:"billing" validate:"omitempty"`
	Items    []line            `json:"items" validate:"dive"`
	Labels   map[string]string `json:"labels" validate:"dive,keys,max=4,endkeys,required"`
	Hidden   string            `json:"-" validate:"required"`
	Bare     string            `validate:"required"`
	Options  string            `json:",omitempty" validate:"required"`
}

const secret = "hunter2-correct-horse"

func build(t testing.TB, mutate func(*Config), rules map[string]Rule) *Validator {
	t.Helper()
	cfg := DefaultConfig()
	if mutate != nil {
		mutate(&cfg)
	}
	v, err := New(cfg, Dependencies{Rules: rules})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func failures(t testing.TB, err error) Errors {
	t.Helper()
	var out Errors
	if !errors.As(err, &out) || len(out) == 0 {
		t.Fatalf("want Errors, got %T: %v", err, err)
	}
	return out
}

func TestConfig(t *testing.T) {
	if cfg := DefaultConfig(); cfg != (Config{FieldNames: "json", RequiredStruct: true}) || cfg.Validate() != nil {
		t.Fatalf("defaults changed: %+v", cfg)
	}
	for _, names := range []string{"", "JSON", "yaml", "Go", " json"} {
		cfg := Config{FieldNames: names}
		err := cfg.Validate()
		var ce *ConfigError
		if !errors.As(err, &ce) || ce.Field != "FieldNames" || !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("%q: %v", names, err)
		}
		if v, err := New(cfg, Dependencies{}); v != nil || !errors.Is(err, ErrInvalidConfig) {
			t.Fatal(v, err)
		}
	}
	for _, cfg := range []Config{{FieldNames: "go"}, {FieldNames: "json"}, {FieldNames: "go", RequiredStruct: true}} {
		if _, err := New(cfg, Dependencies{}); err != nil {
			t.Fatal(cfg, err)
		}
	}
}

func TestInvalidRules(t *testing.T) {
	yes := Rule(func(context.Context, any, string) bool { return true })
	cases := map[string]map[string]Rule{
		"empty name":           {"": yes},
		"space":                {"not empty": yes},
		"tag separator":        {"a,b": yes},
		"parameter separator":  {"a=b": yes},
		"or separator":         {"a|b": yes},
		"unicode":              {"válida": yes},
		"nil rule":             {"present": nil},
		"built-in required":    {"required": yes},
		"built-in email":       {"email": yes},
		"built-in with param":  {"min": yes},
		"built-in alias":       {"iscolor": yes},
		"reserved dive":        {"dive": yes},
		"reserved omitempty":   {"omitempty": yes},
		"reserved keys":        {"keys": yes},
		"reserved structonly":  {"structonly": yes},
		"one bad among others": {"fine": yes, "also_fine": yes, "uuid": yes},
	}
	for name, rules := range cases {
		v, err := New(DefaultConfig(), Dependencies{Rules: rules})
		if v != nil || !errors.Is(err, ErrInvalidRule) {
			t.Fatalf("%s: %v, %v", name, v, err)
		}
	}
	// Names that are free are accepted, and an empty or nil map is fine.
	for _, rules := range []map[string]Rule{nil, {}, {"not_empty": yes, "Tenant-ID": yes, "rule2": yes}} {
		if _, err := New(DefaultConfig(), Dependencies{Rules: rules}); err != nil {
			t.Fatal(err)
		}
	}
	// Go maps cannot hold a duplicate name, and two validators never share
	// rules: the same name may be registered in each.
	a := build(t, nil, map[string]Rule{"same": yes})
	b := build(t, nil, map[string]Rule{"same": func(context.Context, any, string) bool { return false }})
	type probe struct {
		F string `json:"f" validate:"same"`
	}
	if a.Struct(t.Context(), probe{F: "x"}) != nil || b.Struct(t.Context(), probe{F: "x"}) == nil {
		t.Fatal("validators share rules")
	}
}

// TestNoGlobalState reads the package source: no package-level variable may
// hold a validator, and nothing may hand out a shared instance.
func TestNoGlobalState(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "validation.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok != token.VAR {
				continue
			}
			for _, spec := range d.Specs {
				for _, name := range spec.(*ast.ValueSpec).Names {
					if !strings.HasPrefix(name.Name, "Err") {
						t.Fatalf("package-level variable %s", name.Name)
					}
				}
			}
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.IsExported() && d.Name.Name != "New" && d.Name.Name != "DefaultConfig" {
				t.Fatalf("unexpected package function %s", d.Name.Name)
			}
		}
	}
	// A rule registered on one validator is unknown to the next.
	type probe struct {
		F string `validate:"only_here"`
	}
	build(t, nil, map[string]Rule{"only_here": func(context.Context, any, string) bool { return true }})
	if err := build(t, nil, nil).Struct(t.Context(), probe{}); !errors.Is(err, ErrInvalidCall) {
		t.Fatal(err)
	}
}

func TestInvalidCalls(t *testing.T) {
	v := build(t, nil, nil)
	var none context.Context // the nil context is the case under test
	if err := v.Struct(none, signup{}); !errors.Is(err, ErrNilContext) {
		t.Fatal(err)
	}
	var typed *signup
	var iface any
	for _, value := range []any{nil, iface, typed, (**signup)(nil)} {
		if err := v.Struct(t.Context(), value); !errors.Is(err, ErrInvalidCall) {
			t.Fatalf("%T: %v", value, err)
		}
	}
	var zero *Validator
	if err := zero.Struct(t.Context(), signup{}); !errors.Is(err, ErrInvalidCall) {
		t.Fatal(err)
	}
	if err := (&Validator{}).Struct(t.Context(), signup{}); !errors.Is(err, ErrInvalidCall) {
		t.Fatal(err)
	}
}

func TestRequiredStruct(t *testing.T) {
	strict := build(t, nil, nil)
	loose := build(t, func(c *Config) { c.RequiredStruct = false }, nil)
	good := line{SKU: "a"}
	bad := line{Email: "nope"}
	nonStructs := []any{"text", 42, 1.5, true, []line{good}, [1]line{good}, map[string]line{"k": good}, []string{"a"}, func() {}, make(chan int)}
	for _, value := range nonStructs {
		if err := strict.Struct(t.Context(), value); !errors.Is(err, ErrInvalidCall) {
			t.Fatalf("strict accepted %T: %v", value, err)
		}
	}
	// Both accept a struct, a pointer to one and a pointer to that.
	ptr := &good
	for _, v := range []*Validator{strict, loose} {
		for _, value := range []any{good, ptr, &ptr} {
			if err := v.Struct(t.Context(), value); err != nil {
				t.Fatalf("%T: %v", value, err)
			}
		}
		if got := failures(t, v.Struct(t.Context(), &bad)); len(got) != 2 || got[0] != (FieldError{Path: "sku", Rule: "required"}) || got[1] != (FieldError{Path: "email", Rule: "email"}) {
			t.Fatal(got)
		}
	}
	// Without RequiredStruct a collection is validated element by element.
	for _, value := range []any{[]line{good, good}, [1]line{good}, map[string]line{"k": good}, &[]line{good}, []*line{&good}, []string{"plain"}, []line{}} {
		if err := loose.Struct(t.Context(), value); err != nil {
			t.Fatalf("%T: %v", value, err)
		}
	}
	got := failures(t, loose.Struct(t.Context(), []line{good, bad}))
	if len(got) != 2 || got[0] != (FieldError{Path: "[1].sku", Rule: "required"}) || got[1] != (FieldError{Path: "[1].email", Rule: "email"}) {
		t.Fatal(got)
	}
	if got := failures(t, loose.Struct(t.Context(), map[string]line{"k": bad})); got[0].Path != "[k].sku" {
		t.Fatal(got)
	}
	// What the library cannot validate on its own is still an invalid call.
	for _, value := range []any{"text", 42, true, func() {}} {
		if err := loose.Struct(t.Context(), value); !errors.Is(err, ErrInvalidCall) {
			t.Fatalf("loose accepted %T: %v", value, err)
		}
	}
}

func invalid() signup {
	return signup{
		Name: "ab", Password: secret, Confirm: "other-" + secret, Age: 17,
		Address: address{Zip: "123"}, Billing: &address{City: "x", Zip: "1"},
		Items:  []line{{SKU: "ok"}, {SKU: "ok", Email: secret}, {}},
		Labels: map[string]string{"toolong": "v", "ok": ""},
	}
}

func paths(e Errors) map[string]FieldError {
	out := map[string]FieldError{}
	for _, fe := range e {
		out[fe.Path] = fe
	}
	return out
}

func TestPaths(t *testing.T) {
	got := paths(failures(t, build(t, nil, nil).Struct(t.Context(), invalid())))
	want := map[string]FieldError{
		"name":            {Rule: "min", Param: "3"},
		"confirm":         {Rule: "eqfield", Param: "Password"},
		"age":             {Rule: "gte", Param: "18"},
		"address.city":    {Rule: "required"},
		"address.zip":     {Rule: "len", Param: "5"},
		"billing.zip":     {Rule: "len", Param: "5"},
		"items[1].email":  {Rule: "email"},
		"items[2].sku":    {Rule: "required"},
		"labels[toolong]": {Rule: "max", Param: "4"},
		"labels[ok]":      {Rule: "required"},
		// json:"-", no tag and an options-only tag fall back to the Go name.
		"Hidden":  {Rule: "required"},
		"Bare":    {Rule: "required"},
		"Options": {Rule: "required"},
	}
	check := func(got, want map[string]FieldError) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("got %d failures, want %d: %v", len(got), len(want), got)
		}
		for path, fe := range want {
			fe.Path = path
			if got[path] != fe {
				t.Fatalf("%s: got %+v, want %+v", path, got[path], fe)
			}
		}
	}
	check(got, want)

	got = paths(failures(t, build(t, func(c *Config) { c.FieldNames = "go" }, nil).Struct(t.Context(), invalid())))
	check(got, map[string]FieldError{
		"Name": {Rule: "min", Param: "3"}, "Confirm": {Rule: "eqfield", Param: "Password"}, "Age": {Rule: "gte", Param: "18"},
		"Address.City": {Rule: "required"}, "Address.Zip": {Rule: "len", Param: "5"}, "Billing.Zip": {Rule: "len", Param: "5"},
		"Items[1].Email": {Rule: "email"}, "Items[2].SKU": {Rule: "required"},
		"Labels[toolong]": {Rule: "max", Param: "4"}, "Labels[ok]": {Rule: "required"},
		"Hidden": {Rule: "required"}, "Bare": {Rule: "required"}, "Options": {Rule: "required"},
	})

	// Failures come in the order the library reports them: declaration
	// order, depth first.
	ordered := failures(t, build(t, nil, nil).Struct(t.Context(), struct {
		B string  `json:"b" validate:"required"`
		N address `json:"n"`
		A string  `json:"a" validate:"required"`
	}{}))
	if ordered.Error() != "validation failed: b (required); n.city (required); n.zip (len=5); a (required)" {
		t.Fatal(ordered)
	}
	// "required" on a nested struct value is applied, not ignored.
	nested := failures(t, build(t, nil, nil).Struct(t.Context(), struct {
		Inner struct{ X int } `json:"inner" validate:"required"`
	}{}))
	if len(nested) != 1 || nested[0] != (FieldError{Path: "inner", Rule: "required"}) {
		t.Fatal(nested)
	}
}

func TestNoValuesAnywhere(t *testing.T) {
	// FieldError cannot carry a value: it has exactly these string fields.
	typ := reflect.TypeFor[FieldError]()
	if typ.NumField() != 3 || typ.Field(0).Name != "Path" || typ.Field(1).Name != "Rule" || typ.Field(2).Name != "Param" {
		t.Fatalf("FieldError changed shape: %v", typ)
	}
	for i := range typ.NumField() {
		if typ.Field(i).Type.Kind() != reflect.String {
			t.Fatalf("%s is not a string", typ.Field(i).Name)
		}
	}
	for _, names := range []string{"json", "go"} {
		v := build(t, func(c *Config) { c.FieldNames = names }, nil)
		bad := invalid()
		bad.Password = "short-" + secret[:3] // fails min=12 itself
		bad.Confirm = secret
		err := v.Struct(t.Context(), &bad)
		got := failures(t, err)
		if _, ok := paths(got)[map[string]string{"json": "password", "go": "Password"}[names]]; !ok {
			t.Fatalf("the password failure is missing: %v", got)
		}
		for _, text := range []string{err.Error(), fmt.Sprintf("%v", err), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err), fmt.Sprintf("%s", got)} {
			for _, leak := range []string{secret, "hunter2", "short-", "toolong-value", "nope"} {
				if strings.Contains(text, leak) {
					t.Fatalf("value %q leaked into %q", leak, text)
				}
			}
		}
		// No library type is reachable from the error either.
		var libErrs validator.ValidationErrors
		var libErr validator.FieldError
		if errors.As(err, &libErrs) || errors.As(err, &libErr) || errors.Unwrap(err) != nil {
			t.Fatal("the library's error is reachable from Errors")
		}
	}
	// Error() is deterministic and names path and rule.
	e := Errors{{Path: "password", Rule: "min", Param: "12"}, {Path: "items[2].email", Rule: "email"}}
	if e.Error() != "validation failed: password (min=12); items[2].email (email)" {
		t.Fatal(e.Error())
	}
	if (Errors{}).Error() != "validation failed" {
		t.Fatal(Errors{}.Error())
	}
}

func TestMalformedTags(t *testing.T) {
	v := build(t, nil, nil)
	type misspelled struct {
		Password string `json:"password" validate:"reqired"`
	}
	type badParam struct {
		Password string `json:"password" validate:"min=abc"`
	}
	type wrongKind struct {
		Password string `json:"password" validate:"dive"`
	}
	type empty struct {
		Password string `json:"password" validate:"required,,min=3"`
	}
	for _, value := range []any{misspelled{Password: secret}, badParam{Password: secret}, wrongKind{Password: secret}, empty{Password: secret}} {
		err := v.Struct(t.Context(), value)
		var failed Errors
		if !errors.Is(err, ErrInvalidCall) || errors.As(err, &failed) {
			t.Fatalf("%T: %v", value, err)
		}
		if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "hunter2") {
			t.Fatalf("%T: value leaked: %v", value, err)
		}
	}
	// The common mistake still says what to fix.
	if err := v.Struct(t.Context(), misspelled{}); !strings.Contains(err.Error(), "reqired") || !strings.Contains(err.Error(), "Password") {
		t.Fatal(err)
	}
	// The validator is usable after a usage error.
	if err := v.Struct(t.Context(), line{SKU: "a"}); err != nil {
		t.Fatal(err)
	}
}

type key struct{}

func TestRulesReceiveContextAndParam(t *testing.T) {
	type seen struct {
		tenant any
		value  any
		param  string
	}
	var mu sync.Mutex
	var calls []seen
	rules := map[string]Rule{
		"tenant": func(ctx context.Context, value any, param string) bool {
			mu.Lock()
			calls = append(calls, seen{ctx.Value(key{}), value, param})
			mu.Unlock()
			return ctx.Value(key{}) == value
		},
		"not_empty": func(_ context.Context, value any, _ string) bool {
			s, ok := value.(string)
			return ok && strings.TrimSpace(s) != ""
		},
		"multiple_of": func(_ context.Context, value any, param string) bool {
			n, ok := value.(int)
			return ok && param == "5" && n%5 == 0
		},
	}
	type request struct {
		Tenant string `json:"tenant" validate:"tenant=strict"`
		Note   string `json:"note" validate:"not_empty"`
		Qty    int    `json:"qty" validate:"multiple_of=5"`
	}
	v := build(t, nil, rules)
	ctx := context.WithValue(t.Context(), key{}, "acme")
	if err := v.Struct(ctx, request{Tenant: "acme", Note: "x", Qty: 10}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != (seen{"acme", "acme", "strict"}) {
		t.Fatalf("rule saw %+v", calls)
	}
	got := failures(t, v.Struct(ctx, request{Tenant: "other", Note: "   ", Qty: 7}))
	want := Errors{{Path: "tenant", Rule: "tenant", Param: "strict"}, {Path: "note", Rule: "not_empty"}, {Path: "qty", Rule: "multiple_of", Param: "5"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	// The rule sees the caller's context itself, cancellation included.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	var rulectx context.Context
	probe := build(t, nil, map[string]Rule{"probe": func(ctx context.Context, _ any, _ string) bool { rulectx = ctx; return true }})
	if err := probe.Struct(cancelled, struct {
		F string `validate:"probe"`
	}{F: "x"}); err != nil || rulectx != cancelled {
		t.Fatal(err, rulectx == cancelled)
	}
}

func TestRulePanicsPropagate(t *testing.T) {
	boom := errors.New("rule exploded")
	v := build(t, nil, map[string]Rule{"explode": func(context.Context, any, string) bool { panic(boom) }})
	type request struct {
		F string `validate:"explode"`
	}
	func() {
		defer func() {
			if r := recover(); r != boom {
				t.Fatalf("recovered %v, want the rule's own panic value", r)
			}
		}()
		_ = v.Struct(t.Context(), request{F: "x"})
		t.Fatal("the rule's panic was swallowed")
	}()
	// A later call on the same validator is unaffected.
	if err := v.Struct(t.Context(), line{SKU: "a"}); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentUse(t *testing.T) {
	v := build(t, nil, map[string]Rule{"not_empty": func(_ context.Context, value any, _ string) bool { return strings.TrimSpace(value.(string)) != "" }})
	type note struct {
		Text string `json:"text" validate:"not_empty"`
	}
	var wg sync.WaitGroup
	for worker := range 32 {
		wg.Go(func() {
			for i := range 200 {
				switch (worker + i) % 4 {
				case 0:
					if err := v.Struct(t.Context(), line{SKU: "a"}); err != nil {
						t.Error(err)
					}
				case 1:
					var got Errors
					if err := v.Struct(t.Context(), invalid()); !errors.As(err, &got) || len(got) != 13 {
						t.Error(err)
					}
				case 2:
					var got Errors
					if err := v.Struct(t.Context(), note{Text: " "}); !errors.As(err, &got) || got[0].Path != "text" {
						t.Error(err)
					}
				default:
					if err := v.Struct(t.Context(), "not a struct"); !errors.Is(err, ErrInvalidCall) {
						t.Error(err)
					}
				}
			}
		})
	}
	wg.Wait()
}

// TestReuseDoesNotGrow validates the same fixed types many times. The
// library caches per type, so the cost per call must settle rather than
// climb with the number of calls.
func TestReuseDoesNotGrow(t *testing.T) {
	v := build(t, nil, nil)
	good, bad := line{SKU: "a"}, invalid()
	run := func() {
		_ = v.Struct(context.Background(), good)
		_ = v.Struct(context.Background(), &bad)
	}
	for range 100 {
		run()
	}
	early := testing.AllocsPerRun(200, run)
	for range 5000 {
		run()
	}
	late := testing.AllocsPerRun(200, run)
	// The count is not exact: map iteration order and pooled buffers move
	// it by an allocation or two between runs. Growth with reuse would be
	// proportional to the 5000 calls in between, far beyond this margin.
	if late > early+4 {
		t.Fatalf("allocations per call grew with reuse: %v -> %v", early, late)
	}
}

// FuzzStruct feeds arbitrary values to tagged structs. Whatever the input,
// the result is nil or Errors, and no field value appears in the error.
func FuzzStruct(f *testing.F) {
	f.Add("ana", "a@b.co", "correct-horse-battery", 30, "12345", "k")
	f.Add("", "", "", 0, "", "")
	f.Add("ab", "not-an-email", secret, -1, "1", "toolong")
	v, err := New(DefaultConfig(), Dependencies{Rules: map[string]Rule{"not_empty": func(_ context.Context, value any, _ string) bool { return strings.TrimSpace(value.(string)) != "" }}})
	if err != nil {
		f.Fatal(err)
	}
	type target struct {
		Name     string            `json:"name" validate:"required,min=3,max=40"`
		Email    string            `json:"email" validate:"omitempty,email"`
		Password string            `json:"password" validate:"not_empty,min=12"`
		Age      int               `json:"age" validate:"gte=0,lte=150"`
		Address  address           `json:"address"`
		Items    []line            `json:"items" validate:"dive"`
		Labels   map[string]string `json:"labels" validate:"dive,keys,max=4,endkeys,required"`
	}
	f.Fuzz(func(t *testing.T, name, email, password string, age int, zip, label string) {
		if len(name)+len(email)+len(password)+len(zip)+len(label) > 512 {
			return
		}
		value := target{Name: name, Email: email, Password: password, Age: age, Address: address{City: name, Zip: zip},
			Items: []line{{SKU: label, Email: email}}, Labels: map[string]string{label: password}}
		err := v.Struct(context.Background(), &value)
		if err == nil {
			return
		}
		var failed Errors
		if !errors.As(err, &failed) || len(failed) == 0 {
			t.Fatalf("unclassified error %T: %v", err, err)
		}
		for _, fe := range failed {
			if fe.Path == "" || fe.Rule == "" {
				t.Fatalf("incomplete failure %+v", fe)
			}
		}
		// Map keys are part of the path by design; every other input is a
		// value and must stay out. Short inputs are skipped because they
		// collide with rule and field names by chance.
		text := err.Error()
		for _, input := range []string{password, email, zip, name} {
			if len(input) >= 8 && !strings.Contains(label, input) && strings.Contains(text, input) {
				t.Fatalf("value %q leaked into %q", input, text)
			}
		}
	})
}

// The pair below compares one validation of the same struct through this
// package and through the library used directly.
func BenchmarkStruct(b *testing.B) {
	v := build(b, nil, nil)
	value := signupOK()
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if err := v.Struct(ctx, &value); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLibraryDirect(b *testing.B) {
	lib := validator.New(validator.WithRequiredStructEnabled())
	lib.RegisterTagNameFunc(jsonName)
	value := signupOK()
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if err := lib.StructCtx(ctx, &value); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStructFailing(b *testing.B) {
	v := build(b, nil, nil)
	value := invalid()
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if err := v.Struct(ctx, &value); err == nil {
			b.Fatal("expected failures")
		}
	}
}

func signupOK() signup {
	return signup{Name: "ana", Password: secret, Confirm: secret, Age: 30, Address: address{City: "x", Zip: "12345"},
		Items: []line{{SKU: "a", Email: "a@b.co"}}, Labels: map[string]string{"k": "v"}, Hidden: "h", Bare: "b", Options: "o"}
}
