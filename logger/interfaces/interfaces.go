package interfaces

import (
	"fmt"
	"strings"
)

const (
	PanicLevel int32 = iota + 1
	FatalLevel
	ErrorLevel
	WarnLevel
	InfoLevel
	DebugLevel
)

// ParseLevel parses a level name into its level constant. It is case
// insensitive and accepts the aliases "err" and "warning".
func ParseLevel(level string) (int32, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "panic":
		return PanicLevel, nil
	case "fatal":
		return FatalLevel, nil
	case "error", "err":
		return ErrorLevel, nil
	case "warn", "warning":
		return WarnLevel, nil
	case "info":
		return InfoLevel, nil
	case "debug":
		return DebugLevel, nil
	}
	return InfoLevel, fmt.Errorf("unknown log level: %q", level)
}

// Logger interface for pitaya loggers
type Logger interface {
	Fatal(format ...interface{})
	Fatalf(format string, args ...interface{})
	Fatalln(args ...interface{})

	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
	Debugln(args ...interface{})

	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Errorln(args ...interface{})

	Info(args ...interface{})
	Infof(format string, args ...interface{})
	Infoln(args ...interface{})

	Warn(args ...interface{})
	Warnf(format string, args ...interface{})
	Warnln(args ...interface{})

	Panic(args ...interface{})
	Panicf(format string, args ...interface{})
	Panicln(args ...interface{})

	LogWithErrorLevel(err error, args ...interface{})
	LogfWithErrorLevel(err error, format string, args ...interface{})
	LoglnWithErrorLevel(err error, args ...interface{})

	WithFields(fields map[string]interface{}) Logger
	WithField(key string, value interface{}) Logger
	WithError(err error) Logger

	Enabled(level int32) bool
	SetLevel(level int32) error

	GetInternalLogger() any
}
