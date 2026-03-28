// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package otelchi_test

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	chi "github.com/go-chi/chi/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/zchee/otelchi"
)

func TestDefaultTrace(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	router := chi.NewRouter()
	router.Use(otelchi.Middleware("foobar", otelchi.WithTracerProvider(provider)))

	router.HandleFunc("/user/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest(http.MethodGet, "/user/123", http.NoBody)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, r)

	if got, want := w.Code, http.StatusOK; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}

	spans := sr.Ended()

	if diff := cmp.Diff(len(sr.Ended()), 1); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
	span := spans[0]
	attr := span.Attributes()
	if diff := cmp.Diff(ensurePrefix(http.MethodGet, spans[0].Name()), true); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
	if diff := cmp.Diff("GET /user/{id}", span.Name()); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
	if diff := cmp.Diff(trace.SpanKindServer, span.SpanKind()); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
	if !slices.Contains(attr, attribute.Int("http.response.status_code", http.StatusOK)) {
		t.Fatalf("want contain %v but %v", attribute.Int("http.response.status_code", http.StatusOK), attr)
	}
	if !slices.Contains(attr, attribute.String("http.request.method", "GET")) {
		t.Fatalf("want contain %v but %v", attribute.Int("http.response.status_code", http.StatusOK), attr)
	}
	if !slices.Contains(attr, attribute.String("http.route", "/user/{id}")) {
		t.Fatalf("want contain %v but %v", attribute.Int("http.response.status_code", http.StatusOK), attr)
	}
	if diff := cmp.Diff(codes.Unset, span.Status().Code); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
	if span.Status().Description != "" {
		t.Fatal("expected empty")
	}
}

func TestCustomSpanNameFormatter(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()

	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))

	routeTpl := "/user/{id}"

	testdata := []struct {
		spanNameFormatter func(string, *http.Request) string
		want              string
	}{
		{nil, setDefaultName(http.MethodGet, routeTpl)},
		{
			func(string, *http.Request) string { return "custom" },
			"custom",
		},
		{
			func(name string, r *http.Request) string {
				return fmt.Sprintf("%s %s", r.Method, name)
			},
			"GET " + routeTpl,
		},
	}

	for i, d := range testdata {
		t.Run(fmt.Sprintf("%d_%s", i, d.want), func(t *testing.T) {
			router := chi.NewRouter()
			router.Use(otelchi.Middleware(
				"foobar",
				otelchi.WithTracerProvider(tp),
				otelchi.WithSpanNameFormatter(d.spanNameFormatter),
			))
			router.HandleFunc(routeTpl, func(http.ResponseWriter, *http.Request) {})

			r := httptest.NewRequest(http.MethodGet, "/user/123", http.NoBody)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, r)

			spans := exporter.GetSpans()
			if diff := cmp.Diff(len(spans), 1); diff != "" {
				t.Fatalf("(+want, -got)\n%s", diff)
			}
			if diff := cmp.Diff(d.want, spans[0].Name); diff != "" {
				t.Fatalf("(+want, -got)\n%s", diff)
			}

			exporter.Reset()
		})
	}
}

func ok(http.ResponseWriter, *http.Request) {}
func notfound(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not found", http.StatusNotFound)
}

