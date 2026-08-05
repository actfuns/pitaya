package test

import (
	"github.com/actfuns/pitaya/v2/logger/interfaces"
	lwrapper "github.com/actfuns/pitaya/v2/logger/logrus"
	tests "github.com/sirupsen/logrus/hooks/test"
	"io"
)

// NewNullLogger creates a discarding logger and installs the test hook.
func NewNullLogger() (interfaces.Logger, *tests.Hook) {
	logger, hook := tests.NewNullLogger()
	logger.Out = io.Discard
	return lwrapper.NewWithFieldLogger(logger), hook
}
