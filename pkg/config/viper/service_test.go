package viper

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
)

// noopLogWriter is a minimal logger.LogWriter for tests.
type noopLogWriter struct{}

func (noopLogWriter) Info(...interface{})                              {}
func (noopLogWriter) Warn(...interface{})                              {}
func (noopLogWriter) Error(...interface{})                             {}
func (noopLogWriter) Debug(...interface{})                             {}
func (noopLogWriter) Fatal(...interface{})                             {}
func (noopLogWriter) Infof(string, ...interface{})                     {}
func (noopLogWriter) Warnf(string, ...interface{})                     {}
func (noopLogWriter) Errorf(string, ...interface{})                    {}
func (noopLogWriter) Debugf(string, ...interface{})                    {}
func (n noopLogWriter) WithField(string, interface{}) logger.LogWriter { return n }
func (n noopLogWriter) WithFields(logrus.Fields) logger.LogWriter      { return n }

// TestNewService_NotASingleton verifies NewService returns a fresh instance on
// every call, so multiple engines and isolated tests are possible.
func TestNewService_NotASingleton(t *testing.T) {
	a := NewService(noopLogWriter{})
	b := NewService(noopLogWriter{})
	assert.NotSame(t, a, b, "NewService must not return a process-wide singleton")
}