func TestSDKIntegration(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider()
	provider.RegisterSpanProcessor(sr)

	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	router := chi.NewRouter()
	router.Use(otelchi.Middleware("foobar",
		otelchi.WithTracerProvider(provider),
		otelchi.WithMeterProvider(meterProvider)))

	router.HandleFunc("GET /user/{id:[0-9]+}", ok)
	router.HandleFunc("/book/{title}", ok)

	tests := []struct {
		name         string
		method       string
		path         string
		reqFunc      func(r *http.Request)
		wantSpanName string
		wantMethod   string
		wantRoute    string
		wantStatus   int
	}{
		{
			name:         "user route",
			method:       http.MethodGet,
			path:         "/user/123",
			reqFunc:      nil,
			wantSpanName: "GET /user/{id:[0-9]+}",
			wantMethod:   http.MethodGet,
			wantRoute:    "/user/{id:[0-9]+}",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "POST book route",
			method:       http.MethodPost,
			path:         "/book/foo",
			reqFunc:      nil,
			wantSpanName: "POST /book/{title}",
			wantMethod:   http.MethodPost,
			wantRoute:    "/book/{title}",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "book route with custom pattern",
			method:       http.MethodGet,
			path:         "/book/bar",
			reqFunc:      func(r *http.Request) { r.Pattern = "/book/{custom}" },
			wantSpanName: "GET /book/{custom}",
			wantMethod:   http.MethodGet,
			wantRoute:    "/book/{custom}",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "Invalid HTTP Method",
			method:       "INVALID",
			path:         "/book/bar",
			reqFunc:      func(r *http.Request) { r.Pattern = "/book/{custom}" },
			wantSpanName: "HTTP /book/{custom}",
			wantMethod:   http.MethodGet,
			wantRoute:    "/book/{custom}",
			wantStatus:   http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer sr.Reset()

			r := httptest.NewRequest(tt.method, tt.path, http.NoBody)
			if tt.reqFunc != nil {
				tt.reqFunc(r)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			spans := sr.Ended()

			if diff := cmp.Diff(len(spans), 1); diff != "" {
				t.Fatalf("(+want, -got)\n%s", diff)
			}
			assertSpan(t, sr.Ended()[0],
				tt.wantSpanName,
				trace.SpanKindServer,
				attribute.String("server.address", "foobar"),
				attribute.Int("http.response.status_code", tt.wantStatus),
				attribute.String("http.request.method", tt.wantMethod),
				attribute.String("http.route", tt.wantRoute),
			)
		})
	}
}

func TestNotFoundIsNotError(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider()
	provider.RegisterSpanProcessor(sr)

	router := chi.NewRouter()
	router.Use(otelchi.Middleware("foobar", otelchi.WithTracerProvider(provider)))
	router.HandleFunc("GET /does/not/exist", notfound)

	r0 := httptest.NewRequest(http.MethodGet, "/does/not/exist", http.NoBody)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r0)

	if diff := cmp.Diff(len(sr.Ended()), 1); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
	assertSpan(t, sr.Ended()[0],
		"GET /does/not/exist",
		trace.SpanKindServer,
		attribute.String("server.address", "foobar"),
		attribute.Int("http.response.status_code", http.StatusNotFound),
		attribute.String("http.request.method", "GET"),
		attribute.String("http.route", "/does/not/exist"),
	)
	if diff := cmp.Diff(codes.Unset, sr.Ended()[0].Status().Code); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
}

func assertSpan(t *testing.T, span sdktrace.ReadOnlySpan, name string, kind trace.SpanKind, attrs ...attribute.KeyValue) {
	t.Helper()

	if diff := cmp.Diff(name, span.Name()); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
	if diff := cmp.Diff(kind, span.SpanKind()); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}

	got := make(map[attribute.Key]attribute.Value, len(span.Attributes()))
	for _, a := range span.Attributes() {
		got[a.Key] = a.Value
	}
	for _, want := range attrs {
		if !slices.Contains(slices.Sorted(maps.Keys(got)), want.Key) {
			continue
		}
		if diff := cmp.Diff(want.Value, got[want.Key], cmpopts.EquateComparable(attribute.Value{})); diff != "" {
			t.Fatalf("(+want, -got)\n%s", diff)
		}
	}
}

