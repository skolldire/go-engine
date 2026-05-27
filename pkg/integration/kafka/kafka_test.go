package kafka

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	kgo "github.com/segmentio/kafka-go"
	"github.com/skolldire/go-engine/pkg/health"
	"github.com/skolldire/go-engine/pkg/testutil"
	"github.com/stretchr/testify/assert"
)

// ── mock reader ──────────────────────────────────────────────────────────────

// mockReader returns preset messages in order, then blocks until ctx is done.
type mockReader struct {
	msgs []kgo.Message
	pos  int
}

func newMockReader(msgs []kgo.Message) *mockReader {
	return &mockReader{msgs: msgs}
}

func (r *mockReader) FetchMessage(ctx context.Context) (kgo.Message, error) {
	if r.pos < len(r.msgs) {
		m := r.msgs[r.pos]
		r.pos++
		return m, nil
	}
	<-ctx.Done()
	return kgo.Message{}, ctx.Err()
}

func (r *mockReader) CommitMessages(_ context.Context, _ ...kgo.Message) error { return nil }
func (r *mockReader) Close() error                                              { return nil }

// ── mock writer (DLQ) ────────────────────────────────────────────────────────

type mockWriter struct {
	count atomic.Int32
	msgs  []kgo.Message
}

func (w *mockWriter) WriteMessages(_ context.Context, msgs ...kgo.Message) error {
	w.count.Add(1)
	w.msgs = append(w.msgs, msgs...)
	return nil
}
func (w *mockWriter) Close() error { return nil }

// ── helpers ───────────────────────────────────────────────────────────────────

func testConfig() Config {
	return Config{
		Brokers:      []string{"localhost:19092"},
		Topic:        "test-topic",
		MaxRetries:   3,
		RetryBackoff: 0, // instant in tests
	}
}

func newTestConsumer(r readerIface, dlq writerIface, cfg Config) *consumer {
	return &consumer{
		reader:    r,
		dlqWriter: dlq,
		cfg:       cfg,
		log:       &testutil.MockLogger{},
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestNewProducer(t *testing.T) {
	p := NewProducer(testConfig(), &testutil.MockLogger{})
	assert.NotNil(t, p)
	// kafka.Writer connects lazily; Close is safe to call without a broker.
	assert.NoError(t, p.Close())
}

func TestNewConsumer_WithoutDLQ(t *testing.T) {
	c := NewConsumer(testConfig(), &testutil.MockLogger{})
	assert.NotNil(t, c)
	assert.NoError(t, c.Close())
}

func TestNewConsumer_WithDLQ(t *testing.T) {
	cfg := testConfig()
	cfg.DLQTopic = "test-dlq"
	c := NewConsumer(cfg, &testutil.MockLogger{})
	assert.NotNil(t, c)
	assert.NoError(t, c.Close())
}

func TestNewConsumer_DefaultsApplied(t *testing.T) {
	cfg := Config{Brokers: []string{"localhost:19092"}, Topic: "t"}
	c := NewConsumer(cfg, &testutil.MockLogger{}).(*consumer)
	assert.Equal(t, defaultMaxRetries, c.cfg.MaxRetries)
	assert.Equal(t, defaultRetryBackoff, c.cfg.RetryBackoff)
	assert.NoError(t, c.Close())
}

func TestNewChecker_Check_NoConnection(t *testing.T) {
	checker := NewChecker([]string{"localhost:19092"})
	checker.timeout = 300 * time.Millisecond
	err := checker.Check(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no brokers reachable")
}

func TestKafkaChecker_SatisfiesInterface(t *testing.T) {
	var _ health.Checker = (*KafkaChecker)(nil)
}

func TestConsumer_Subscribe_CancelContext(t *testing.T) {
	r := newMockReader(nil) // no messages → blocks immediately on ctx.Done()
	c := newTestConsumer(r, nil, testConfig())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- c.Subscribe(ctx, func(_ context.Context, _ Message) error { return nil }) }()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe did not return after context cancellation")
	}
}

func TestConsumer_RetryLogic(t *testing.T) {
	msg := kgo.Message{Topic: "test-topic", Value: []byte("payload"), Offset: 7}
	r := newMockReader([]kgo.Message{msg})
	dlq := &mockWriter{}
	cfg := testConfig() // MaxRetries = 3, RetryBackoff = 0

	c := newTestConsumer(r, dlq, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var handlerCalls atomic.Int32
	errAlways := errors.New("handler error")

	handler := func(_ context.Context, _ Message) error {
		handlerCalls.Add(1)
		return errAlways
	}

	done := make(chan error, 1)
	go func() { done <- c.Subscribe(ctx, handler) }()

	// After the single message is fully processed (retries + DLQ), Subscribe
	// will block on the next FetchMessage waiting for ctx. Poll until the DLQ
	// write happens, then cancel so Subscribe can exit cleanly.
	assert.Eventually(t, func() bool {
		return dlq.count.Load() == 1
	}, 2*time.Second, 5*time.Millisecond, "DLQ was not written within timeout")

	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err, "Subscribe should return nil on context cancel")
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe did not return after context cancellation")
	}

	// MaxRetries = 3 → loop: attempt 0, 1, 2, 3 → 4 total handler calls
	assert.Equal(t, int32(cfg.MaxRetries+1), handlerCalls.Load())
	assert.Equal(t, int32(1), dlq.count.Load())
	if assert.Len(t, dlq.msgs, 1) {
		assert.Equal(t, msg.Value, dlq.msgs[0].Value)
	}
}
