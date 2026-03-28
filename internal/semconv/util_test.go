// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package semconv

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSplitHostPort(t *testing.T) {
	tests := []struct {
		hostport string
		host     string
		port     int
	}{
		{"", "", -1},
		{":8080", "", 8080},
		{"127.0.0.1", "127.0.0.1", -1},
		{"www.example.com", "www.example.com", -1},
		{"127.0.0.1%25en0", "127.0.0.1%25en0", -1},
		{"[]", "", -1}, // Ensure this doesn't panic.
		{"[fe80::1", "", -1},
		{"[fe80::1]", "fe80::1", -1},
		{"[fe80::1%25en0]", "fe80::1%25en0", -1},
		{"[fe80::1]:8080", "fe80::1", 8080},
		{"[fe80::1]::", "", -1}, // Too many colons.
		{"127.0.0.1:", "127.0.0.1", -1},
		{"127.0.0.1:port", "127.0.0.1", -1},
		{"127.0.0.1:8080", "127.0.0.1", 8080},
		{"www.example.com:8080", "www.example.com", 8080},
		{"127.0.0.1%25en0:8080", "127.0.0.1%25en0", 8080},
	}

	for _, test := range tests {
		h, p := SplitHostPort(test.hostport)
		if diff := cmp.Diff(test.host, h); diff != "" {
			t.Errorf("SplitHostPort(%q) host mismatch (-want +got):\n%s", test.hostport, diff)
		}
		if diff := cmp.Diff(test.port, p); diff != "" {
			t.Errorf("SplitHostPort(%q) port mismatch (-want +got):\n%s", test.hostport, diff)
		}
	}
}

func TestStandardizeHTTPMethod(t *testing.T) {
	tests := []struct {
		method string
		want   string
	}{
		{"GET", "GET"},
		{"get", "GET"},
		{"POST", "POST"},
		{"post", "POST"},
		{"PUT", "PUT"},
		{"put", "PUT"},
		{"DELETE", "DELETE"},
		{"delete", "DELETE"},
		{"HEAD", "HEAD"},
		{"head", "HEAD"},
		{"OPTIONS", "OPTIONS"},
		{"options", "OPTIONS"},
		{"CONNECT", "CONNECT"},
		{"connect", "CONNECT"},
		{"TRACE", "TRACE"},
		{"trace", "TRACE"},
		{"PATCH", "PATCH"},
		{"patch", "PATCH"},
		{"unknown", "_OTHER"},
		{"", "_OTHER"},
	}
	for _, test := range tests {
		got := standardizeHTTPMethod(test.method)
		if diff := cmp.Diff(test.want, got); diff != "" {
			t.Errorf("standardizeHTTPMethod(%q) mismatch (-want +got):\n%s", test.method, diff)
		}
	}
}
