// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file tests service lifecycle behavior around serving failures.

package main

import (
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

// failingListener makes http.Server.Serve return a non-shutdown error.
type failingListener struct{}

func (failingListener) Accept() (net.Conn, error) { return nil, errors.New("accept failed") }
func (failingListener) Close() error              { return nil }
func (failingListener) Addr() net.Addr            { return &net.TCPAddr{} }

// TestServeFailureStopsGracefullyBeforeExit verifies a serving error closes the servers and
// OpenRGB connection, and only then exits non-zero.
func TestServeFailureStopsGracefullyBeforeExit(t *testing.T) {
	api := newTestServer(t)
	exitCode := make(chan int, 1)
	program := &webProgram{api: api, exit: func(code int) { exitCode <- code }}
	program.httpServer = newHTTPServer("127.0.0.1:0", http.NotFoundHandler())

	program.serve(program.httpServer, failingListener{})

	select {
	case code := <-exitCode:
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("process did not exit after a serving failure")
	}
	if api.reconnectCtx.Err() == nil {
		t.Fatal("serving failure exited without running the graceful stop")
	}
	if err := program.Stop(nil); err != nil {
		t.Fatalf("second Stop() = %v, want the first result (nil)", err)
	}
}

// TestRequestsFailCleanlyWithoutOpenRGBConnection verifies the service answers with 502,
// not a panic, while it has not connected to OpenRGB yet.
func TestRequestsFailCleanlyWithoutOpenRGBConnection(t *testing.T) {
	s := newTestServer(t)
	s.openRGBConfig.Host = "127.0.0.1"
	s.openRGBConfig.Port = "1"
	defer s.cancelReconnect()
	handler := s.AdminHandler()

	for _, test := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/controllers", ""},
		{http.MethodGet, "/api/server/profiles", ""},
		{http.MethodGet, "/api/device/modes?id=0", ""},
		{http.MethodGet, "/api/server/effects", ""},
	} {
		var body []byte
		if test.body != "" {
			body = []byte(test.body)
		}
		response := makeAPIRequest(t, handler, test.method, test.path, localAddr, "", "", body)
		if response.Code != http.StatusBadGateway {
			t.Errorf("%s %s status = %d, want %d: %s", test.method, test.path, response.Code, http.StatusBadGateway, response.Body)
		}
	}

	s.reconnectMu.Lock()
	reconnecting := s.reconnecting
	s.reconnectMu.Unlock()
	if !reconnecting {
		t.Error("an unconnected request did not schedule reconnection")
	}

	health := makeAPIRequest(t, handler, http.MethodGet, "/health", localAddr, "", "", nil)
	if health.Code != http.StatusOK {
		t.Errorf("health without OpenRGB status = %d, want %d", health.Code, http.StatusOK)
	}
}
