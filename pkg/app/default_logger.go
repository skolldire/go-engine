package app

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/logger/logrusadapter"
	"go.elastic.co/ecslogrus"
)

// newDefaultLogger builds the bootstrap logger used before application config
// is loaded. It wraps a *logrus.Logger with ECS formatting so that the adapter
// can later extract the underlying logger for configuration in logger.NewService.
func newDefaultLogger() logger.LogWriter {
	l := logrus.New()
	l.SetOutput(os.Stdout)
	l.SetFormatter(&ecslogrus.Formatter{})
	l.Level = logrus.InfoLevel
	return logrusadapter.New(l)
}
