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

package otel

import (
	"context"
	"io"
	"time"

	"github.com/baidu/go-lib/log"
	"github.com/opentracing/opentracing-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelbridge "go.opentelemetry.io/otel/bridge/opentracing"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const Name = "otel"

type Config struct {
	Endpoint    string  // OTLP endpoint
	Insecure    bool    // Use insecure connection
	SampleRate  float64 // The rate between 0.0 and 1.0 of requests to trace
	ServiceName string  // The name of this service
}

func (c *Config) SetDefaults() {
	c.Endpoint = "localhost:4317"
	c.Insecure = false
	c.SampleRate = 1.0
	c.ServiceName = "bfe"
}

func (c *Config) Setup(serviceName string) (opentracing.Tracer, io.Closer, error) {
	if c.ServiceName == "" {
		c.ServiceName = serviceName
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(c.Endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		log.Logger.Error("Could not initialize OTLP exporter: %s", err.Error())
		return nil, nil, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(c.ServiceName),
			semconv.ServiceVersion("1.0.0"),
			attribute.String("component", "bfe"),
		),
	)
	if err != nil {
		log.Logger.Error("Could not create resource: %s", err.Error())
		return nil, nil, err
	}

	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(c.SampleRate))

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(traceProvider)

	tracer := traceProvider.Tracer(c.ServiceName)
	bridgeTracer, _ := otelbridge.NewTracerPair(tracer)

	opentracing.SetGlobalTracer(bridgeTracer)

	return bridgeTracer, &otelCloser{traceProvider: traceProvider}, nil
}

type otelCloser struct {
	traceProvider *sdktrace.TracerProvider
}

func (c *otelCloser) Close() error {
	if c.traceProvider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return c.traceProvider.Shutdown(ctx)
	}
	return nil
}
