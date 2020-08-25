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

package elastic

import (
	"io"
	"net/url"
)

import (
	"github.com/opentracing/opentracing-go"
	"go.elastic.co/apm"
	"go.elastic.co/apm/module/apmot"
	"go.elastic.co/apm/transport"
)

// Name sets the name of this tracer.
const Name = "elastic"

func init() {
	// The APM lib uses the init() function to create a default tracer.
	// So this default tracer must be disabled.
	// https://github.com/elastic/apm-agent-go/blob/8dd383d0d21776faad8841fe110f35633d199a03/tracer.go#L61-L65
	apm.DefaultTracer.Close()
}

// Config provides configuration settings for a elastic.co tracer.
type Config struct {
	ServerURL   string // Set the URL of the Elastic APM server
	SecretToken string // Set the token used to connect to Elastic APM Server
}

// Setup sets up the tracer.
func (c *Config) Setup(serviceName string) (opentracing.Tracer, io.Closer, error) {
	// Create default transport.
	tr, err := transport.NewHTTPTransport()
	if err != nil {
		return nil, nil, err
	}

	if c.ServerURL != "" {
		serverURL, err := url.Parse(c.ServerURL)
		if err != nil {
			return nil, nil, err
		}
		tr.SetServerURL(serverURL)
	}

	if c.SecretToken != "" {
		tr.SetSecretToken(c.SecretToken)
	}

	tracer, err := apm.NewTracerOptions(apm.TracerOptions{
		ServiceName: serviceName,
		Transport:   tr,
	})
	if err != nil {
		return nil, nil, err
	}

	otTracer := apmot.New(apmot.WithTracer(tracer))

	// Without this, child spans are getting the NOOP tracer
	opentracing.SetGlobalTracer(otTracer)

	return otTracer, nil, nil
}
