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
	"testing"

	"github.com/topfreegames/pitaya/v2/errors"
	"github.com/topfreegames/pitaya/v2/logger/interfaces"

	"github.com/stretchr/testify/assert"
	logruswrapper "github.com/topfreegames/pitaya/v2/logger/logrus"
)

func TestInitLogrusLogger(t *testing.T) {
	// 初始化 zap logger
	log := initLogrusLogger()

	// 基础级别日志
	log.Debug("debug msg")
	log.Debugf("debug %s", "formatted")
	log.Debugln("debug", "ln")

	log.Info("info msg")
	log.Infof("info %s", "formatted")
	log.Infoln("info", "ln")

	log.Warn("warn msg")
	log.Warnf("warn %s", "formatted")
	log.Warnln("warn", "ln")

	log.Error("error msg")
	log.Errorf("error %s", "formatted")
	log.Errorln("error", "ln")

	// Panic/Fatal 级别我们不直接执行（防止中断测试）
	// 所以我们只验证 WithFields/WithError 和 LogErrorWithLevel
	err := &errors.Error{Level: interfaces.ErrorLevel, Message: "error msg"}
	log.WithError(err).Error("with error field")
	log.WithField("key1", "val1").Info("with single field")
	log.WithFields(map[string]interface{}{
		"f1": "v1",
		"f2": 123,
	}).Info("with multiple fields")

	// 测试 LogErrorWithLevel 系列
	log.LogWithErrorLevel(err, "logerror simple")
	log.LogfWithErrorLevel(err, "logerror formatted: %d", 100)
	log.LoglnWithErrorLevel(err, "logerror ln 1", "and more")

	// 测试传入 nil error
	log.LogWithErrorLevel(nil, "no error")
	log.LogfWithErrorLevel(nil, "no error formatted")
	log.LoglnWithErrorLevel(nil, "no error ln")

	// 内部 logger 获取（只是验证不会 panic）
	_ = log.GetInternalLogger()
}

func TestInitZapLogger(t *testing.T) {
	// 初始化 zap logger
	log := initZapLogger()

	// 基础级别日志
	log.Debug("debug msg")
	log.Debugf("debug %s", "formatted")
	log.Debugln("debug", "ln")

	log.Info("info msg")
	log.Infof("info %s", "formatted")
	log.Infoln("info", "ln")

	log.Warn("warn msg")
	log.Warnf("warn %s", "formatted")
	log.Warnln("warn", "ln")

	log.Error("error msg")
	log.Errorf("error %s", "formatted")
	log.Errorln("error", "ln")

	// Panic/Fatal 级别我们不直接执行（防止中断测试）
	// 所以我们只验证 WithFields/WithError 和 LogErrorWithLevel
	err := &errors.Error{Level: interfaces.ErrorLevel, Message: "error msg"}
	log.WithError(err).Error("with error field")
	log.WithField("key1", "val1").Info("with single field")
	log.WithFields(map[string]interface{}{
		"f1": "v1",
		"f2": 123,
	}).Info("with multiple fields")

	// 测试 LogErrorWithLevel 系列
	log.LogWithErrorLevel(err, "logerror simple")
	log.LogfWithErrorLevel(err, "logerror formatted: %d", 100)
	log.LoglnWithErrorLevel(err, "logerror ln 1", "and more")

	// 测试传入 nil error
	log.LogWithErrorLevel(nil, "no error")
	log.LogfWithErrorLevel(nil, "no error formatted")
	log.LoglnWithErrorLevel(nil, "no error ln")

	// 内部 logger 获取（只是验证不会 panic）
	_ = log.GetInternalLogger()
}

func TestSetLogger(t *testing.T) {
	l := logruswrapper.New()
	SetLogger(l)
	assert.Equal(t, l, Log)
}
