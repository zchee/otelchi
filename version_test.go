// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package otelchi_test

import (
	"regexp"
	"testing"

	"github.com/zchee/otelchi"
)

// regex taken from https://semver.org/#is-there-a-suggested-regular-expression-regex-to-check-a-semver-string
var versionRegex = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
	`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)` +
	`(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?` +
	`(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

func TestVersionSemver(t *testing.T) {
	v := otelchi.Version()
	if versionRegex.FindStringSubmatch(v) == nil {
		t.Fatalf("version is not semver: %s", v)
	}
}
