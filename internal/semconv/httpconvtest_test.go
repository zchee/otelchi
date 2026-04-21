// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package semconv_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/metric/metricdata/metricdatatest"

	"github.com/zchee/otelchi/internal/semconv"
)

func TestNewTraceRequest(t *testing.T) {
	serv := semconv.NewHTTPServer(nil)
	want := func(req testServerReq) []attribute.KeyValue {
		return []attribute.KeyValue{
			attribute.String("http.request.method", "GET"),
			attribute.String("url.scheme", "http"),
			attribute.String("server.address", req.hostname),
			attribute.Int("server.port", req.serverPort),
			attribute.String("network.peer.address", req.peerAddr),
			attribute.Int("network.peer.port", req.peerPort),
			attribute.String("user_agent.original", "Go-http-client/1.1"),
			attribute.String("client.address", req.clientIP),
			attribute.String("network.protocol.version", "1.1"),
			attribute.String("url.path", "/"),
		}
	}
	testTraceRequest(t, serv, want)
}

func TestNewServerRecordMetrics(t *testing.T) {
	oldAttrs := attribute.NewSet(
		attribute.String("http.scheme", "http"),
		attribute.String("http.method", "POST"),
		attribute.Int64("http.status_code", 301),
		attribute.String("key", "value"),
		attribute.String("net.host.name", "stuff"),
		attribute.String("net.protocol.name", "http"),
		attribute.String("net.protocol.version", "1.1"),
	)

	currAttrs := attribute.NewSet(
		attribute.String("http.request.method", "POST"),
		attribute.Int64("http.response.status_code", 301),
		attribute.String("key", "value"),
		attribute.String("network.protocol.name", "http"),
		attribute.String("network.protocol.version", "1.1"),
		attribute.String("server.address", "stuff"),
		attribute.String("url.scheme", "http"),
	)

	// the HTTPServer version
	expectedCurrentScopeMetric := metricdata.ScopeMetrics{
		Scope: instrumentation.Scope{
			Name: "test",
		},
		Metrics: []metricdata.Metrics{
			{
				Name:        "http.server.request.body.size",
				Description: "Size of HTTP server request bodies.",
				Unit:        "By",
				Data: metricdata.Histogram[int64]{
					Temporality: metricdata.CumulativeTemporality,
					DataPoints: []metricdata.HistogramDataPoint[int64]{
						{
							Attributes: currAttrs,
						},
					},
				},
			},
			{
				Name:        "http.server.response.body.size",
				Description: "Size of HTTP server response bodies.",
				Unit:        "By",
				Data: metricdata.Histogram[int64]{
					Temporality: metricdata.CumulativeTemporality,
					DataPoints: []metricdata.HistogramDataPoint[int64]{
						{
							Attributes: currAttrs,
						},
					},
				},
			},
			{
				Name:        "http.server.request.duration",
				Description: "Duration of HTTP server requests.",
				Unit:        "s",
				Data: metricdata.Histogram[float64]{
					Temporality: metricdata.CumulativeTemporality,
					DataPoints: []metricdata.HistogramDataPoint[float64]{
						{
							Attributes: currAttrs,
						},
					},
				},
			},
		},
	}

	// The OldHTTPServer version
	expectedOldScopeMetric := expectedCurrentScopeMetric
	expectedOldScopeMetric.Metrics = append(expectedOldScopeMetric.Metrics, []metricdata.Metrics{
		{
			Name:        "http.server.request.size",
			Description: "Measures the size of HTTP request messages.",
			Unit:        "By",
			Data: metricdata.Sum[int64]{
				Temporality: metricdata.CumulativeTemporality,
				IsMonotonic: true,
				DataPoints: []metricdata.DataPoint[int64]{
					{
						Attributes: oldAttrs,
					},
				},
			},
		},
		{
			Name:        "http.server.response.size",
			Description: "Measures the size of HTTP response messages.",
			Unit:        "By",
			Data: metricdata.Sum[int64]{
				Temporality: metricdata.CumulativeTemporality,
				IsMonotonic: true,
				DataPoints: []metricdata.DataPoint[int64]{
					{
						Attributes: oldAttrs,
					},
				},
			},
		},
		{
			Name:        "http.server.duration",
			Description: "Measures the duration of inbound HTTP requests.",
			Unit:        "ms",
			Data: metricdata.Histogram[float64]{
				Temporality: metricdata.CumulativeTemporality,
				DataPoints: []metricdata.HistogramDataPoint[float64]{
					{
						Attributes: oldAttrs,
					},
				},
			},
		},
	}...)

	tests := []struct {
		name       string
		serverFunc func(metric.MeterProvider) semconv.HTTPServer
		wantFunc   func(t *testing.T, rm metricdata.ResourceMetrics)
	}{
		{
			name: "No Meter",
			serverFunc: func(metric.MeterProvider) semconv.HTTPServer {
				return semconv.NewHTTPServer(nil)
			},
			wantFunc: func(t *testing.T, rm metricdata.ResourceMetrics) {
				if len(rm.ScopeMetrics) != 0 {
					t.Errorf("expected empty ScopeMetrics, got %d items", len(rm.ScopeMetrics))
				}
			},
		},
		{
			name: "With Meter",
			serverFunc: func(mp metric.MeterProvider) semconv.HTTPServer {
				return semconv.NewHTTPServer(mp.Meter("test"))
			},
			wantFunc: func(t *testing.T, rm metricdata.ResourceMetrics) {
				if len(rm.ScopeMetrics) != 1 {
					t.Fatalf("expected 1 ScopeMetrics, got %d", len(rm.ScopeMetrics))
				}

				// because of OldHTTPServer
				if len(rm.ScopeMetrics[0].Metrics) != 3 {
					t.Fatalf("expected 3 Metrics, got %d", len(rm.ScopeMetrics[0].Metrics))
				}
				metricdatatest.AssertEqual(t, expectedCurrentScopeMetric, rm.ScopeMetrics[0], metricdatatest.IgnoreTimestamp(), metricdatatest.IgnoreValue())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

			server := tt.serverFunc(mp)
			req, err := http.NewRequest("POST", "http://example.com", http.NoBody)
			if err != nil {
				t.Errorf("unexpected error creating request: %v", err)
			}

			ctx := t.Context()
			server.RecordMetrics(ctx, semconv.ServerMetricData{
				ServerName:   "stuff",
				ResponseSize: 200,
				MetricAttributes: semconv.MetricAttributes{
					Req:        req,
					StatusCode: 301,
					AdditionalAttributes: []attribute.KeyValue{
						attribute.String("key", "value"),
					},
				},
				MetricData: semconv.MetricData{
					RequestSize:     100,
					RequestDuration: 300 * time.Millisecond,
				},
			})

			rm := metricdata.ResourceMetrics{}
			if err := reader.Collect(ctx, &rm); err != nil {
				t.Fatalf("failed to collect metrics: %v", err)
			}
			tt.wantFunc(t, rm)
		})
	}
}

