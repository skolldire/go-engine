package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOpenLogFile_RestrictivePermissions is the regression test for the log file
// being created 0666 (world-writable) inside a 0755 directory.
func TestOpenLogFile_RestrictivePermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "logs")
	path := filepath.Join(dir, "app.log")

	svc := NewService(Config{Level: "info", Path: path}, nil)
	require.NotNil(t, svc)

	fi, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, logFilePerm, fi.Mode().Perm(),
		"log files must not be readable or writable by group/other")

	di, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, logDirPerm, di.Mode().Perm(),
		"log directory must not be world-accessible")
}

// TestNewService_HonoursConfiguredLevel guards the engine-side regression where
// the level from configuration was overwritten with "trace".
func TestNewService_HonoursConfiguredLevel(t *testing.T) {
	// logrus normalises "warn" to "warning" when reporting the level back.
	cases := map[string]string{"warn": "warning", "error": "error", "debug": "debug"}
	for configured, reported := range cases {
		t.Run(configured, func(t *testing.T) {
			svc := NewService(Config{Level: configured}, nil)
			assert.Equal(t, reported, svc.GetLogLevel())
		})
	}
}

// TestNewService_AppliesFormat guards that Format survives construction, which
// it did not when the engine rebuilt the config with only Level and Path.
func TestNewService_AppliesFormat(t *testing.T) {
	svc := NewService(Config{Level: "info", Format: "json"}, nil)
	require.NotNil(t, svc)

	impl, ok := svc.(*service)
	require.True(t, ok)
	assert.IsType(t, &logrus.JSONFormatter{}, impl.Log.Formatter)
}

// captureLogger builds a Service writing JSON into buf so assertions can be
// made on the actual emitted records.
func captureLogger(t *testing.T, level string, extractor ContextExtractor) (Service, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer
	svc := NewService(Config{
		Level:            level,
		Format:           "json",
		OutputWriters:    []io.Writer{&buf},
		ContextExtractor: extractor,
	}, nil)

	return svc, &buf
}

func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m), "line: %s", line)
		out = append(out, m)
	}
	return out
}

func TestService_EmitsAtEachLevel(t *testing.T) {
	svc, buf := captureLogger(t, "debug", nil)
	ctx := context.Background()

	svc.Debug(ctx, "debug msg", nil)
	svc.Info(ctx, "info msg", nil)
	svc.Warn(ctx, "warn msg", nil)
	svc.Error(ctx, errors.New("boom"), nil)

	lines := decodeLines(t, buf)
	require.Len(t, lines, 4)
	assert.Equal(t, "debug msg", lines[0]["message"])
	assert.Equal(t, "info msg", lines[1]["message"])
	assert.Equal(t, "warn msg", lines[2]["message"])
	assert.Equal(t, "boom", lines[3]["message"])
}

// TestService_NewServiceIsSilent is the regression test for the library
// emitting "Logger service initialized" on every construction.
func TestService_NewServiceIsSilent(t *testing.T) {
	_, buf := captureLogger(t, "debug", nil)
	assert.Empty(t, buf.String(), "constructing a logger must not emit a log line")
}

func TestService_LevelFiltersLowerSeverity(t *testing.T) {
	svc, buf := captureLogger(t, "warn", nil)
	ctx := context.Background()

	svc.Debug(ctx, "dropped", nil)
	svc.Info(ctx, "dropped", nil)
	svc.Warn(ctx, "kept", nil)

	lines := decodeLines(t, buf)
	require.Len(t, lines, 1)
	assert.Equal(t, "kept", lines[0]["message"])
}

func TestService_WithFieldAndWithFields(t *testing.T) {
	svc, buf := captureLogger(t, "info", nil)

	svc.WithField("component", "router").
		WithFields(map[string]any{"attempt": 2}).
		Info(context.Background(), "hello", map[string]any{"extra": "value"})

	lines := decodeLines(t, buf)
	require.Len(t, lines, 1)
	assert.Equal(t, "router", lines[0]["component"])
	assert.Equal(t, float64(2), lines[0]["attempt"])
	assert.Equal(t, "value", lines[0]["extra"])
}

