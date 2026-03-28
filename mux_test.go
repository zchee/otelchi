// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package otelchi

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chi "github.com/go-chi/chi/v5"
	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var sc = trace.NewSpanContext(trace.SpanContextConfig{
	TraceID:    [16]byte{1},
	SpanID:     [8]byte{1},
	Remote:     true,
	TraceFlags: trace.FlagsSampled,
})

func TestPassthroughSpanFromGlobalTracer(t *testing.T) {
	var called bool
	router := chi.NewRouter()
	router.Use(Middleware("foobar"))
	// The default global TracerProvider provides "pass through" spans for any
	// span context in the incoming request context.
	router.HandleFunc("/user/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		got := trace.SpanFromContext(r.Context()).SpanContext()
		if diff := cmp.Diff(sc, got); diff != "" {
			t.Fatalf("(+want, -got)\n%s", diff)
		}
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/user/123", http.NoBody)
	r = r.WithContext(trace.ContextWithRemoteSpanContext(t.Context(), sc))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, r)
	if diff := cmp.Diff(called, true); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
}

func TestPropagationWithGlobalPropagators(t *testing.T) {
	defer func(p propagation.TextMapPropagator) {
		otel.SetTextMapPropagator(p)
	}(otel.GetTextMapPropagator())

	prop := propagation.TraceContext{}
	otel.SetTextMapPropagator(prop)

	r := httptest.NewRequest(http.MethodGet, "/user/123", http.NoBody)
	w := httptest.NewRecorder()

	ctx := trace.ContextWithRemoteSpanContext(t.Context(), sc)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(r.Header))

	var called bool
	router := chi.NewRouter()
	router.Use(Middleware("foobar"))
	router.HandleFunc("/user/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		span := trace.SpanFromContext(r.Context())
		if diff := cmp.Diff(sc, span.SpanContext()); diff != "" {
			t.Fatalf("(+want, -got)\n%s", diff)
		}
		w.WriteHeader(http.StatusOK)
	}))

	router.ServeHTTP(w, r)
	if diff := cmp.Diff(called, true); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
}

func TestPropagationWithCustomPropagators(t *testing.T) {
	prop := propagation.TraceContext{}

	r := httptest.NewRequest(http.MethodGet, "/user/123", http.NoBody)
	w := httptest.NewRecorder()

	ctx := trace.ContextWithRemoteSpanContext(t.Context(), sc)
	prop.Inject(ctx, propagation.HeaderCarrier(r.Header))

	var called bool
	router := chi.NewRouter()
	router.Use(Middleware("foobar", WithPropagators(prop)))
	router.HandleFunc("/user/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		span := trace.SpanFromContext(r.Context())
		if diff := cmp.Diff(sc, span.SpanContext()); diff != "" {
			t.Fatalf("(+want, -got)\n%s", diff)
		}
		w.WriteHeader(http.StatusOK)
	}))

	router.ServeHTTP(w, r)
	if diff := cmp.Diff(called, true); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
}

type testResponseWriter struct {
	writer http.ResponseWriter
}

func (rw *testResponseWriter) Header() http.Header {
	return rw.writer.Header()
}

func (rw *testResponseWriter) Write(b []byte) (int, error) {
	return rw.writer.Write(b)
}

func (rw *testResponseWriter) WriteHeader(statusCode int) {
	rw.writer.WriteHeader(statusCode)
}

// implement Hijacker.
func (*testResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}

// implement Pusher.
func (*testResponseWriter) Push(string, *http.PushOptions) error {
	return nil
}

// implement Flusher.
func (*testResponseWriter) Flush() {
}

// implement io.ReaderFrom.
func (*testResponseWriter) ReadFrom(io.Reader) (int64, error) {
	return 0, nil
}

