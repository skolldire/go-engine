package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedOp struct {
	inv      Invocation
	duration time.Duration
	err      error
}

type fakeRecorder struct{ ops []recordedOp }

func (r *fakeRecorder) RecordOperation(_ context.Context, inv Invocation, d time.Duration, err error) {
	r.ops = append(r.ops, recordedOp{inv: inv, duration: d, err: err})
}

type fakeTracer struct{ spans []string }

func (t *fakeTracer) Span(ctx context.Context, name string, fn func(context.Context) error) error {
	t.spans = append(t.spans, name)
	return fn(ctx)
}

func okOp(context.Context) (any, error)  { return "value", nil }
func badOp(context.Context) (any, error) { return nil, errors.New("boom") }

// TestChain_AppliesOutermostFirst pins the composition contract, because the
// order decides whether one logical call produces one log line or one per retry.
func TestChain_AppliesOutermostFirst(t *testing.T) {
	var order []string

	record := func(name string) Middleware {
		return func(next Handler) Handler {
			return func(ctx context.Context, inv Invocation, op Operation) (any, error) {
				order = append(order, "enter:"+name)
				res, err := next(ctx, inv, op)
				order = append(order, "exit:"+name)
				return res, err
			}
		}
	}

	h := Chain(record("first"), record("second"))(baseHandler)
	_, err := h(context.Background(), Invocation{}, okOp)

	require.NoError(t, err)
	assert.Equal(t, []string{"enter:first", "enter:second", "exit:second", "exit:first"}, order)
}

func TestChain_EmptyAndNilAreNoops(t *testing.T) {
	h := Chain()(baseHandler)
	res, err := h(context.Background(), Invocation{}, okOp)
	require.NoError(t, err)
	assert.Equal(t, "value", res)

	h = Chain(nil, nil)(baseHandler)
	res, err = h(context.Background(), Invocation{}, okOp)
	require.NoError(t, err)
	assert.Equal(t, "value", res)
}

func TestWithMetrics_RecordsEveryOutcome(t *testing.T) {
	rec := &fakeRecorder{}
	h := Chain(WithMetrics(rec))(baseHandler)
	inv := Invocation{Client: "REST", Operation: "GET /users/{id}"}

	_, err := h(context.Background(), inv, okOp)
	require.NoError(t, err)
	_, err = h(context.Background(), inv, badOp)
	require.Error(t, err)

	require.Len(t, rec.ops, 2)
	assert.Equal(t, inv, rec.ops[0].inv)
	assert.NoError(t, rec.ops[0].err)
	assert.Error(t, rec.ops[1].err, "a failed operation must still be measured")
	assert.Equal(t, "GET /users/{id}", rec.ops[0].inv.Operation,
		"the operation template, not a resolved path, keeps metric cardinality bounded")
}

func TestWithMetrics_NilRecorderIsANoop(t *testing.T) {
	h := Chain(WithMetrics(nil))(baseHandler)
	res, err := h(context.Background(), Invocation{}, okOp)
	require.NoError(t, err)
	assert.Equal(t, "value", res)
}

func TestWithTracing_WrapsAndNamesTheSpan(t *testing.T) {
	tr := &fakeTracer{}
	h := Chain(WithTracing(tr))(baseHandler)

	res, err := h(context.Background(), Invocation{Client: "SQS", Operation: "SendMessage"}, okOp)

	require.NoError(t, err)
	assert.Equal(t, "value", res, "the result must survive the span wrapper")
	assert.Equal(t, []string{"SQS.SendMessage"}, tr.spans)
}

func TestWithTracing_PropagatesTheError(t *testing.T) {
	tr := &fakeTracer{}
	h := Chain(WithTracing(tr))(baseHandler)

	_, err := h(context.Background(), Invocation{Client: "SQS", Operation: "Send"}, badOp)
	assert.Error(t, err)
	assert.Len(t, tr.spans, 1)
}

func TestWithLogging_HonoursTheRuntimeFlag(t *testing.T) {
	enabled := false
	h := Chain(WithLogging(&mockLogger{}, func() bool { return enabled }))(baseHandler)

	res, err := h(context.Background(), Invocation{}, okOp)
	require.NoError(t, err)
	assert.Equal(t, "value", res)

	enabled = true
	res, err = h(context.Background(), Invocation{}, okOp)
	require.NoError(t, err)
	assert.Equal(t, "value", res)
}

func TestWithLogging_NilLoggerIsANoop(t *testing.T) {
	h := Chain(WithLogging(nil, nil))(baseHandler)
	res, err := h(context.Background(), Invocation{}, okOp)
	require.NoError(t, err)
	assert.Equal(t, "value", res)
}

// TestBaseClient_UseComposesConcerns is the point of the refactor: a client can
// take metrics and tracing without its package growing a single conditional.
func TestBaseClient_UseComposesConcerns(t *testing.T) {
	rec := &fakeRecorder{}
	tr := &fakeTracer{}

	bc := NewBaseClientWithMiddleware(
		BaseConfig{Timeout: time.Second},
		&mockLogger{},
		"Redis",
		WithMetrics(rec),
		WithTracing(tr),
	)

	res, err := bc.Execute(context.Background(), "GET", okOp)

	require.NoError(t, err)
	assert.Equal(t, "value", res)
	require.Len(t, rec.ops, 1)
	assert.Equal(t, "Redis", rec.ops[0].inv.Client)
	assert.Equal(t, []string{"Redis.GET"}, tr.spans)
}

// TestBaseClient_DefaultChainPreservesBehaviour guards the migration: clients
// built the old way must behave exactly as before.
func TestBaseClient_DefaultChainPreservesBehaviour(t *testing.T) {
	bc := NewBaseClientWithName(
		BaseConfig{EnableLogging: true, Timeout: time.Second},
		&mockLogger{},
		"REST",
	)

	res, err := bc.Execute(context.Background(), "GET /x", okOp)
	require.NoError(t, err)
	assert.Equal(t, "value", res)

	_, err = bc.Execute(context.Background(), "GET /x", badOp)
	assert.Error(t, err)
}

func TestBaseClient_UseIsSafeAfterConstruction(t *testing.T) {
	rec := &fakeRecorder{}
	bc := NewBaseClientWithName(BaseConfig{}, &mockLogger{}, "X")

	bc.Use(WithMetrics(rec))
	_, err := bc.Execute(context.Background(), "op", okOp)

	require.NoError(t, err)
	assert.Len(t, rec.ops, 1)
}
