package logrus

import (
	"github.com/sirupsen/logrus"
	"github.com/topfreegames/pitaya/v2/errors"
	"github.com/topfreegames/pitaya/v2/logger/interfaces"
)

type logrusImpl struct {
	logrus.FieldLogger
}

// New returns a new interfaces.Logger implementation based on logrus
func New() interfaces.Logger {
	log := logrus.New()
	return NewWithLogger(log)
}

// NewWithEntry returns a new interfaces.Logger implementation based on a provided logrus entry instance
// Deprecated: NewWithEntry is deprecated.
func NewWithEntry(logger *logrus.Entry) interfaces.Logger {
	return &logrusImpl{FieldLogger: logger}
}

// NewWithLogger returns a new interfaces.Logger implementation based on a provided logrus instance
// Deprecated: NewWithLogger is deprecated.
func NewWithLogger(logger *logrus.Logger) interfaces.Logger {
	return &logrusImpl{FieldLogger: logrus.NewEntry(logger)}
}

// NewWithFieldLogger returns a new interfaces.Logger implementation based on a provided logrus instance
func NewWithFieldLogger(logger logrus.FieldLogger) interfaces.Logger {
	return &logrusImpl{FieldLogger: logger}
}

func (l *logrusImpl) WithFields(fields map[string]interface{}) interfaces.Logger {
	return &logrusImpl{FieldLogger: l.FieldLogger.WithFields(fields)}
}

func (l *logrusImpl) WithField(key string, value interface{}) interfaces.Logger {
	return &logrusImpl{FieldLogger: l.FieldLogger.WithField(key, value)}
}

func (l *logrusImpl) WithError(err error) interfaces.Logger {
	return &logrusImpl{FieldLogger: l.FieldLogger.WithError(err)}
}

func (l *logrusImpl) GetInternalLogger() any {
	return l.FieldLogger
}

func (l *logrusImpl) LogWithErrorLevel(err error, args ...interface{}) {
	level, log := l.prepareLoggerWithError(err)
	switch level {
	case interfaces.PanicLevel:
		log.Panic(args...)
	case interfaces.FatalLevel:
		log.Fatal(args...)
	case interfaces.ErrorLevel:
		log.Error(args...)
	case interfaces.WarnLevel:
		log.Warn(args...)
	case interfaces.InfoLevel:
		log.Info(args...)
	case interfaces.DebugLevel:
		log.Debug(args...)
	default:
		log.Error(args...)
	}
}

func (l *logrusImpl) LogfWithErrorLevel(err error, format string, args ...interface{}) {
	level, log := l.prepareLoggerWithError(err)
	switch level {
	case interfaces.PanicLevel:
		log.Panicf(format, args...)
	case interfaces.FatalLevel:
		log.Fatalf(format, args...)
	case interfaces.ErrorLevel:
		log.Errorf(format, args...)
	case interfaces.WarnLevel:
		log.Warnf(format, args...)
	case interfaces.InfoLevel:
		log.Infof(format, args...)
	case interfaces.DebugLevel:
		log.Debugf(format, args...)
	default:
		log.Errorf(format, args...)
	}
}

func (l *logrusImpl) LoglnWithErrorLevel(err error, args ...interface{}) {
	level, log := l.prepareLoggerWithError(err)
	switch level {
	case interfaces.PanicLevel:
		log.Panicln(args...)
	case interfaces.FatalLevel:
		log.Fatalln(args...)
	case interfaces.ErrorLevel:
		log.Errorln(args...)
	case interfaces.WarnLevel:
		log.Warnln(args...)
	case interfaces.InfoLevel:
		log.Infoln(args...)
	case interfaces.DebugLevel:
		log.Debugln(args...)
	default:
		log.Errorln(args...)
	}
}

func (l *logrusImpl) prepareLoggerWithError(err error) (int32, *logrusImpl) {
	if err == nil {
		return interfaces.InfoLevel, l
	}
	var level = interfaces.ErrorLevel
	if e, ok := err.(errors.PitayaError); ok {
		level = e.GetLevel()
	}
	return level, l
}
