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

package mod_otel

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	ModOtel = "mod_otel"
	CtxSpan = "mod_otel.span"
)

var (
	openDebug = false
	tracer    trace.Tracer
	tp        *sdktrace.TracerProvider
)

type ModuleOtel struct {
	name    string
	conf    *ConfModOtel
	metrics metrics.Metrics
	state   ModuleOtelState
}

type ModuleOtelState struct {
	StartSpanCount  *metrics.Counter
	FinishSpanCount *metrics.Counter
}

func NewModuleOtel() *ModuleOtel {
	m := new(ModuleOtel)
	m.name = ModOtel
	m.metrics.Init(&m.state, ModOtel, 0)
	return m
}

func (m *ModuleOtel) Name() string {
	return m.name
}

func (m *ModuleOtel) initTracer() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var opts []otlptracegrpc.Option
	if m.conf.Basic.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	opts = append(opts, otlptracegrpc.WithEndpoint(m.conf.Basic.Endpoint))

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(m.conf.Basic.ServiceName),
			semconv.ServiceVersion("1.0.0"),
			attribute.String("component", "bfe"),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(m.conf.Basic.SampleRate))

	tp = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer = tp.Tracer(m.conf.Basic.ServiceName)

	if openDebug {
		log.Logger.Info("OpenTelemetry initialized: service=%s, endpoint=%s",
			m.conf.Basic.ServiceName, m.conf.Basic.Endpoint)
	}

	return nil
}

func (m *ModuleOtel) startTrace(request *bfe_basic.Request) (int, *bfe_http.Response) {
	if !m.conf.Basic.Enabled {
		return bfe_module.BfeHandlerGoOn, nil
	}

	m.state.StartSpanCount.Inc(1)

	ctx := context.Background()
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(request.HttpRequest.Header))

	spanName := spanName(request.HttpRequest)
	ctx, span := tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindServer),
	)

	logRequest(span, request.HttpRequest)

	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(request.HttpRequest.Header))

	request.SetContext(CtxSpan, span)

	if openDebug {
		log.Logger.Info("%s start span: %s/%s", m.name, request.HttpRequest.Host, request.HttpRequest.URL.Path)
	}

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleOtel) finishTrace(req *bfe_basic.Request, res *bfe_http.Response) int {
	if !m.conf.Basic.Enabled {
		return bfe_module.BfeHandlerGoOn
	}

	value := req.GetContext(CtxSpan)
	if value == nil {
		return bfe_module.BfeHandlerGoOn
	}

	span := value.(trace.Span)
	defer span.End()

	if req.HttpResponse != nil {
		logResponseCode(span, req.HttpResponse.StatusCode)
	}

	if len(req.ErrMsg) > 0 {
		setErrorWithEvent(span, req.ErrMsg)
	}

	logBackend(span, req)

	m.state.FinishSpanCount.Inc(1)

	if openDebug {
		log.Logger.Info("%s finish span: %s/%s, err:[%s]", m.name,
			req.HttpRequest.Host, req.HttpRequest.URL.Path, req.ErrMsg)
	}

	return bfe_module.BfeHandlerGoOn
}

func (m *ModuleOtel) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleOtel) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleOtel) monitorHandlers() map[string]interface{} {
	return map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
}

func (m *ModuleOtel) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	var err error
	var conf *ConfModOtel

	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}

	m.conf = conf
	openDebug = conf.Log.OpenDebug

	if !m.conf.Basic.Enabled {
		log.Logger.Info("%s disabled, skip init", m.name)
		return nil
	}

	if err = m.initTracer(); err != nil {
		return fmt.Errorf("%s: init tracer err %s", m.name, err.Error())
	}

	if err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.startTrace); err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.startTrace): %v", m.name, err)
	}

	if err = cbs.AddFilter(bfe_module.HandleRequestFinish, m.finishTrace); err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.finishTrace): %v", m.name, err)
	}

	if err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers()); err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(monitor): %v", m.name, err)
	}

	log.Logger.Info("%s: Init OK", m.name)
	return nil
}

func spanName(r *bfe_http.Request) string {
	host := strings.SplitN(r.Host, ":", 2)[0]
	return host + r.URL.Path
}

func logRequest(span trace.Span, r *bfe_http.Request) {
	if span == nil || r == nil || r.URL == nil {
		return
	}
	span.SetAttributes(
		attribute.String("http.method", r.Method),
		attribute.String("http.url", r.URL.String()),
		attribute.String("http.host", r.Host),
	)
}

func logResponseCode(span trace.Span, code int) {
	if span == nil {
		return
	}
	span.SetAttributes(attribute.Int("http.status_code", code))
	if code >= http.StatusBadRequest {
		span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", code))
	}
}

func logBackend(span trace.Span, r *bfe_basic.Request) {
	if span == nil || r == nil {
		return
	}
	if len(r.Route.Product) > 0 {
		span.SetAttributes(attribute.String("product", r.Route.Product))
	}
	if len(r.Backend.ClusterName) > 0 {
		span.SetAttributes(attribute.String("cluster", r.Backend.ClusterName))
	}
	if len(r.Backend.SubclusterName) > 0 {
		span.SetAttributes(attribute.String("subcluster", r.Backend.SubclusterName))
	}
	if len(r.Backend.BackendAddr) > 0 {
		span.SetAttributes(attribute.String("backend",
			fmt.Sprintf("%s:%d", r.Backend.BackendAddr, r.Backend.BackendPort)))
	}
}

func setErrorWithEvent(span trace.Span, msg string) {
	if span == nil {
		return
	}
	span.SetStatus(codes.Error, msg)
	span.AddEvent("error", trace.WithAttributes(attribute.String("message", msg)))
}
