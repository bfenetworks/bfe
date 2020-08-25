// Copyright (c) 2019 The BFE Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Copyright (c) 2016-2020 Containous SAS

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

package jaeger

import (
	"fmt"
	"io"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/opentracing/opentracing-go"
	jaegercli "github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-client-go/zipkin"
	jaegermet "github.com/uber/jaeger-lib/metrics"
)

// Name sets the name of this tracer.
const Name = "jaeger"

// Config provides configuration settings for a jaeger tracer.
type Config struct {
	SamplingServerURL      string  // Set the sampling server url
	SamplingType           string  // Set the sampling type
	SamplingParam          float64 // Set the sampling parameter
	LocalAgentHostPort     string  // Set jaeger-agent's host:port that the reporter will used
	Gen128Bit              bool    // Generate 128 bit span IDs
	Propagation            string  // Which propagation format to use (jaeger/b3)
	TraceContextHeaderName string  // Set the header to use for the trace-id
	CollectorEndpoint      string  // Instructs reporter to send spans to jaeger-collector at this URL
	CollectorUser          string  // CollectorUser for basic http authentication when sending spans to jaeger-collector
	CollectorPassword      string  // CollectorPassword for basic http authentication when sending spans to jaeger-collector
}

// SetDefaults sets the default values.
func (c *Config) SetDefaults() {
	c.SamplingServerURL = "http://localhost:5778/sampling"
	c.SamplingType = "const"
	c.SamplingParam = 1.0
	c.LocalAgentHostPort = "127.0.0.1:6831"
	c.Propagation = "jaeger"
	c.Gen128Bit = true
	c.TraceContextHeaderName = jaegercli.TraceContextHeaderName
	c.CollectorEndpoint = ""
	c.CollectorUser = ""
	c.CollectorPassword = ""
}

// Setup sets up the tracer
func (c *Config) Setup(componentName string) (opentracing.Tracer, io.Closer, error) {
	reporter := &jaegercfg.ReporterConfig{
		LogSpans:           true,
		LocalAgentHostPort: c.LocalAgentHostPort,
		CollectorEndpoint:  c.CollectorEndpoint,
		User:               c.CollectorUser,
		Password:           c.CollectorPassword,
	}

	jcfg := jaegercfg.Configuration{
		Sampler: &jaegercfg.SamplerConfig{
			SamplingServerURL: c.SamplingServerURL,
			Type:              c.SamplingType,
			Param:             c.SamplingParam,
		},
		Reporter: reporter,
		Headers: &jaegercli.HeadersConfig{
			TraceContextHeaderName: c.TraceContextHeaderName,
		},
	}

	jMetricsFactory := jaegermet.NullFactory

	opts := []jaegercfg.Option{
		jaegercfg.Metrics(jMetricsFactory),
		jaegercfg.Gen128Bit(c.Gen128Bit),
	}

	switch c.Propagation {
	case "b3":
		p := zipkin.NewZipkinB3HTTPHeaderPropagator()
		opts = append(opts,
			jaegercfg.Injector(opentracing.HTTPHeaders, p),
			jaegercfg.Extractor(opentracing.HTTPHeaders, p),
		)
	case "jaeger", "":
	default:
		return nil, nil, fmt.Errorf("unknown propagation format: %s", c.Propagation)
	}

	// Initialize tracer with a logger and a metrics factory
	closer, err := jcfg.InitGlobalTracer(
		componentName,
		opts...,
	)
	if err != nil {
		log.Logger.Error("Could not initialize jaeger tracer: %s", err.Error())
		return nil, nil, err
	}
	return opentracing.GlobalTracer(), closer, nil
}