func TestResponseWriterInterfaces(t *testing.T) {
	// make sure the recordingResponseWriter preserves interfaces implemented by the wrapped writer
	router := chi.NewRouter()
	router.Use(Middleware("foobar"))
	router.HandleFunc("/user/{id}", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, ok := w.(http.Hijacker); !ok {
			t.Fatalf("%v not Implements http.Hijacker", w)
		}
		if _, ok := w.(http.Pusher); !ok {
			t.Fatalf("%v not Implements http.Pusher", w)
		}
		if _, ok := w.(http.Flusher); !ok {
			t.Fatalf("%v not Implements http.Flusher", w)
		}
		if _, ok := w.(io.ReaderFrom); !ok {
			t.Fatalf("%v not Implements io.ReaderFrom", w)
		}
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/user/123", http.NoBody)
	w := &testResponseWriter{
		writer: httptest.NewRecorder(),
	}

	router.ServeHTTP(w, r)
}

func TestFilter(t *testing.T) {
	prop := propagation.TraceContext{}

	router := chi.NewRouter()
	var calledHealth, calledTest int
	router.Use(Middleware("foobar", WithFilter(func(r *http.Request) bool {
		return r.URL.Path != "/health"
	})))
	router.HandleFunc("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledHealth++
		span := trace.SpanFromContext(r.Context())
		if diff := cmp.Diff(sc, span.SpanContext()); diff == "" {
			t.Fatalf("got %v want %v are not equal", sc, span.SpanContext())
		}
		w.WriteHeader(http.StatusOK)
	}))
	router.HandleFunc("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledTest++
		span := trace.SpanFromContext(r.Context())
		if diff := cmp.Diff(sc, span.SpanContext()); diff != "" {
			t.Fatalf("(+want, -got)\n%s", diff)
		}
		w.WriteHeader(http.StatusOK)
	}))

	ctx := t.Context()
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/health", http.NoBody)
	ctx = trace.ContextWithRemoteSpanContext(ctx, sc)
	prop.Inject(ctx, propagation.HeaderCarrier(r.Header))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)

	r = httptest.NewRequestWithContext(ctx, http.MethodGet, "/test", http.NoBody)
	ctx = trace.ContextWithRemoteSpanContext(ctx, sc)
	prop.Inject(ctx, propagation.HeaderCarrier(r.Header))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, r)

	if diff := cmp.Diff(1, calledHealth); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
	if diff := cmp.Diff(1, calledTest); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
}

func TestPassthroughSpanFromGlobalTracerWithBody(t *testing.T) {
	expectedBody := `{"message":"successfully"}`
	router := chi.NewRouter()
	router.Use(Middleware("foobar"))

	var called bool
	router.HandleFunc("POST /user", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		got := trace.SpanFromContext(r.Context()).SpanContext()
		if diff := cmp.Diff(sc, got); diff != "" {
			t.Fatalf("(+want, -got)\n%s", diff)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()

		if diff := cmp.Diff(`{"name":"John Doe","age":30}`, string(body)); diff != "" {
			t.Fatalf("(+want, -got)\n%s", diff)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, err = w.Write([]byte(expectedBody))
		if err != nil {
			t.Fatal(err)
		}
	}))

	r := httptest.NewRequest(http.MethodPost, "/user", strings.NewReader(`{"name":"John Doe","age":30}`))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(trace.ContextWithRemoteSpanContext(t.Context(), sc))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, r)

	// Validate the assertions
	if diff := cmp.Diff(called, true); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
	if diff := cmp.Diff(http.StatusCreated, w.Code); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
	if diff := cmp.Diff(expectedBody, w.Body.String()); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
}

func TestHeaderAlreadyWrittenWhenFlushing(t *testing.T) {
	var called bool

	router := chi.NewRouter()
	router.Use(Middleware("foobar"))

	router.HandleFunc("/user/{id}", func(w http.ResponseWriter, _ *http.Request) {
		called = true

		w.WriteHeader(http.StatusBadRequest)
		f := w.(http.Flusher)
		f.Flush()
	})

	r := httptest.NewRequest(http.MethodGet, "/user/123", http.NoBody)
	r = r.WithContext(trace.ContextWithRemoteSpanContext(t.Context(), sc))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, r)

	// Assertions
	if diff := cmp.Diff(called, true); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
	if diff := cmp.Diff(http.StatusBadRequest, w.Code); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
}
