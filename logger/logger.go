// Copyright (c) TFG Co. All Rights Reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package logger

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/topfreegames/pitaya/v2/constants"
	"github.com/topfreegames/pitaya/v2/logger/interfaces"
	logruswrapper "github.com/topfreegames/pitaya/v2/logger/logrus"
	zapwrapper "github.com/topfreegames/pitaya/v2/logger/zap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Log is the default logger
var Log = initZapLogger()

func initLogrusLogger() interfaces.Logger {
	plog := logrus.New()
	plog.Formatter = new(logrus.TextFormatter)
	plog.Level = logrus.DebugLevel

	log := plog.WithFields(logrus.Fields{
		"source": "pitaya",
	})
	return logruswrapper.NewWithLogger(log.Logger)
}

func initZapLogger() interfaces.Logger {
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	cfg.Encoding = "console"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	cfg.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	logger, err := cfg.Build(
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.PanicLevel),
	)
	if err != nil {
		panic(err)
	}
	sugar := logger.Sugar().With("source", "pitaya")
	return zapwrapper.NewWithSugaredLogger(sugar)
}

// SetLogger rewrites the default logger
func SetLogger(l interfaces.Logger) {
	if l != nil {
		Log = l
	}
}

// WithCtx returns a logger with the context
func WithCtx(ctx context.Context) interfaces.Logger {
	if ctx == nil {
		return Log
	}
	l := ctx.Value(constants.LoggerCtxKey)
	if l == nil {
		return Log
	}

	return l.(interfaces.Logger)
}
