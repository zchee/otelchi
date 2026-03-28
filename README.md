# otelchi

`otelchi` instruments `github.com/go-chi/chi/v5` routers with OpenTelemetry
server spans and HTTP server metrics.

It is a chi-focused port of the upstream `otelmux` middleware. Use
`Middleware` to wrap a chi router, then configure tracing, propagation, public
endpoint behavior, filters, and metric attributes with options.

## Requirements

- Go 1.26 or newer
- `github.com/go-chi/chi/v5`
- An OpenTelemetry SDK configuration in your application

## Installation

```bash
go get github.com/zchee/otelchi
```

## Usage

```go
package main

import (
	"log"
	"net/http"

	chi "github.com/go-chi/chi/v5"
	"github.com/zchee/otelchi"
)

func main() {
	r := chi.NewRouter()
	r.Use(otelchi.Middleware("my-server"))

	r.Get("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		_, _ = w.Write([]byte("user " + id + "\n"))
	})

	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatal(err)
	}
}
```

`Middleware` uses the global OpenTelemetry tracer provider, meter provider,
and text map propagator unless you pass explicit options. Configure exporters,
resources, and sampling in your application as you normally would with the
OpenTelemetry Go SDK.

## Options

- `WithTracerProvider(trace.TracerProvider)` uses a specific tracer provider
  instead of the global one.
- `WithMeterProvider(metric.MeterProvider)` uses a specific meter provider
  instead of the global one.
- `WithPropagators(propagation.TextMapPropagator)` overrides request context
  extraction.
- `WithSpanNameFormatter(func(string, *http.Request) string)` customizes span
  names. The default format is `METHOD /route`.
- `WithFilter(Filter)` suppresses instrumentation for requests that do not pass
  the filter.
- `WithPublicEndpoint()` creates a new root span and links any incoming remote
  span context.
- `WithPublicEndpointFn(func(*http.Request) bool)` enables public-endpoint
  behavior conditionally per request.
- `WithMetricAttributesFn(func(*http.Request) []attribute.KeyValue)` appends
  request-derived attributes to emitted metrics.

## Behavior

- Span names use the matched chi route pattern when available.
- Server metrics include the resolved `http.route` attribute together with
  method, status, protocol, and server address attributes.
- Unknown or non-standard HTTP methods fall back to an `HTTP /route` span-name
  prefix.
- The middleware preserves optional `http.ResponseWriter` interfaces such as
  `http.Flusher`, `http.Hijacker`, `http.Pusher`, and `io.ReaderFrom`.

## Testing

```bash
go test ./...
```

## License

Apache-2.0
