// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file tests server route registration and listener-specific behavior.

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestServer builds a server without an OpenRGB connection for routes that never use it.
func newTestServer(t *testing.T) *server {
	t.Helper()
	s, err := newServer(nil, DefaultConfig(), filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestNewServerStoresPairedDevicesNextToConfig verifies the auth store location.
func TestNewServerStoresPairedDevicesNextToConfig(t *testing.T) {
	dir := t.TempDir()
	s, err := newServer(nil, DefaultConfig(), filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "paired-devices.json"); s.auth.path != want {
		t.Fatalf("auth store path = %q, want %q", s.auth.path, want)
	}
}

// TestNewServerRejectsCorruptPairedDevices verifies startup fails on an unreadable auth store.
func TestNewServerRejectsCorruptPairedDevices(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "paired-devices.json"), "{not json")
	if _, err := newServer(nil, DefaultConfig(), filepath.Join(dir, "config.toml")); err == nil {
		t.Fatal("expected error for corrupt paired devices file")
	}
}

// TestHandlerServesEmbeddedUI verifies the index page and static assets are bundled.
func TestHandlerServesEmbeddedUI(t *testing.T) {
	handler := newTestServer(t).AdminHandler()
	for _, path := range []string{"/", "/static/app.js", "/static/style.css"} {
		response := makeAPIRequest(t, handler, http.MethodGet, path, "127.0.0.1:5050", "", "", nil)
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Errorf("GET %s status = %d, length = %d, want 200 with a body", path, response.Code, response.Body.Len())
		}
	}
}

func TestHealthAllowsCrossOriginRequests(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set("Origin", "http://localhost:8082")
	response := httptest.NewRecorder()

	newTestServer(t).APIHandler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
	if got := strings.TrimSpace(response.Body.String()); got != `{"status":"ok"}` {
		t.Errorf("body = %q, want %q", got, `{"status":"ok"}`)
	}
}

func TestAdminHandlerServesSwaggerUIAndSpec(t *testing.T) {
	s := newTestServer(t)
	s.config.Server.ListenAddress = "0.0.0.0:9902"
	s.config.Server.AdminAddress = "0.0.0.0:9903"
	handler := s.AdminHandler()

	response := makeAPIRequest(t, handler, http.MethodGet, "/docs", "127.0.0.1:5050", "", "", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "SwaggerUIBundle") {
		t.Errorf("GET /docs status = %d, want 200 with Swagger UI", response.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	request.Host = "attacker.localhost:9903"
	request.RemoteAddr = "127.0.0.1:5050"
	spec := httptest.NewRecorder()
	handler.ServeHTTP(spec, request)
	if spec.Code != http.StatusOK || !strings.HasPrefix(spec.Body.String(), "openapi: 3.1.0") {
		t.Fatalf("GET /openapi.yaml status = %d, want 200 with the spec", spec.Code)
	}
	body := spec.Body.String()
	if !strings.Contains(body, "url: /") {
		t.Error("spec does not use same-origin relative server URLs")
	}
	if strings.Contains(body, "http://127.0.0.1:") {
		t.Error("spec still contains an absolute listener URL")
	}
	if strings.Contains(body, "attacker.localhost") {
		t.Error("spec reflected the request Host header")
	}
}

// TestHandlerUnknownRoutes verifies unknown paths and methods are rejected.
func TestHandlerUnknownRoutes(t *testing.T) {
	handler := newTestServer(t).AdminHandler()

	response := makeAPIRequest(t, handler, http.MethodGet, "/missing", "127.0.0.1:5050", "", "", nil)
	if response.Code != http.StatusNotFound {
		t.Errorf("GET /missing status = %d, want %d", response.Code, http.StatusNotFound)
	}
	response = makeAPIRequest(t, handler, http.MethodPost, "/health", "127.0.0.1:5050", "", "", nil)
	if response.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /health status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestOpenRGBReconnectMiddlewareSchedulesReconnectAfterBadGateway(t *testing.T) {
	s := newTestServer(t)
	s.openRGBConfig.Host = "127.0.0.1"
	s.openRGBConfig.Port = "1"
	defer s.cancelReconnect()

	handler := openRGBReconnectMiddleware(s, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusBadGateway, context.DeadlineExceeded)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/controllers", nil))
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadGateway)
	}

	s.reconnectMu.Lock()
	reconnecting := s.reconnecting
	s.reconnectMu.Unlock()
	if !reconnecting {
		t.Fatal("a failed OpenRGB request did not schedule reconnection")
	}

	s.cancelReconnect()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.reconnectMu.Lock()
		reconnecting = s.reconnecting
		s.reconnectMu.Unlock()
		if !reconnecting {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("reconnection worker did not stop after cancellation")
}

// TestOpenRGBReconnectMiddlewareIgnoresNonConnectionErrors verifies a rejected request does not drop the connection.
func TestOpenRGBReconnectMiddlewareIgnoresNonConnectionErrors(t *testing.T) {
	s := newTestServer(t)
	defer s.cancelReconnect()

	handler := openRGBReconnectMiddleware(s, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusBadGateway, errors.New("effects plugin not found"))
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/server/effect", nil))
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadGateway)
	}
	s.reconnectMu.Lock()
	reconnecting := s.reconnecting
	s.reconnectMu.Unlock()
	if reconnecting {
		t.Fatal("a non-connection error scheduled reconnection")
	}
}

// TestHandlerRemovedResizeAliasIsNotRouted verifies the body-less GET resize alias stays removed.
func TestHandlerRemovedResizeAliasIsNotRouted(t *testing.T) {
	handler := newTestServer(t).AdminHandler()
	response := makeAPIRequest(t, handler, http.MethodGet, "/api/device/zones/resize?id=0", "127.0.0.1:5050", "", "", nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("GET zones/resize status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

// TestHandlerIndexRejectsNonRootPaths verifies the catch-all route only serves "/".
func TestHandlerIndexRejectsNonRootPaths(t *testing.T) {
	recorder := httptest.NewRecorder()
	newTestServer(t).index(recorder, httptest.NewRequest(http.MethodGet, "/other", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if strings.Contains(recorder.Body.String(), "<html") {
		t.Fatal("index page served for a non-root path")
	}
}
