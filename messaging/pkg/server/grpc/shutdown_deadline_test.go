package grpc

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// rawCodec moves opaque bytes so a test RPC needs no generated protobuf code.
type rawCodec struct{}

func (rawCodec) Marshal(v any) ([]byte, error)   { return *(v.(*[]byte)), nil }
func (rawCodec) Unmarshal(d []byte, v any) error { *(v.(*[]byte)) = d; return nil }
func (rawCodec) Name() string                    { return "raw" }

// blockedServer returns a started server holding one in-flight RPC that never
// returns until release is closed.
//
// A stuck handler is the only condition under which the shutdown deadlines
// matter: GracefulStop waits for in-flight RPCs with no bound of its own, so
// without one every Stop returns immediately and proves nothing.
func blockedServer(t *testing.T) (s *server, release chan struct{}) {
	t.Helper()

	release = make(chan struct{})
	entered := make(chan struct{})

	gs := grpc.NewServer(grpc.UnknownServiceHandler(
		func(_ any, stream grpc.ServerStream) error {
			var in []byte
			_ = stream.RecvMsg(&in)
			close(entered)
			<-release
			return nil
		}))

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s = &server{
		server:          gs,
		logger:          silentLogger{},
		shutdownTimeout: time.Minute,
	}
	s.addr = lis.Addr().String()

	go func() { _ = gs.Serve(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	go func() {
		in, out := []byte("ping"), []byte(nil)
		_ = conn.Invoke(context.Background(), "/test.Svc/Block", &in, &out,
			grpc.ForceCodec(rawCodec{}))
	}()

	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the RPC never reached the handler; the test would prove nothing")
	}

	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		_ = conn.Close()
		gs.Stop()
	})

	return s, release
}

// TestStop_EachCallerIsBoundedByItsOwnDeadline is the regression test for the
// sync.Once shutdown.
//
// Do blocks every later caller until the first returns, so a caller asking for
// a 150ms shutdown waited out another caller's five-second drain with its own
// context ignored — the deadline it passed had no effect whatsoever. What is
// wanted is one drain observed by many callers, each bounded by its own context.
func TestStop_EachCallerIsBoundedByItsOwnDeadline(t *testing.T) {
	s, release := blockedServer(t)

	type outcome struct {
		err     error
		elapsed time.Duration
	}

	patient := make(chan outcome, 1)
	impatient := make(chan outcome, 1)

	// The patient caller starts the drain and blocks: the RPC is stuck.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		start := time.Now()
		err := s.Stop(ctx)
		patient <- outcome{err, time.Since(start)}
	}()

	// Give the drain time to be under way, so this is genuinely a second caller
	// arriving mid-drain rather than a race for who goes first.
	time.Sleep(300 * time.Millisecond)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		start := time.Now()
		err := s.Stop(ctx)
		impatient <- outcome{err, time.Since(start)}
	}()

	select {
	case got := <-impatient:
		assert.ErrorIs(t, got.err, ErrForcedShutdown,
			"the caller whose deadline expired must be told the shutdown was forced")
		assert.ErrorIs(t, got.err, context.DeadlineExceeded)
		assert.Less(t, got.elapsed, 2*time.Second,
			"a 150ms deadline must be honoured, not deferred to another caller's drain")
	case <-time.After(4 * time.Second):
		t.Fatal("the short-deadline caller blocked on the long-deadline caller's drain")
	}

	// Independence runs both ways. The handler ignores its context, and gRPC
	// cannot abort such a handler, so the patient caller is still legitimately
	// waiting — it must not have been dragged to an early return by the other
	// caller's shorter deadline either.
	select {
	case got := <-patient:
		t.Fatalf("the patient caller returned early (%v after %s) because another "+
			"caller had a shorter deadline", got.err, got.elapsed)
	default:
	}

	// Once the handler finally returns, the drain completes and the patient
	// caller observes it — well inside its own five-second window.
	close(release)

	select {
	case got := <-patient:
		assert.NoError(t, got.err,
			"the patient caller observed a clean drain, so it reports success")
		assert.Less(t, got.elapsed, 5*time.Second)
	case <-time.After(4 * time.Second):
		t.Fatal("the patient caller never observed the drain finishing")
	}
}

