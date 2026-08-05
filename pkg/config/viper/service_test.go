package viper

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
)

// noopLogWriter is a minimal logger.LogWriter for tests.
type noopLogWriter struct{}

func (noopLogWriter) Info(...any)                                 {}
func (noopLogWriter) Warn(...any)                                 {}
func (noopLogWriter) Error(...any)                                {}
func (noopLogWriter) Debug(...any)                                {}
func (noopLogWriter) Fatal(...any)                                {}
func (noopLogWriter) Infof(string, ...any)                        {}
func (noopLogWriter) Warnf(string, ...any)                        {}
func (noopLogWriter) Errorf(string, ...any)                       {}
func (noopLogWriter) Debugf(string, ...any)                       {}
func (n noopLogWriter) WithField(string, any) logger.LogWriter    { return n }
func (n noopLogWriter) WithFields(logrus.Fields) logger.LogWriter { return n }

// TestNewService_NotASingleton verifies NewService returns a fresh instance on
// every call, so multiple engines and isolated tests are possible.
func TestNewService_NotASingleton(t *testing.T) {
	a := NewService(noopLogWriter{})
	b := NewService(noopLogWriter{})
	assert.NotSame(t, a, b, "NewService must not return a process-wide singleton")
}
