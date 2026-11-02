// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file provides shared JSON response and error-writing helpers.

package main

import (
	"encoding/json"
	"errors"
	"net/http"
)

// define a sentinel error for internal server failures to avoid leaking details to the browser.
var errInternalServer = errors.New("internal server error")

// writeJSON writes a consistent JSON response for API endpoints.
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		errorLogger.Printf("write JSON response: %v", err)
	}
}

// writeError keeps API failures machine-readable and predictable.
func writeError(w http.ResponseWriter, status int, err error) {
	if status >= http.StatusInternalServerError {
		errorLogger.Printf("HTTP %d: %v", status, err)
	}
	if recorder, ok := w.(*responseStatusWriter); ok {
		recorder.err = err
	}
	message := err.Error()
	if status >= http.StatusInternalServerError {
		message = errInternalServer.Error()
	}
	writeJSON(w, status, map[string]string{"error": message})
}
