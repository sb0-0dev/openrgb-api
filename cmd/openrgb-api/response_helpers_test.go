// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file tests shared JSON response and error-writing helpers.

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWriteJSON verifies the status, content type, and body of JSON responses.
func TestWriteJSON(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeJSON(recorder, http.StatusCreated, map[string]string{"status": "ok"})

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q, want application/json", got)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body = %v, want status ok", body)
	}
}

// TestWriteJSONUnencodableValueDoesNotPanic covers the logged encoding failure path.
func TestWriteJSONUnencodableValueDoesNotPanic(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeJSON(recorder, http.StatusOK, make(chan int))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

// TestWriteError verifies API errors use the machine-readable error shape.
func TestWriteError(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeError(recorder, http.StatusBadRequest, errors.New("bad input"))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "bad input" {
		t.Fatalf("error = %q, want %q", body["error"], "bad input")
	}
}
