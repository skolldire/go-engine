package logrusadapter

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSilentLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(devNull{})
	l.SetLevel(logrus.DebugLevel)
	return l
}

// devNull discards all log output in tests.
type devNull struct{}

func (devNull) Write(p []byte) (int, error) { return len(p), nil }

func TestNew_ImplementsLogWriter(t *testing.T) {
	var _ logger.LogWriter = New(logrus.New())
}

func TestAdapter_LogMethods_DoNotPanic(t *testing.T) {
	w := New(newSilentLogger())

	assert.NotPanics(t, func() { w.Info("info") })
	assert.NotPanics(t, func() { w.Warn("warn") })
	assert.NotPanics(t, func() { w.Error("error") })
	assert.NotPanics(t, func() { w.Debug("debug") })
	assert.NotPanics(t, func() { w.Infof("infof %s", "x") })
	assert.NotPanics(t, func() { w.Warnf("warnf %s", "x") })
	assert.NotPanics(t, func() { w.Errorf("errorf %s", "x") })
	assert.NotPanics(t, func() { w.Debugf("debugf %s", "x") })
}

func TestAdapter_WithField_ReturnsLogWriter(t *testing.T) {
	w := New(newSilentLogger())
	w2 := w.WithField("key", "value")

	require.NotNil(t, w2)
	var _ logger.LogWriter = w2
}

func TestAdapter_WithFields_ReturnsLogWriter(t *testing.T) {
	w := New(newSilentLogger())
	w2 := w.WithFields(logrus.Fields{"a": 1, "b": "two"})

	require.NotNil(t, w2)
	var _ logger.LogWriter = w2
}

func TestEntryAdapter_Chaining(t *testing.T) {
	w := New(newSilentLogger())

	// WithField → WithField → WithFields should all return logger.LogWriter.
	chained := w.WithField("k1", "v1").
		WithField("k2", "v2").
		WithFields(logrus.Fields{"k3": "v3"})

	require.NotNil(t, chained)
	assert.NotPanics(t, func() { chained.Info("chained log") })
}

func TestEntryAdapter_LogMethods_DoNotPanic(t *testing.T) {
	entry := New(newSilentLogger()).WithField("component", "test")

	assert.NotPanics(t, func() { entry.Info("info") })
	assert.NotPanics(t, func() { entry.Warn("warn") })
	assert.NotPanics(t, func() { entry.Error("error") })
	assert.NotPanics(t, func() { entry.Debug("debug") })
	assert.NotPanics(t, func() { entry.Infof("infof %d", 1) })
	assert.NotPanics(t, func() { entry.Warnf("warnf %d", 1) })
	assert.NotPanics(t, func() { entry.Errorf("errorf %d", 1) })
	assert.NotPanics(t, func() { entry.Debugf("debugf %d", 1) })
}
