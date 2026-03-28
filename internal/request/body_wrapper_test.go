// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package request

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

var errFirstCall = errors.New("first call")

func TestBodyWrapper(t *testing.T) {
	bw := NewBodyWrapper(io.NopCloser(strings.NewReader("hello world")), func(int64) {})

	data, err := io.ReadAll(bw)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff("hello world", string(data)); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}

	if diff := cmp.Diff(int64(11), bw.BytesRead()); diff != "" {
		t.Fatalf("(+want, -got)\n%s", diff)
	}
	if !errors.Is(bw.Error(), io.EOF) {
		t.Fatalf("got %v but want %v", bw.Error(), io.EOF)
	}
}

type multipleErrorsReader struct {
	calls int
}

type errorWrapper struct{}

func (errorWrapper) Error() string {
	return "subsequent calls"
}

func (mer *multipleErrorsReader) Read([]byte) (int, error) {
	mer.calls = mer.calls + 1
	if mer.calls == 1 {
		return 0, errFirstCall
	}

	return 0, errorWrapper{}
}

func TestBodyWrapperWithErrors(t *testing.T) {
	bw := NewBodyWrapper(io.NopCloser(&multipleErrorsReader{}), func(int64) {})

	data, err := io.ReadAll(bw)
	if !errors.Is(err, errFirstCall) {
		t.Fatalf("got %v but want %v", err, errFirstCall)
	}
	if string(data) != "" {
		t.Fatalf("got %s but want empty", string(data))
	}
	if !errors.Is(bw.Error(), errFirstCall) {
		t.Fatalf("got %v but want %v", bw.Error(), errFirstCall)
	}

	data, err = io.ReadAll(bw)
	if !errors.Is(err, errorWrapper{}) {
		t.Fatalf("got %v but want %v", err, errorWrapper{})
	}
	if string(data) != "" {
		t.Fatalf("got %s but want empty", string(data))
	}
	if !errors.Is(bw.Error(), errorWrapper{}) {
		t.Fatalf("got %v but want %v", bw.Error(), errorWrapper{})
	}
}

func TestConcurrentBodyWrapper(t *testing.T) {
	bw := NewBodyWrapper(io.NopCloser(strings.NewReader("hello world")), func(int64) {})

	go func() {
		_, _ = io.ReadAll(bw)
	}()

	if _ = bw.BytesRead(); false {
		t.Fatal("bw.BytesRead() is non-nil")
	}

	// Poll for the condition to become true
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(time.Second)

	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for bw.Error() to return io.EOF")
		case <-ticker.C:
			if errors.Is(bw.Error(), io.EOF) {
				return
			}
		}
	}
}
