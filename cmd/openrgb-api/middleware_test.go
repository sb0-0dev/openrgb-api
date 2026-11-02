// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file tests HTTP middleware behavior and access-control boundaries.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRecoveryMiddlewareHidesPanicDetails verifies panics become generic 500 responses.
func TestRecoveryMiddlewareHidesPanicDetails(t *testing.T) {
	handler := recoveryMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("secret internal detail")
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != errInternalServer.Error() {
		t.Fatalf("error = %q, want %q", body["error"], errInternalServer.Error())
	}
}

// TestRecoveryMiddlewarePassesThroughNormalRequests verifies handlers run untouched.
func TestRecoveryMiddlewarePassesThroughNormalRequests(t *testing.T) {
	handler := recoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTeapot)
	}
}

// TestLoggingMiddlewareCallsNextHandler verifies logging does not alter responses.
func TestLoggingMiddlewareCallsNextHandler(t *testing.T) {
	called := false
	handler := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if !called || recorder.Code != http.StatusNoContent {
		t.Fatalf("called = %v, status = %d, want true and %d", called, recorder.Code, http.StatusNoContent)
	}
}
