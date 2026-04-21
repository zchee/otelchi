// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package semconv

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"go.opentelemetry.io/otel/attribute"
)

func TestHTTPServer_MetricAttributes(t *testing.T) {
	defaultRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/path?query=test", http.NoBody)
	reqWithPattern := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/path/abc123", http.NoBody)
	reqWithPattern.Pattern = "/path/abc123"

	tests := []struct {
		name                 string
		server               string
		req                  *http.Request
		statusCode           int
		route                string
		additionalAttributes []attribute.KeyValue
		wantFunc             func(t *testing.T, attrs []attribute.KeyValue)
	}{
		{
			name:                 "routine testing",
			server:               "",
			req:                  defaultRequest,
			statusCode:           200,
			route:                "",
			additionalAttributes: []attribute.KeyValue{attribute.String("test", "test")},
			wantFunc: func(t *testing.T, attrs []attribute.KeyValue) {
				if len(attrs) != 7 {
					t.Fatalf("expected 7 attributes, got %d", len(attrs))
				}
				want := []attribute.KeyValue{
					attribute.String("http.request.method", "GET"),
					attribute.String("url.scheme", "http"),
					attribute.String("server.address", "example.com"),
					attribute.String("network.protocol.name", "http"),
					attribute.String("network.protocol.version", "1.1"),
					attribute.Int64("http.response.status_code", 200),
					attribute.String("test", "test"),
				}
				if diff := cmp.Diff(want, attrs, cmpopts.IgnoreUnexported(attribute.Value{}), cmpopts.SortSlices(func(a, b attribute.KeyValue) bool {
					return a.Key < b.Key
				})); diff != "" {
					t.Errorf("MetricAttributes() mismatch (-want +got):\n%s", diff)
				}
			},
		},
		{
			name:                 "use server address",
			server:               "example.com:9999",
			req:                  defaultRequest,
			statusCode:           200,
			route:                "/path/${id}",
			additionalAttributes: nil,
			wantFunc: func(t *testing.T, attrs []attribute.KeyValue) {
				if len(attrs) != 8 {
					t.Fatalf("expected 8 attributes, got %d", len(attrs))
				}
				want := []attribute.KeyValue{
					attribute.String("http.request.method", "GET"),
					attribute.String("url.scheme", "http"),
					attribute.String("server.address", "example.com"),
					attribute.Int("server.port", 9999),
					attribute.String("network.protocol.name", "http"),
					attribute.String("network.protocol.version", "1.1"),
					attribute.Int64("http.response.status_code", 200),
					attribute.String("http.route", "/path/${id}"),
				}
				if diff := cmp.Diff(want, attrs, cmpopts.IgnoreUnexported(attribute.Value{}), cmpopts.SortSlices(func(a, b attribute.KeyValue) bool {
					return a.Key < b.Key
				})); diff != "" {
					t.Errorf("MetricAttributes() mismatch (-want +got):\n%s", diff)
				}
			},
		},
		{
			name: "use route from request pattern",
			req:  reqWithPattern,
			wantFunc: func(t *testing.T, attrs []attribute.KeyValue) {
				if len(attrs) != 6 {
					t.Fatalf("expected 6 attributes, got %d", len(attrs))
				}
				want := []attribute.KeyValue{
					attribute.String("http.request.method", "GET"),
					attribute.String("url.scheme", "http"),
					attribute.String("server.address", "example.com"),
					attribute.String("network.protocol.name", "http"),
					attribute.String("network.protocol.version", "1.1"),
					attribute.String("http.route", "/path/abc123"),
				}
				if diff := cmp.Diff(want, attrs, cmpopts.IgnoreUnexported(attribute.Value{}), cmpopts.SortSlices(func(a, b attribute.KeyValue) bool {
					return a.Key < b.Key
				})); diff != "" {
					t.Errorf("MetricAttributes() mismatch (-want +got):\n%s", diff)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HTTPServer{}.MetricAttributes(tt.server, tt.req, tt.statusCode, tt.route, tt.additionalAttributes)
			tt.wantFunc(t, got)
		})
	}
}

func TestNewMethod(t *testing.T) {
	testCases := []struct {
		method   string
		n        int
		want     attribute.KeyValue
		wantOrig attribute.KeyValue
	}{
		{
			method: http.MethodPost,
			n:      1,
			want:   attribute.String("http.request.method", "POST"),
		},
		{
			method:   "Put",
			n:        2,
			want:     attribute.String("http.request.method", "PUT"),
			wantOrig: attribute.String("http.request.method_original", "Put"),
		},
		{
			method:   "Unknown",
			n:        2,
			want:     attribute.String("http.request.method", "GET"),
			wantOrig: attribute.String("http.request.method_original", "Unknown"),
		},
	}

	for _, tt := range testCases {
		t.Run(tt.method, func(t *testing.T) {
			got, gotOrig := HTTPServer{}.method(tt.method)
			if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreUnexported(attribute.Value{})); diff != "" {
				t.Errorf("method() result mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantOrig, gotOrig, cmpopts.IgnoreUnexported(attribute.Value{})); diff != "" {
				t.Errorf("method() original mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRequestTraceAttrs_HTTPRoute(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		wantRoute string
	}{
		{
			name:      "only path",
			pattern:   "/path/{id}",
			wantRoute: "/path/{id}",
		},
		{
			name:      "with method",
			pattern:   "GET /path/{id}",
			wantRoute: "/path/{id}",
		},
		{
			name:      "with domain",
			pattern:   "example.com/path/{id}",
			wantRoute: "/path/{id}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/path/abc123", http.NoBody)
			req.Pattern = tt.pattern

			attrs := (HTTPServer{}).RequestTraceAttrs("", req, RequestTraceAttrsOpts{})

			var gotRoute string
			for _, attr := range attrs {
				if attr.Key == "http.route" {
					gotRoute = attr.Value.AsString()
					break
				}
			}
			if diff := cmp.Diff(tt.wantRoute, gotRoute); diff != "" {
				t.Errorf("http.route mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRequestTraceAttrs_ClientIP(t *testing.T) {
	for _, tt := range []struct {
		name              string
		requestModifierFn func(r *http.Request)
		requestTraceOpts  RequestTraceAttrsOpts

		wantClientIP string
	}{
		{
			name:         "with a client IP from the network",
			wantClientIP: "1.2.3.4",
		},
		{
			name: "with a client IP from x-forwarded-for header",
			requestModifierFn: func(r *http.Request) {
				r.Header.Add("X-Forwarded-For", "5.6.7.8")
			},
			wantClientIP: "5.6.7.8",
		},
		{
			name: "with a client IP in options",
			requestModifierFn: func(r *http.Request) {
				r.Header.Add("X-Forwarded-For", "5.6.7.8")
			},
			requestTraceOpts: RequestTraceAttrsOpts{
				HTTPClientIP: "9.8.7.6",
			},
			wantClientIP: "9.8.7.6",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/example", http.NoBody)
			req.RemoteAddr = "1.2.3.4:5678"

			if tt.requestModifierFn != nil {
				tt.requestModifierFn(req)
			}

			var found bool
			for _, attr := range (HTTPServer{}).RequestTraceAttrs("", req, tt.requestTraceOpts) {
				if attr.Key != "client.address" {
					continue
				}
				found = true
				if diff := cmp.Diff(tt.wantClientIP, attr.Value.AsString()); diff != "" {
					t.Errorf("client.address mismatch (-want +got):\n%s", diff)
				}
			}
			if !found {
				t.Error("client.address attribute not found")
			}
		})
	}
}