func TestWithPublicEndpoint(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider()
	provider.RegisterSpanProcessor(sr)

	remoteSpan := trace.SpanContextConfig{
		TraceID: trace.TraceID{0x01},
		SpanID:  trace.SpanID{0x01},
		Remote:  true,
	}
	prop := propagation.TraceContext{}

	router := chi.NewRouter()
	router.Use(otelchi.Middleware("foobar",
		otelchi.WithPublicEndpoint(),
		otelchi.WithPropagators(prop),
		otelchi.WithTracerProvider(provider),
	))
	router.HandleFunc("/with/public/endpoint", func(_ http.ResponseWriter, r *http.Request) {
		s := trace.SpanFromContext(r.Context())
		sc := s.SpanContext()

		// Should be with new root trace.
		if diff := cmp.Diff(sc.IsValid(), true); diff != "" {
			t.Fatalf("(+want, -got)\n%s", diff)
		}
		if diff := cmp.Diff(sc.IsRemote(), false); diff != "" {
			t.Fatalf("(+want, -got)\n%s", diff)
		}
		if diff := cmp.Diff(remoteSpan.TraceID, sc.TraceID()); diff == "" {
			t.Fatalf("got %v want %v are not equal", sc.TraceID(), remoteSpan.TraceID)
		}
	})

	r0 := httptest.NewRequest(http.MethodGet, "/with/public/endpoint", http.NoBody)
	w := httptest.NewRecorder()

	sc := trace.NewSpanContext(remoteSpan)
	ctx := trace.ContextWithSpanContext(t.Context(), sc)
	prop.Inject(ctx, propagation.HeaderCarrier(r0.Header))

	router.ServeHTTP(w, r0)
	if diff := cmp.Diff(http.StatusOK, w.Result().StatusCode); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}

	// Recorded span should be linked with an incoming span context.
	if err := sr.ForceFlush(ctx); err != nil {
		t.Fatal(err)
	}
	done := sr.Ended()
	if diff := cmp.Diff(len(done), 1); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
	if diff := cmp.Diff(len(done[0].Links()), 1); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
	if diff := cmp.Diff(sc.Equal(done[0].Links()[0].SpanContext), true); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
}

func TestWithPublicEndpointFn(t *testing.T) {
	remoteSpan := trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x01},
		SpanID:     trace.SpanID{0x01},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	}
	prop := propagation.TraceContext{}

	testdata := []struct {
		name          string
		fn            func(*http.Request) bool
		handlerAssert func(*testing.T, trace.SpanContext)
		spansAssert   func(*testing.T, trace.SpanContext, []sdktrace.ReadOnlySpan)
	}{
		{
			name: "with the method returning true",
			fn: func(*http.Request) bool {
				return true
			},
			handlerAssert: func(t *testing.T, sc trace.SpanContext) {
				// Should be with new root trace.
				if diff := cmp.Diff(sc.IsValid(), true); diff != "" {
					t.Fatalf("(+want, -got)\n%s", diff)
				}
				if diff := cmp.Diff(sc.IsRemote(), false); diff != "" {
					t.Fatalf("(+want, -got)\n%s", diff)
				}
				if diff := cmp.Diff(remoteSpan.TraceID, sc.TraceID()); diff == "" {
					t.Fatalf("got %v want %v are not equal", sc.TraceID(), remoteSpan.TraceID)
				}
			},
			spansAssert: func(t *testing.T, sc trace.SpanContext, spans []sdktrace.ReadOnlySpan) {
				if diff := cmp.Diff(len(spans), 1); diff != "" {
					t.Fatalf("(+want, -got)\n%s", diff)
				}
				if diff := cmp.Diff(len(spans[0].Links()), 1); diff != "" {
					t.Fatalf("(+want, -got)\n%s", diff)
				}
				if diff := cmp.Diff(sc.Equal(spans[0].Links()[0].SpanContext), true); diff != "" {
					t.Fatalf("(+want, -got)\n%s", diff)
				}
			},
		},
		{
			name: "with the method returning false",
			fn: func(*http.Request) bool {
				return false
			},
			handlerAssert: func(t *testing.T, sc trace.SpanContext) {
				// Should have remote span as parent
				if diff := cmp.Diff(sc.IsValid(), true); diff != "" {
					t.Fatalf("(+want, -got)\n%s", diff)
				}
				if diff := cmp.Diff(sc.IsRemote(), false); diff != "" {
					t.Fatalf("(+want, -got)\n%s", diff)
				}
				if diff := cmp.Diff(remoteSpan.TraceID, sc.TraceID()); diff != "" {
					t.Fatalf("(+want, -got)\n%s", diff)
				}
			},
			spansAssert: func(t *testing.T, _ trace.SpanContext, spans []sdktrace.ReadOnlySpan) {
				if diff := cmp.Diff(len(spans), 1); diff != "" {
					t.Fatalf("(+want, -got)\n%s", diff)
				}
				if spans[0].Links() != nil {
					t.Fatal("expected empty")
				}
			},
		},
	}

	for _, tt := range testdata {
		t.Run(tt.name, func(t *testing.T) {
			sr := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider()
			provider.RegisterSpanProcessor(sr)

			router := chi.NewRouter()
			router.Use(otelchi.Middleware("foobar",
				otelchi.WithPublicEndpointFn(tt.fn),
				otelchi.WithPropagators(prop),
				otelchi.WithTracerProvider(provider),
			))
			router.HandleFunc("/with/public/endpointfn", func(_ http.ResponseWriter, r *http.Request) {
				s := trace.SpanFromContext(r.Context())
				tt.handlerAssert(t, s.SpanContext())
			})

			r0 := httptest.NewRequest(http.MethodGet, "/with/public/endpointfn", http.NoBody)
			w := httptest.NewRecorder()

			sc := trace.NewSpanContext(remoteSpan)
			ctx := trace.ContextWithSpanContext(t.Context(), sc)
			prop.Inject(ctx, propagation.HeaderCarrier(r0.Header))

			router.ServeHTTP(w, r0)
			if diff := cmp.Diff(http.StatusOK, w.Result().StatusCode); diff != "" {
				t.Fatalf("(+want, -got)\n%s", diff)
			}

			// Recorded span should be linked with an incoming span context.
			if err := sr.ForceFlush(ctx); err != nil {
				t.Fatal(err)
			}
			spans := sr.Ended()
			tt.spansAssert(t, sc, spans)
		})
	}
}