// TestStop_CallerArrivingAfterAForcedCloseIsStillBounded covers the reverse
// arrival order: the forcing caller starts the drain itself, and a later caller
// must be bounded by its own deadline rather than by the stuck handler.
func TestStop_CallerArrivingAfterAForcedCloseIsStillBounded(t *testing.T) {
	s, _ := blockedServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	require.ErrorIs(t, s.Stop(ctx), ErrForcedShutdown)
	assert.Less(t, time.Since(start), 2*time.Second)

	later, laterCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer laterCancel()

	done := make(chan error, 1)
	go func() { done <- s.Stop(later) }()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, ErrForcedShutdown)
	case <-time.After(4 * time.Second):
		t.Fatal("a caller arriving after the forced close hung past its own deadline")
	}
}

// TestStop_ForcesOnlyOnce guards that concurrent expiring callers do not each
// call the underlying Stop.
func TestStop_ForcesOnlyOnce(t *testing.T) {
	s, _ := blockedServer(t)

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			errs[i] = s.Stop(ctx)
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			assert.ErrorIs(t, err, ErrForcedShutdown)
		}
	}
}

// TestStop_ForcedCloseReleasesAContextAwareHandler is the counterpart to the
// stuck-handler tests: forcing is best effort against a handler that ignores
// its context, but it must genuinely cancel one that honours it.
func TestStop_ForcedCloseReleasesAContextAwareHandler(t *testing.T) {
	returned := make(chan struct{})
	entered := make(chan struct{})

	gs := grpc.NewServer(grpc.UnknownServiceHandler(
		func(_ any, stream grpc.ServerStream) error {
			var in []byte
			_ = stream.RecvMsg(&in)
			close(entered)
			<-stream.Context().Done()
			close(returned)
			return nil
		}))

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = gs.Serve(lis) }()

	s := &server{server: gs, logger: silentLogger{}, shutdownTimeout: time.Minute}
	s.addr = lis.Addr().String()

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	go func() {
		in, out := []byte("ping"), []byte(nil)
		_ = conn.Invoke(context.Background(), "/test.Svc/Block", &in, &out,
			grpc.ForceCodec(rawCodec{}))
	}()

	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the RPC never reached the handler; the test would prove nothing")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_ = s.Stop(ctx)

	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("forcing the close did not cancel a context-aware handler")
	}
}

// TestStart_AfterStopIsRefused pins the restart contract.
//
// A *grpc.Server is single-use. Before this check Start bound a listener and
// returned nil while Serve returned ErrServerStopped immediately: the caller
// held a server that accepted nothing and a listener nobody would ever close.
func TestStart_AfterStopIsRefused(t *testing.T) {
	s, release := blockedServer(t)
	close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, s.Stop(ctx))

	err := s.Start(context.Background())
	require.ErrorIs(t, err, ErrServerStopped,
		"restarting a stopped server must fail loudly, not yield a deaf server")
}

// TestStart_AfterStopBindsNoListener proves the refusal happens before the
// listener is created: a rejected Start must leave the port free.
func TestStart_AfterStopBindsNoListener(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := lis.Addr().(*net.TCPAddr).Port
	require.NoError(t, lis.Close())

	s := &server{
		server:          grpc.NewServer(),
		puerto:          port,
		logger:          silentLogger{},
		shutdownTimeout: time.Second,
	}

	require.NoError(t, s.Stop(context.Background()))
	require.ErrorIs(t, s.Start(context.Background()), ErrServerStopped)

	// The port must still be bindable: a refused Start that leaked a listener
	// would make this fail with "address already in use".
	again, err := net.Listen("tcp", lis.Addr().String())
	require.NoError(t, err, "the refused Start leaked a listener")
	require.NoError(t, again.Close())
}
