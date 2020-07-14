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

package zipkin

import (
	"io"
	"time"
)

import (
	"github.com/opentracing/opentracing-go"
	zipkinot "github.com/openzipkin-contrib/zipkin-go-opentracing"
	"github.com/openzipkin/zipkin-go"
	"github.com/openzipkin/zipkin-go/reporter/http"
)

// Name sets the name of this tracer.
const Name = "zipkin"

// Config provides configuration settings for a zipkin tracer.
type Config struct {
	HTTPEndpoint string  // HTTP Endpoint to report traces to
	SameSpan     bool    // Use Zipkin SameSpan RPC style traces
	ID128Bit     bool    // Use Zipkin 128 bit root span IDs
	SampleRate   float64 // The rate between 0.0 and 1.0 of requests to trace
}

// SetDefaults sets the default values.
func (c *Config) SetDefaults() {
	c.HTTPEndpoint = "http://localhost:9411/api/v2/spans"
	c.SameSpan = false
	c.ID128Bit = true
	c.SampleRate = 1.0
}

// Setup sets up the tracer
func (c *Config) Setup(serviceName string) (opentracing.Tracer, io.Closer, error) {
	// create our local endpoint
	endpoint, err := zipkin.NewEndpoint(serviceName, "0.0.0.0:0")
	if err != nil {
		return nil, nil, err
	}

	// create our sampler
	sampler, err := zipkin.NewBoundarySampler(c.SampleRate, time.Now().Unix())
	if err != nil {
		return nil, nil, err
	}

	// create the span reporter
	reporter := http.NewReporter(c.HTTPEndpoint)

	// create the native Zipkin tracer
	nativeTracer, err := zipkin.NewTracer(
		reporter,
		zipkin.WithLocalEndpoint(endpoint),
		zipkin.WithSharedSpans(c.SameSpan),
		zipkin.WithTraceID128Bit(c.ID128Bit),
		zipkin.WithSampler(sampler),
	)
	if err != nil {
		return nil, nil, err
	}

	// wrap the Zipkin native tracer with the OpenTracing Bridge
	tracer := zipkinot.Wrap(nativeTracer)

	// Without this, child spans are getting the NOOP tracer
	opentracing.SetGlobalTracer(tracer)

	return tracer, reporter, nil
}
