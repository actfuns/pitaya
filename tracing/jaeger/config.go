/*
 * Copyright (c) 2018 TFG Co <backend@tfgco.com>
 * Author: TFG Co <backend@tfgco.com>
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy of
 * this software and associated documentation files (the "Software"), to deal in
 * the Software without restriction, including without limitation the rights to
 * use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
 * the Software, and to permit persons to whom the Software is furnished to do so,
 * subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
 * FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
 * COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
 * IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
 * CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

package jaeger

import (
	"io"
	"time"

	"github.com/topfreegames/pitaya/v2/logger"
	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/config"
)

// JaegerConfig holds configuration options for Jaeger
type JaegerConfig struct {
	ServiceName       string  `yaml:"serviceName"`
	CollectorEndpoint string  `yaml:"collectorEndpoint"`
	User              string  `yaml:"user,omitempty"`
	Password          string  `yaml:"password,omitempty"`
	Disabled          bool    `yaml:"disabled"`
	SamplerType       string  `yaml:"samplerType"`
	SamplerParam      float64 `yaml:"samplerParam"`
	FlushInterval     int     `yaml:"flushInterval"`
}

func NewDefaultJaegerConfig() JaegerConfig {
	return JaegerConfig{
		CollectorEndpoint: "http://localhost:14268/api/traces",
		User:              "",
		Password:          "",
		Disabled:          true,
		SamplerType:       jaeger.SamplerTypeProbabilistic,
		SamplerParam:      0.1,
		FlushInterval:     10,
	}
}

// Configure configures a global Jaeger tracer
func Configure(opt JaegerConfig) (io.Closer, error) {
	logger.Log.Infof("Configuring Jaeger with options: %+v", opt)

	cfg := config.Configuration{
		ServiceName: opt.ServiceName,
		Disabled:    opt.Disabled,
		Sampler: &config.SamplerConfig{
			Type:  opt.SamplerType,
			Param: opt.SamplerParam,
		},
		Reporter: &config.ReporterConfig{
			LogSpans:            true,
			CollectorEndpoint:   opt.CollectorEndpoint,
			User:                opt.User,
			Password:            opt.Password,
			BufferFlushInterval: time.Duration(opt.FlushInterval) * time.Second,
		},
	}

	return cfg.InitGlobalTracer(opt.ServiceName)
}
