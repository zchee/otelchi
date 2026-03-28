// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package semconv

import (
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/noop"
)

func TestHTTPServerDoesNotPanic(t *testing.T) {
	testCases := []struct {
		name   string
		server HTTPServer
	}{
		{
			name:   "nil meter",
			server: NewHTTPServer(nil),
		},
		{
			name:   "with Meter",
			server: NewHTTPServer(noop.Meter{}),
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("unexpected panic: %v", r)
					}
				}()
				req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
				if err != nil {
					t.Fatalf("failed to create request: %v", err)
				}

				_ = tt.server.RequestTraceAttrs("stuff", req, RequestTraceAttrsOpts{})
				_ = tt.server.ResponseTraceAttrs(ResponseTelemetry{StatusCode: 200})
				tt.server.RecordMetrics(t.Context(), ServerMetricData{
					ServerName: "stuff",
					MetricAttributes: MetricAttributes{
						Req: req,
					},
				})
			}()
		})
	}
}

func TestServerNetworkTransportAttr(t *testing.T) {
	for _, tt := range []struct {
		name    string
		network string

		wantAttributes []attribute.KeyValue
	}{
		{
			name:    "without any opt-in",
			network: "tcp",

			wantAttributes: []attribute.KeyValue{
				attribute.String("network.transport", "tcp"),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := NewHTTPServer(nil)

			got := s.NetworkTransportAttr(tt.network)
			if diff := cmp.Diff(tt.wantAttributes, got, cmpopts.IgnoreUnexported(attribute.Value{})); diff != "" {
				t.Errorf("NetworkTransportAttr() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestHTTPClientDoesNotPanic(t *testing.T) {
	testCases := []struct {
		name   string
		client HTTPClient
	}{
		{
			name:   "nil meter",
			client: NewHTTPClient(nil),
		},
		{
			name:   "with Meter",
			client: NewHTTPClient(noop.Meter{}),
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("unexpected panic: %v", r)
					}
				}()
				req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
				if err != nil {
					t.Fatalf("failed to create request: %v", err)
				}

				_ = tt.client.RequestTraceAttrs(req)
				_ = tt.client.ResponseTraceAttrs(&http.Response{StatusCode: 200})

				opts := tt.client.MetricOptions(MetricAttributes{
					Req:        req,
					StatusCode: 200,
				})
				tt.client.RecordMetrics(t.Context(), MetricData{
					RequestSize: 20,
					ElapsedTime: 1,
				}, opts)
			}()
		})
	}
}

func TestHTTPClientTraceAttributes(t *testing.T) {
	for _, tt := range []struct {
		name string

		wantAttributes []attribute.KeyValue
	}{
		{
			name: "with no optin set",

			wantAttributes: []attribute.KeyValue{
				attribute.String("server.address", "example.com"),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := NewHTTPClient(nil)
			a := c.TraceAttributes("example.com")
			if diff := cmp.Diff(tt.wantAttributes, a, cmpopts.IgnoreUnexported(attribute.Value{})); diff != "" {
				t.Errorf("TraceAttributes() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClientTraceAttributes(t *testing.T) {
	for _, tt := range []struct {
		name string
		host string

		wantAttributes []attribute.KeyValue
	}{
		{
			name: "without any opt-in",
			host: "example.com",

			wantAttributes: []attribute.KeyValue{
				attribute.String("server.address", "example.com"),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := NewHTTPClient(nil)

			got := s.TraceAttributes(tt.host)
			if diff := cmp.Diff(tt.wantAttributes, got, cmpopts.IgnoreUnexported(attribute.Value{})); diff != "" {
				t.Errorf("TraceAttributes() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func BenchmarkRecordMetrics(b *testing.B) {
	benchmarks := []struct {
		name   string
		server HTTPServer
	}{
		{
			name:   "empty",
			server: HTTPServer{},
		},
		{
			name:   "nil meter",
			server: NewHTTPServer(nil),
		},
		{
			name:   "with Meter",
			server: NewHTTPServer(noop.Meter{}),
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			req, _ := http.NewRequest("GET", "http://example.com", http.NoBody)
			_ = bm.server.RequestTraceAttrs("stuff", req, RequestTraceAttrsOpts{})
			_ = bm.server.ResponseTraceAttrs(ResponseTelemetry{StatusCode: 200})
			ctx := b.Context()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				bm.server.RecordMetrics(ctx, ServerMetricData{
					ServerName: bm.name,
					MetricAttributes: MetricAttributes{
						Req: req,
					},
				})
			}
		})
	}
}
