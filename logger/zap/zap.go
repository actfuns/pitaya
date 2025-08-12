package zap

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/topfreegames/pitaya/v2/errors"
	"github.com/topfreegames/pitaya/v2/logger/interfaces"
)

type zapImpl struct {
	sugar *zap.SugaredLogger
}

func New() interfaces.Logger {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	logger, _ := cfg.Build(zap.AddCaller(), zap.AddCallerSkip(1))
	return &zapImpl{
		sugar: logger.Sugar(),
	}
}

func NewWithLogger(logger *zap.SugaredLogger) interfaces.Logger {
	return &zapImpl{sugar: logger}
}

func (l *zapImpl) WithFields(fields map[string]interface{}) interfaces.Logger {
	kv := make([]interface{}, 0, len(fields)*2)
	for k, v := range fields {
		kv = append(kv, k, v)
	}
	return &zapImpl{sugar: l.sugar.With(kv...)}
}

func (l *zapImpl) WithField(key string, value interface{}) interfaces.Logger {
	return &zapImpl{sugar: l.sugar.With(zap.Any(key, value))}
}

func (l *zapImpl) WithError(err error) interfaces.Logger {
	if err == nil {
		return l
	}
	return &zapImpl{sugar: l.sugar.With(zap.Error(err))}
}

func (l *zapImpl) Fatal(args ...interface{}) {
	l.sugar.Fatal(args...)
}

func (l *zapImpl) Fatalf(format string, args ...interface{}) {
	l.sugar.Fatalf(format, args...)
}

func (l *zapImpl) Fatalln(args ...interface{}) {
	l.sugar.Fatal(fmt.Sprintln(args...))
}

func (l *zapImpl) Debug(args ...interface{}) {
	l.sugar.Debug(args...)
}

func (l *zapImpl) Debugf(format string, args ...interface{}) {
	l.sugar.Debugf(format, args...)
}

func (l *zapImpl) Debugln(args ...interface{}) {
	l.sugar.Debug(fmt.Sprintln(args...))
}

func (l *zapImpl) Error(args ...interface{}) {
	l.sugar.Error(args...)
}

func (l *zapImpl) Errorf(format string, args ...interface{}) {
	l.sugar.Errorf(format, args...)
}

func (l *zapImpl) Errorln(args ...interface{}) {
	l.sugar.Error(fmt.Sprintln(args...))
}

func (l *zapImpl) Info(args ...interface{}) {
	l.sugar.Info(args...)
}

func (l *zapImpl) Infof(format string, args ...interface{}) {
	l.sugar.Infof(format, args...)
}

func (l *zapImpl) Infoln(args ...interface{}) {
	l.sugar.Info(fmt.Sprintln(args...))
}

func (l *zapImpl) Warn(args ...interface{}) {
	l.sugar.Warn(args...)
}

func (l *zapImpl) Warnf(format string, args ...interface{}) {
	l.sugar.Warnf(format, args...)
}

func (l *zapImpl) Warnln(args ...interface{}) {
	l.sugar.Warn(fmt.Sprintln(args...))
}

func (l *zapImpl) Panic(args ...interface{}) {
	l.sugar.Panic(args...)
}

func (l *zapImpl) Panicf(format string, args ...interface{}) {
	l.sugar.Panicf(format, args...)
}

func (l *zapImpl) Panicln(args ...interface{}) {
	l.sugar.Panic(fmt.Sprintln(args...))
}

func (l *zapImpl) GetInternalLogger() any {
	return l.sugar
}

func (l *zapImpl) LogErrorWithLevel(err error, args ...interface{}) {
	level, log := l.prepareLoggerWithError(err)
	switch level {
	case interfaces.PanicLevel:
		log.sugar.Panic(args...)
	case interfaces.FatalLevel:
		log.sugar.Fatal(args...)
	case interfaces.ErrorLevel:
		log.sugar.Error(args...)
	case interfaces.WarnLevel:
		log.sugar.Warn(args...)
	case interfaces.InfoLevel:
		log.sugar.Info(args...)
	case interfaces.DebugLevel:
		log.sugar.Debug(args...)
	default:
		log.sugar.Error(args...)
	}
}

func (l *zapImpl) LogErrorWithLevelf(err error, format string, args ...interface{}) {
	level, log := l.prepareLoggerWithError(err)
	switch level {
	case interfaces.PanicLevel:
		log.sugar.Panicf(format, args...)
	case interfaces.FatalLevel:
		log.sugar.Fatalf(format, args...)
	case interfaces.ErrorLevel:
		log.sugar.Errorf(format, args...)
	case interfaces.WarnLevel:
		log.sugar.Warnf(format, args...)
	case interfaces.InfoLevel:
		log.sugar.Infof(format, args...)
	case interfaces.DebugLevel:
		log.sugar.Debugf(format, args...)
	default:
		log.sugar.Errorf(format, args...)
	}
}

func (l *zapImpl) LogErrorWithLevelln(err error, args ...interface{}) {
	level, log := l.prepareLoggerWithError(err)
	switch level {
	case interfaces.PanicLevel:
		log.sugar.Panicln(args...)
	case interfaces.FatalLevel:
		log.sugar.Fatalln(args...)
	case interfaces.ErrorLevel:
		log.sugar.Errorln(args...)
	case interfaces.WarnLevel:
		log.sugar.Warnln(args...)
	case interfaces.InfoLevel:
		log.sugar.Infoln(args...)
	case interfaces.DebugLevel:
		log.sugar.Debugln(args...)
	default:
		log.sugar.Errorln(args...)
	}
}

func (l *zapImpl) prepareLoggerWithError(err error) (int32, *zapImpl) {
	if err == nil {
		return interfaces.InfoLevel, l
	}
	var level = interfaces.ErrorLevel
	if e, ok := err.(errors.PitayaError); ok {
		level = e.GetLevel()
	}
	return level, &zapImpl{sugar: l.sugar.With(zap.Error(err))}
}
