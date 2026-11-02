// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file provides HTTP request logging and panic recovery middleware.

package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"runtime/debug"
	"strings"

	openrgb "github.com/sb0-0dev/openrgb-golang/client"
)

// openRGBContextKey is used as the key for storing the OpenRGB client in the request context.
type openRGBContextKey struct{}

// responseStatusWriter wraps http.ResponseWriter to capture the status code and error.
type responseStatusWriter struct {
	http.ResponseWriter
	status int
	err    error
}

// WriteHeader captures the status code for the response.
func (w *responseStatusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

// Write captures the status code if it has not been set and writes the response body.
func (w *responseStatusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

// Unwrap returns the underlying http.ResponseWriter.
func (w *responseStatusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// loggingMiddleware records every request handled by the local application.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &responseStatusWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		accessLogger.Printf("%s %s %d", r.Method, r.URL.Path, status)
	})
}

// openRGBReconnectMiddleware attempts to reconnect to the OpenRGB server if a request fails due to a connection error.
func openRGBReconnectMiddleware(s *server, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client := s.openRGBClient()
		if client != nil {
			r = r.WithContext(context.WithValue(r.Context(), openRGBContextKey{}, client))
		}
		recorder := &responseStatusWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		if recorder.status == http.StatusBadGateway && isOpenRGBConnectionError(recorder.err) {
			s.reconnectOpenRGB()
		}
	})
}

// isOpenRGBConnectionError reports whether err means the OpenRGB connection is unusable,
// as opposed to the server rejecting or being unable to perform one request. The SDK
// does not wrap every connection failure, so its message text is checked as a fallback.
func isOpenRGBConnectionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errOpenRGBUnavailable) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	message := err.Error()
	for _, marker := range []string{
		"client closed", "client is closed", "connection closed", "socket read",
		"timed out waiting", "failed to connect", "broken pipe", "connection reset", "forcibly closed",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// openRGBClientFromRequest retrieves the OpenRGB client from the request context, falling back to the server's client if not present.
func openRGBClientFromRequest(s *server, r *http.Request) *openrgb.Client {
	if client, ok := r.Context().Value(openRGBContextKey{}).(*openrgb.Client); ok {
		return client
	}
	return s.openRGBClient()
}

// errOpenRGBUnavailable is returned while the service has no connection to the OpenRGB SDK server.
var errOpenRGBUnavailable = errors.New("OpenRGB is not connected; reconnecting in the background")

// requireOpenRGBClient returns the request's OpenRGB client. When the service started
// without a connection it writes a 502 and returns nil.
func requireOpenRGBClient(s *server, w http.ResponseWriter, r *http.Request) *openrgb.Client {
	client := openRGBClientFromRequest(s, r)
	if client == nil {
		writeError(w, http.StatusBadGateway, errOpenRGBUnavailable)
	}
	return client
}

// recoveryMiddleware converts unexpected handler panics into a server error.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				errorLogger.Printf("HTTP panic: %v\n%s", recovered, debug.Stack())
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": errInternalServer.Error()})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