func TestDefaultMetricAttributes(t *testing.T) {
	defaultMetricAttributes := []attribute.KeyValue{
		attribute.String("http.route", "/user/{id:[0-9]+}"),
		attribute.String("server.address", "foobar"),
	}

	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	router := chi.NewRouter()
	router.Use(otelchi.Middleware("foobar",
		otelchi.WithMeterProvider(meterProvider),
	))

	router.HandleFunc("GET /user/{id:[0-9]+}", ok)
	r, err := http.NewRequest(http.MethodGet, "http://localhost/user/123", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, r)

	rm := metricdata.ResourceMetrics{}
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(len(rm.ScopeMetrics), 1); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
	if diff := cmp.Diff(len(rm.ScopeMetrics[0].Metrics), 3); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}

	matchedMetrics := 0
	for _, m := range rm.ScopeMetrics[0].Metrics {
		switch d := m.Data.(type) {
		case metricdata.Histogram[int64]:
			if diff := cmp.Diff(len(d.DataPoints), 1); diff != "" {
				t.Fatalf("(+want, -got)\n%s", diff)
			}
			containsAttributes(t, d.DataPoints[0].Attributes, defaultMetricAttributes)
			matchedMetrics++
		case metricdata.Histogram[float64]:
			if diff := cmp.Diff(len(d.DataPoints), 1); diff != "" {
				t.Fatalf("(+want, -got)\n%s", diff)
			}
			containsAttributes(t, d.DataPoints[0].Attributes, defaultMetricAttributes)
			matchedMetrics++
		default:
			t.Fatalf("unexpected metric type %T", m.Data)
		}
	}
	if diff := cmp.Diff(matchedMetrics, 3); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
}