func TestService_WithFieldsDoesNotMutateParent(t *testing.T) {
	svc, buf := captureLogger(t, "info", nil)

	child := svc.WithField("only_on", "child")
	svc.Info(context.Background(), "parent", nil)
	child.Info(context.Background(), "child", nil)

	lines := decodeLines(t, buf)
	require.Len(t, lines, 2)
	assert.NotContains(t, lines[0], "only_on", "the parent logger must not inherit child fields")
	assert.Equal(t, "child", lines[1]["only_on"])
}

func TestService_WithFieldsEmptyReturnsSameLogger(t *testing.T) {
	svc, _ := captureLogger(t, "info", nil)
	assert.Same(t, svc, svc.WithFields(nil))
	assert.Same(t, svc, svc.WithFields(map[string]any{}))
}

func TestService_ContextExtractorAddsFields(t *testing.T) {
	extractor := func(ctx context.Context) map[string]any {
		if v := ctx.Value(ctxKeyTrace{}); v != nil {
			return map[string]any{"trace_id": v}
		}
		return nil
	}

	svc, buf := captureLogger(t, "info", extractor)
	ctx := context.WithValue(context.Background(), ctxKeyTrace{}, "abc-123")

	svc.Info(ctx, "with trace", nil)

	lines := decodeLines(t, buf)
	require.Len(t, lines, 1)
	assert.Equal(t, "abc-123", lines[0]["trace_id"])
}

type ctxKeyTrace struct{}

func TestService_ErrorWithNilError(t *testing.T) {
	svc, buf := captureLogger(t, "info", nil)
	svc.Error(context.Background(), nil, nil)

	lines := decodeLines(t, buf)
	require.Len(t, lines, 1)
	assert.Equal(t, "Unknown error", lines[0]["message"])
}

func TestService_SetLogLevel(t *testing.T) {
	svc, _ := captureLogger(t, "info", nil)

	require.NoError(t, svc.SetLogLevel("error"))
	assert.Equal(t, "error", svc.GetLogLevel())

	err := svc.SetLogLevel("not-a-level")
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid log level")
	assert.Equal(t, "error", svc.GetLogLevel(), "an invalid level must not change the current one")
}

func TestService_WrapError(t *testing.T) {
	svc, _ := captureLogger(t, "info", nil)

	cause := errors.New("cause")
	wrapped := svc.WrapError(cause, "while doing X")
	require.Error(t, wrapped)
	assert.ErrorIs(t, wrapped, cause, "wrapping must preserve the cause")
	assert.Contains(t, wrapped.Error(), "while doing X")

	fromNil := svc.WrapError(nil, "message only")
	require.Error(t, fromNil)
	assert.Equal(t, "message only", fromNil.Error())
}

func TestService_MultipleOutputWriters(t *testing.T) {
	var a, b bytes.Buffer
	svc := NewService(Config{
		Level:         "info",
		Format:        "json",
		OutputWriters: []io.Writer{&a, &b},
	}, nil)

	svc.Info(context.Background(), "fan out", nil)

	assert.Contains(t, a.String(), "fan out")
	assert.Contains(t, b.String(), "fan out")
}

func TestService_FatalErrorUsesExitFunc(t *testing.T) {
	var buf bytes.Buffer
	exitCode := -1

	svc := NewService(Config{
		Level:         "info",
		Format:        "json",
		OutputWriters: []io.Writer{&buf},
		ExitFunc:      func(code int) { exitCode = code },
	}, nil)

	svc.FatalError(context.Background(), errors.New("fatal boom"), nil)

	assert.Equal(t, 1, exitCode, "FatalError must go through the configured ExitFunc")
	assert.Contains(t, buf.String(), "fatal boom")
}
