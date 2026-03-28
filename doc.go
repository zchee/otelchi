// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package otelchi instruments the github.com/go-chi/chi package.
//
// Currently only the routing of a received message can be instrumented. To do
// it, use the Middleware function.
package otelchi