func TestNewTraceResponse(t *testing.T) {
	testCases := []struct {
		name string
		resp semconv.ResponseTelemetry
		want []attribute.KeyValue
	}{
		{
			name: "empty",
			resp: semconv.ResponseTelemetry{},
			want: nil,
		},
		{
			name: "no errors",
			resp: semconv.ResponseTelemetry{
				StatusCode: 200,
				ReadBytes:  701,
				WriteBytes: 802,
			},
			want: []attribute.KeyValue{
				attribute.Int("http.request.body.size", 701),
				attribute.Int("http.response.body.size", 802),
				attribute.Int("http.response.status_code", 200),
			},
		},
		{
			name: "with errors",
			resp: semconv.ResponseTelemetry{
				StatusCode: 200,
				ReadBytes:  701,
				ReadError:  fmt.Errorf("read error"),
				WriteBytes: 802,
				WriteError: fmt.Errorf("write error"),
			},
			want: []attribute.KeyValue{
				attribute.Int("http.request.body.size", 701),
				attribute.Int("http.response.body.size", 802),
				attribute.Int("http.response.status_code", 200),
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			got := semconv.HTTPServer{}.ResponseTraceAttrs(tt.resp)
			if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreUnexported(attribute.Value{}), cmpopts.EquateEmpty(), cmpopts.SortSlices(func(a, b attribute.KeyValue) bool {
				return a.Key < b.Key
			})); diff != "" {
				t.Errorf("ResponseTraceAttrs() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewTraceRequest_Client(t *testing.T) {
	body := strings.NewReader("Hello, world!")
	url := "https://example.com:8888/foo/bar?stuff=morestuff"
	req := httptest.NewRequest("pOST", url, body)
	req.Header.Set("User-Agent", "go-test-agent")

	want := []attribute.KeyValue{
		attribute.String("http.request.method", "POST"),
		attribute.String("http.request.method_original", "pOST"),
		attribute.String("url.full", url),
		attribute.String("server.address", "example.com"),
		attribute.Int("server.port", 8888),
		attribute.String("network.protocol.version", "1.1"),
	}
	got := semconv.NewHTTPClient(nil).RequestTraceAttrs(req)
	if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(attribute.Value{}), cmpopts.SortSlices(func(a, b attribute.KeyValue) bool {
		return a.Key < b.Key
	})); diff != "" {
		t.Errorf("RequestTraceAttrs() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewTraceResponse_Client(t *testing.T) {
	testcases := []struct {
		resp http.Response
		want []attribute.KeyValue
	}{
		{resp: http.Response{StatusCode: 200, ContentLength: 123}, want: []attribute.KeyValue{attribute.Int("http.response.status_code", 200)}},
		{resp: http.Response{StatusCode: 404, ContentLength: 0}, want: []attribute.KeyValue{attribute.Int("http.response.status_code", 404), attribute.String("error.type", "404")}},
	}

	for _, tt := range testcases {
		got := semconv.NewHTTPClient(nil).ResponseTraceAttrs(&tt.resp)
		if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreUnexported(attribute.Value{}), cmpopts.SortSlices(func(a, b attribute.KeyValue) bool {
			return a.Key < b.Key
		})); diff != "" {
			t.Errorf("ResponseTraceAttrs() mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestClientRequest(t *testing.T) {
	body := strings.NewReader("Hello, world!")
	url := "https://example.com:8888/foo/bar?stuff=morestuff"
	req := httptest.NewRequest("pOST", url, body)
	req.Header.Set("User-Agent", "go-test-agent")

	want := []attribute.KeyValue{
		attribute.String("http.request.method", "POST"),
		attribute.String("http.request.method_original", "pOST"),
		attribute.String("url.full", url),
		attribute.String("server.address", "example.com"),
		attribute.Int("server.port", 8888),
		attribute.String("network.protocol.version", "1.1"),
	}
	got := semconv.HTTPClient{}.RequestTraceAttrs(req)
	if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(attribute.Value{}), cmpopts.SortSlices(func(a, b attribute.KeyValue) bool {
		return a.Key < b.Key
	})); diff != "" {
		t.Errorf("RequestTraceAttrs() mismatch (-want +got):\n%s", diff)
	}
}

func TestClientResponse(t *testing.T) {
	testcases := []struct {
		resp http.Response
		want []attribute.KeyValue
	}{
		{resp: http.Response{StatusCode: 200, ContentLength: 123}, want: []attribute.KeyValue{attribute.Int("http.response.status_code", 200)}},
		{resp: http.Response{StatusCode: 404, ContentLength: 0}, want: []attribute.KeyValue{attribute.Int("http.response.status_code", 404), attribute.String("error.type", "404")}},
	}

	for _, tt := range testcases {
		got := semconv.HTTPClient{}.ResponseTraceAttrs(&tt.resp)
		if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreUnexported(attribute.Value{}), cmpopts.SortSlices(func(a, b attribute.KeyValue) bool {
			return a.Key < b.Key
		})); diff != "" {
			t.Errorf("ResponseTraceAttrs() mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestNewClientRecordMetrics(t *testing.T) {
	currAttrs := attribute.NewSet(
		attribute.String("http.request.method", "POST"),
		attribute.Int64("http.response.status_code", 301),
		attribute.String("network.protocol.name", "http"),
		attribute.String("network.protocol.version", "1.1"),
		attribute.String("server.address", "example.com"),
		attribute.String("url.scheme", "http"),
	)

	// the HTTPClient version
	expectedCurrentScopeMetric := metricdata.ScopeMetrics{
		Scope: instrumentation.Scope{
			Name: "test",
		},
		Metrics: []metricdata.Metrics{
			{
				Name:        "http.client.request.body.size",
				Description: "Size of HTTP client request bodies.",
				Unit:        "By",
				Data: metricdata.Histogram[int64]{
					Temporality: metricdata.CumulativeTemporality,
					DataPoints: []metricdata.HistogramDataPoint[int64]{
						{
							Attributes: currAttrs,
						},
					},
				},
			},
			{
				Name:        "http.client.request.duration",
				Description: "Duration of HTTP client requests.",
				Unit:        "s",
				Data: metricdata.Histogram[float64]{
					Temporality: metricdata.CumulativeTemporality,
					DataPoints: []metricdata.HistogramDataPoint[float64]{
						{
							Attributes: currAttrs,
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name       string
		clientFunc func(metric.MeterProvider) semconv.HTTPClient
		wantFunc   func(t *testing.T, rm metricdata.ResourceMetrics)
	}{
		{
			name: "No environment variable set, and no Meter",
			clientFunc: func(metric.MeterProvider) semconv.HTTPClient {
				return semconv.NewHTTPClient(nil)
			},
			wantFunc: func(t *testing.T, rm metricdata.ResourceMetrics) {
				if len(rm.ScopeMetrics) != 0 {
					t.Errorf("expected empty ScopeMetrics, got %d items", len(rm.ScopeMetrics))
				}
			},
		},
		{
			name: "With Meter",
			clientFunc: func(mp metric.MeterProvider) semconv.HTTPClient {
				return semconv.NewHTTPClient(mp.Meter("test"))
			},
			wantFunc: func(t *testing.T, rm metricdata.ResourceMetrics) {
				if len(rm.ScopeMetrics) != 1 {
					t.Fatalf("expected 1 ScopeMetrics, got %d", len(rm.ScopeMetrics))
				}

				if len(rm.ScopeMetrics[0].Metrics) != 2 {
					t.Fatalf("expected 2 Metrics, got %d", len(rm.ScopeMetrics[0].Metrics))
				}
				metricdatatest.AssertEqual(t, expectedCurrentScopeMetric, rm.ScopeMetrics[0], metricdatatest.IgnoreTimestamp(), metricdatatest.IgnoreValue())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

			client := tt.clientFunc(mp)
			req, err := http.NewRequest("POST", "http://example.com", http.NoBody)
			if err != nil {
				t.Errorf("unexpected error creating request: %v", err)
			}

			ctx := t.Context()
			client.RecordMetrics(ctx, semconv.MetricData{
				RequestSize:     100,
				RequestDuration: 300 * time.Millisecond,
			}, client.MetricOptions(semconv.MetricAttributes{
				Req:        req,
				StatusCode: 301,
			}))

			rm := metricdata.ResourceMetrics{}
			if err := reader.Collect(ctx, &rm); err != nil {
				t.Fatalf("failed to collect metrics: %v", err)
			}
			tt.wantFunc(t, rm)
		})
	}
}

type customError struct{}

func (customError) Error() string {
	return "custom error"
}
