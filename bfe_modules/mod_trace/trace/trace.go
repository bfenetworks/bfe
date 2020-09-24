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

package trace

import (
	"fmt"
	"io"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

// TraceAgent is an abstraction for trace agent (Zipkin, Jaeger, ...).
type TraceAgent interface {
	Setup(componentName string) (opentracing.Tracer, io.Closer, error)
}

type Trace struct {
	ServiceName string

	tracer opentracing.Tracer
	closer io.Closer
}

func NewTrace(serviceName string, traceAgent TraceAgent) (*Trace, error) {
	trace := &Trace{
		ServiceName: serviceName,
	}

	if traceAgent == nil {
		return nil, fmt.Errorf("not set trace agent")
	}

	var err error
	trace.tracer, trace.closer, err = traceAgent.Setup(serviceName)
	if err != nil {
		return nil, err
	}
	return trace, nil
}

// StartSpan delegates to opentracing.Tracer.
func (t *Trace) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	return t.tracer.StartSpan(operationName, opts...)
}

// Inject delegates to opentracing.Tracer.
func (t *Trace) Inject(sm opentracing.SpanContext, format interface{}, carrier interface{}) error {
	return t.tracer.Inject(sm, format, carrier)
}

// Extract delegates to opentracing.Tracer.
func (t *Trace) Extract(format interface{}, carrier interface{}) (opentracing.SpanContext, error) {
	return t.tracer.Extract(format, carrier)
}

// IsEnabled determines if Tracer was successfully activated.
func (t *Trace) IsEnabled() bool {
	return t != nil && t.tracer != nil
}

// Close trace
func (t *Trace) Close() {
	if t.closer != nil {
		err := t.closer.Close()
		if err != nil {
			log.Logger.Error("close trace error, %v", err)
		}
	}
}

// LogRequest used to create span tags from the request.
func LogRequest(span opentracing.Span, r *bfe_http.Request) {
	if span != nil && r != nil && r.URL != nil {
		ext.HTTPMethod.Set(span, r.Method)
		ext.HTTPUrl.Set(span, r.URL.String())
		span.SetTag("http.host", r.Host)
	}
}

// LogBackend used to log backend info
func LogBackend(span opentracing.Span, r *bfe_basic.Request) {
	if span != nil && r != nil {
		if len(r.Route.Product) > 0 {
			span.SetTag("product", r.Route.Product)
		}

		if len(r.Backend.ClusterName) > 0 {
			span.SetTag("cluster", r.Backend.ClusterName)
		}

		if len(r.Backend.SubclusterName) > 0 {
			span.SetTag("subcluster", r.Backend.SubclusterName)
		}

		if len(r.Backend.BackendAddr) > 0 {
			span.SetTag("backend", fmt.Sprintf("%s:%d", r.Backend.BackendAddr, r.Backend.BackendPort))
		}
	}
}

// LogResponseCode used to log response code in span.
func LogResponseCode(span opentracing.Span, code int) {
	if span != nil {
		ext.HTTPStatusCode.Set(span, uint16(code))
	}
}

// LogEventf logs an event to the span in the request context.
func LogEventf(span opentracing.Span, format string, args ...interface{}) {
	if span != nil {
		span.LogKV("event", fmt.Sprintf(format, args...))
	}
}

// SetError flags the span associated with this request as in error.
func SetError(span opentracing.Span) {
	if span != nil {
		ext.Error.Set(span, true)
	}
}

// SetErrorWithEvent flags the span associated with this request as in error and log an event.
func SetErrorWithEvent(span opentracing.Span, format string, args ...interface{}) {
	SetError(span)
	LogEventf(span, format, args...)
}