func TestHandlerWithMetricAttributesFn(t *testing.T) {
	const (
		serverRequestSize  = "http.server.request.body.size"
		serverResponseSize = "http.server.response.body.size"
		serverDuration     = "http.server.request.duration"
	)
	testCases := []struct {
		name                    string
		fn                      func(r *http.Request) []attribute.KeyValue
		wantAdditionalAttribute []attribute.KeyValue
	}{
		{
			name:                    "With a nil function",
			fn:                      nil,
			wantAdditionalAttribute: []attribute.KeyValue{},
		},
		{
			name: "With a function that returns an additional attribute",
			fn: func(*http.Request) []attribute.KeyValue {
				return []attribute.KeyValue{
					attribute.String("fooKey", "fooValue"),
					attribute.String("barKey", "barValue"),
				}
			},
			wantAdditionalAttribute: []attribute.KeyValue{
				attribute.String("fooKey", "fooValue"),
				attribute.String("barKey", "barValue"),
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

			router := chi.NewRouter()
			router.Use(otelchi.Middleware("foobar",
				otelchi.WithMeterProvider(meterProvider),
				otelchi.WithMetricAttributesFn(tc.fn),
			))

			router.HandleFunc("/user/{id:[0-9]+}", ok)
			r, err := http.NewRequest(http.MethodGet, "http://localhost/user/123", http.NoBody)
			if err != nil {
				t.Fatal(err)
			}
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, r)

			rm := metricdata.ResourceMetrics{}
			err = reader.Collect(t.Context(), &rm)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(len(rm.ScopeMetrics), 1); diff != "" {
				t.Fatalf("(+want, -got)\n%s", diff)
			}
			if diff := cmp.Diff(len(rm.ScopeMetrics[0].Metrics), 3); diff != "" {
				t.Fatalf("(+want, -got)\n%s", diff)
			}

			// Verify that the additional attribute is present in the metrics.
			matchedMetrics := 0
			for _, m := range rm.ScopeMetrics[0].Metrics {
				switch m.Name {
				case serverRequestSize, serverResponseSize:
					d, ok := m.Data.(metricdata.Histogram[int64])
					if diff := cmp.Diff(ok, true); diff != "" {
						t.Fatalf("(+want, -got)\n%s", diff)
					}
					if diff := cmp.Diff(len(d.DataPoints), 1); diff != "" {
						t.Fatalf("(+want, -got)\n%s", diff)
					}
					containsAttributes(t, d.DataPoints[0].Attributes, tc.wantAdditionalAttribute)
					matchedMetrics++
				case serverDuration:
					d, ok := m.Data.(metricdata.Histogram[float64])
					if diff := cmp.Diff(ok, true); diff != "" {
						t.Fatalf("(+want, -got)\n%s", diff)
					}
					if diff := cmp.Diff(len(d.DataPoints), 1); diff != "" {
						t.Fatalf("(+want, -got)\n%s", diff)
					}
					containsAttributes(t, d.DataPoints[0].Attributes, tc.wantAdditionalAttribute)
					matchedMetrics++
				default:
					t.Fatalf("unexpected metric name %q", m.Name)
				}
			}
			if diff := cmp.Diff(matchedMetrics, 3); diff != "" {
				t.Fatalf("(+want, -got)\n%s", diff)
			}
		})
	}
}

func TestDefaultTraceWithNestedRoute(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	router := chi.NewRouter()
	router.Use(otelchi.Middleware("foobar", otelchi.WithTracerProvider(provider)))

	router.Route("/api", func(r chi.Router) {
		r.Route("/users", func(r chi.Router) {
			r.Get("/{id}", ok)
		})
	})

	r := httptest.NewRequest(http.MethodGet, "/api/users/123", http.NoBody)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, r)

	spans := sr.Ended()
	if diff := cmp.Diff(len(spans), 1); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
	assertSpan(t, spans[0],
		"GET /api/users/{id}",
		trace.SpanKindServer,
		attribute.String("server.address", "foobar"),
		attribute.Int("http.response.status_code", http.StatusOK),
		attribute.String("http.request.method", "GET"),
		attribute.String("http.route", "/api/users/{id}"),
	)
}

func containsAttributes(t *testing.T, attrSet attribute.Set, expected []attribute.KeyValue) {
	for _, att := range expected {
		actualValue, ok := attrSet.Value(att.Key)
		if diff := cmp.Diff(ok, true); diff != "" {
			t.Fatalf("(+want, -got)\n%s", diff)
		}
		if diff := cmp.Diff(att.Value.AsString(), actualValue.AsString()); diff != "" {
			t.Fatalf("(+want, -got)\n%s", diff)
		}
	}
}

func setDefaultName(method, path string) string {
	return method + " " + path
}

func ensurePrefix(prefix, s string) bool {
	return strings.HasPrefix(s, prefix)
}
