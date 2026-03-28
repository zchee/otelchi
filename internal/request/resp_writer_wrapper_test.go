// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package request

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRespWriterWriteHeader(t *testing.T) {
	rw := NewRespWriterWrapper(&httptest.ResponseRecorder{}, func(int64) {})

	rw.WriteHeader(http.StatusTeapot)
	if diff := cmp.Diff(http.StatusTeapot, rw.statusCode); diff != "" {
		t.Errorf("statusCode mismatch (-want +got):\n%s", diff)
	}
	if !rw.wroteHeader {
		t.Error("expected wroteHeader to be true")
	}

	rw.WriteHeader(http.StatusGone)
	if diff := cmp.Diff(http.StatusTeapot, rw.statusCode); diff != "" {
		t.Errorf("statusCode mismatch after second WriteHeader (-want +got):\n%s", diff)
	}
}

func TestRespWriterWriteInformationalStatusCode(t *testing.T) {
	rw := NewRespWriterWrapper(&httptest.ResponseRecorder{}, func(int64) {})

	rw.WriteHeader(http.StatusContinue)
	if diff := cmp.Diff(http.StatusOK, rw.statusCode); diff != "" {
		t.Errorf("statusCode mismatch after informational WriteHeader (-want +got):\n%s", diff)
	}
	if rw.wroteHeader {
		t.Error("expected wroteHeader to be false after informational status")
	}

	rw.WriteHeader(http.StatusGone)
	if diff := cmp.Diff(http.StatusGone, rw.statusCode); diff != "" {
		t.Errorf("statusCode mismatch after final WriteHeader (-want +got):\n%s", diff)
	}
	if !rw.wroteHeader {
		t.Error("expected wroteHeader to be true after final status")
	}
}

func TestRespWriterFlush(t *testing.T) {
	rw := NewRespWriterWrapper(&httptest.ResponseRecorder{}, func(int64) {})

	rw.Flush()
	if diff := cmp.Diff(http.StatusOK, rw.statusCode); diff != "" {
		t.Errorf("statusCode mismatch (-want +got):\n%s", diff)
	}
	if !rw.wroteHeader {
		t.Error("expected wroteHeader to be true")
	}
}

type nonFlushableResponseWriter struct{}

func (nonFlushableResponseWriter) Header() http.Header {
	return http.Header{}
}

func (nonFlushableResponseWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (nonFlushableResponseWriter) WriteHeader(int) {}

func TestRespWriterFlushNoFlusher(t *testing.T) {
	rw := NewRespWriterWrapper(nonFlushableResponseWriter{}, func(int64) {})

	rw.Flush()
	if diff := cmp.Diff(http.StatusOK, rw.statusCode); diff != "" {
		t.Errorf("statusCode mismatch (-want +got):\n%s", diff)
	}
	if !rw.wroteHeader {
		t.Error("expected wroteHeader to be true")
	}
}

func TestConcurrentRespWriterWrapper(t *testing.T) {
	rw := NewRespWriterWrapper(&httptest.ResponseRecorder{}, func(int64) {})

	go func() {
		_, _ = rw.Write([]byte("hello world"))
	}()

	if rw.BytesWritten() == 0 {
		// This is actually acceptable in concurrent scenarios
		// Just checking it doesn't panic
	}
	if rw.StatusCode() == 0 {
		// This is actually acceptable in concurrent scenarios
		// Just checking it doesn't panic
	}
	if err := rw.Error(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
