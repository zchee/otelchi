// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package semconv_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opentelemetry.io/otel/attribute"

	"github.com/zchee/otelchi/internal/semconv"
)

type testServerReq struct {
	hostname   string
	serverPort int
	peerAddr   string
	peerPort   int
	clientIP   string
}

func testTraceRequest(t *testing.T, serv semconv.HTTPServer, want func(testServerReq) []attribute.KeyValue) {
	t.Helper()

	got := make(chan *http.Request, 1)
	handler := func(w http.ResponseWriter, r *http.Request) {
		got <- r
		close(got)
		w.WriteHeader(http.StatusOK)
	}

	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	srvURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}
	srvPort, err := strconv.ParseInt(srvURL.Port(), 10, 32)
	if err != nil {
		t.Fatalf("failed to parse server port: %v", err)
	}

	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("failed to close response body: %v", err)
	}

	req := <-got
	peer, peerPort := semconv.SplitHostPort(req.RemoteAddr)

	const user = "alice"
	req.SetBasicAuth(user, "pswrd")

	const clientIP = "127.0.0.5"
	req.Header.Add("X-Forwarded-For", clientIP)

	srvReq := testServerReq{
		hostname:   srvURL.Hostname(),
		serverPort: int(srvPort),
		peerAddr:   peer,
		peerPort:   peerPort,
		clientIP:   clientIP,
	}

	attrs := serv.RequestTraceAttrs("", req, semconv.RequestTraceAttrsOpts{})
	wantAttrs := want(srvReq)
	if diff := cmp.Diff(wantAttrs, attrs, cmpopts.IgnoreUnexported(attribute.Value{}), cmpopts.SortSlices(func(a, b attribute.KeyValue) bool {
		return a.Key < b.Key
	})); diff != "" {
		t.Errorf("RequestTraceAttrs() mismatch (-want +got):\n%s", diff)
	}
}
