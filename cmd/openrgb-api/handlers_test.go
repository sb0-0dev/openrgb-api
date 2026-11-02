// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file tests the HTTP handlers and their request and response behavior.

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

// writeTestFile writes fixture content for tests.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

const (
	localAddr  = "127.0.0.1:5050"
	remoteAddr = "192.168.1.40:5050"
)

// TestHealthEndpoint verifies the health check needs no OpenRGB connection.
func TestHealthEndpoint(t *testing.T) {
	handler := newTestServer(t).APIHandler()
	response := makeAPIRequest(t, handler, http.MethodGet, "/health", localAddr, "", "", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ok"`) {
		t.Fatalf("health status/body = %d %s", response.Code, response.Body)
	}
}

// TestConfigEndpoints verifies reading, updating, and rejecting configuration.
func TestConfigEndpoints(t *testing.T) {
	s := newTestServer(t)
	handler := s.AdminHandler()

	response := makeAPIRequest(t, handler, http.MethodGet, "/api/config", localAddr, "", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("get config status = %d", response.Code)
	}
	var current configResponse
	if err := json.Unmarshal(response.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if current.Path != s.configPath || current.Server.ListenAddress != DefaultConfig().Server.ListenAddress {
		t.Fatalf("unexpected config response: %+v", current)
	}

	updated := DefaultConfig()
	updated.OpenRGB.Host = "10.0.0.9"
	body, _ := json.Marshal(updated)
	response = makeAPIRequest(t, handler, http.MethodPut, "/api/config", localAddr, "", "", body)
	if response.Code != http.StatusOK {
		t.Fatalf("update config status = %d: %s", response.Code, response.Body)
	}
	saved, err := LoadConfig(s.configPath)
	if err != nil || saved.OpenRGB.Host != "10.0.0.9" {
		t.Fatalf("saved config = %+v, err = %v, want host 10.0.0.9", saved, err)
	}
	if s.config.OpenRGB.Host != "10.0.0.9" {
		t.Fatal("in-memory config was not updated")
	}

	invalid := DefaultConfig()
	invalid.OpenRGB.Host = ""
	body, _ = json.Marshal(invalid)
	response = makeAPIRequest(t, handler, http.MethodPut, "/api/config", localAddr, "", "", body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid config status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if s.config.OpenRGB.Host != "10.0.0.9" {
		t.Fatal("invalid config replaced the in-memory config")
	}

	response = makeAPIRequest(t, handler, http.MethodPut, "/api/config", localAddr, "", "", []byte("{bad"))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("malformed config status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

// TestStartPairingEndpoint verifies pairing can only be started locally and QR codes appear only for network hosts.
func TestStartPairingEndpoint(t *testing.T) {
	handler := newTestServer(t).AdminHandler()

	response := makeAPIRequest(t, handler, http.MethodPost, "/api/pairing/start", localAddr, "", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("local pairing status = %d: %s", response.Code, response.Body)
	}
	var pairing pairingCodeResponse
	if err := json.Unmarshal(response.Body.Bytes(), &pairing); err != nil {
		t.Fatal(err)
	}
	if len(pairing.Code) != 13 || pairing.QRImage != "" {
		t.Fatalf("loopback pairing = code length %d, qr %q; want 13-character code and no QR", len(pairing.Code), pairing.QRImage)
	}

	response = makeAPIRequest(t, handler, http.MethodPost, "/api/pairing/start", remoteAddr, "", "", nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("remote pairing status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

// TestExchangePairingCodeEndpointValidation verifies bad pairing exchange requests are rejected.
func TestExchangePairingCodeEndpointValidation(t *testing.T) {
	handler := newTestServer(t).APIHandler()
	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "malformed JSON", body: "{bad", want: http.StatusBadRequest},
		{name: "missing device name", body: `{"code":"ABCDEFGHIJKLM"}`, want: http.StatusBadRequest},
		{name: "device name too long", body: `{"code":"ABCDEFGHIJKLM","device_name":"` + strings.Repeat("x", 65) + `"}`, want: http.StatusBadRequest},
		{name: "no active code", body: `{"code":"ABCDEFGHIJKLM","device_name":"Phone"}`, want: http.StatusUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := makeAPIRequest(t, handler, http.MethodPost, "/api/pairing/exchange", remoteAddr, "", "", []byte(test.body))
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.want, response.Body)
			}
		})
	}
}

// TestPairedDeviceEndpointsAreLocalOnly verifies listing and revoking require the host computer.
func TestPairedDeviceEndpointsAreLocalOnly(t *testing.T) {
	s := newTestServer(t)
	handler := s.AdminHandler()
	code, _, err := s.auth.startPairing()
	if err != nil {
		t.Fatal(err)
	}
	issued, err := s.auth.exchangePairingCode(code, "Phone")
	if err != nil {
		t.Fatal(err)
	}

	// A valid token from another machine passes authentication but must still be refused.
	response := makeAPIRequest(t, handler, http.MethodGet, "/api/pairing/devices", remoteAddr, "", issued.AccessToken, nil)
	if response.Code != http.StatusForbidden {
		t.Errorf("remote list status = %d, want %d", response.Code, http.StatusForbidden)
	}
	response = makeAPIRequest(t, handler, http.MethodDelete, "/api/pairing/devices/"+issued.Device.ID, remoteAddr, "", issued.AccessToken, nil)
	if response.Code != http.StatusForbidden {
		t.Errorf("remote revoke status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if !s.auth.validateToken(issued.AccessToken) {
		t.Error("remote revoke attempt removed the device")
	}

	response = makeAPIRequest(t, handler, http.MethodDelete, "/api/pairing/devices/unknown", localAddr, "", "", nil)
	if response.Code != http.StatusNotFound {
		t.Errorf("revoke unknown device status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

// TestSetDeviceModeKeepsExplicitZeroBrightness verifies the request type distinguishes an omitted brightness from 0.
func TestSetDeviceModeKeepsExplicitZeroBrightness(t *testing.T) {
	var omitted, zero deviceModeRequest
	if err := json.Unmarshal([]byte(`{"mode":"Static"}`), &omitted); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"mode":"Static","brightness":0}`), &zero); err != nil {
		t.Fatal(err)
	}
	if omitted.Brightness != nil {
		t.Errorf("omitted brightness = %d, want nil", *omitted.Brightness)
	}
	if zero.Brightness == nil || *zero.Brightness != 0 {
		t.Errorf("explicit zero brightness = %v, want pointer to 0", zero.Brightness)
	}
}

// TestRequestValidationBeforeOpenRGBAccess verifies malformed requests fail with 400 before any OpenRGB call.
func TestRequestValidationBeforeOpenRGBAccess(t *testing.T) {
	handler := newTestServer(t).AdminHandler()
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "load profile malformed", method: http.MethodPost, path: "/api/server/profiles/load", body: "{bad"},
		{name: "load profile empty", method: http.MethodPost, path: "/api/server/profiles/load", body: `{"profile":" "}`},
		{name: "save profile empty", method: http.MethodPost, path: "/api/server/profiles/save", body: `{"profile":""}`},
		{name: "delete profile empty", method: http.MethodPost, path: "/api/server/profiles/delete", body: `{"profile":""}`},
		{name: "set effect malformed", method: http.MethodPost, path: "/api/server/effect", body: "{bad"},
		{name: "set effect empty", method: http.MethodPost, path: "/api/server/effect", body: `{"effect":""}`},
		{name: "device modes bad id", method: http.MethodGet, path: "/api/device/modes?id=abc"},
		{name: "device modes missing id", method: http.MethodGet, path: "/api/device/modes"},
		{name: "device leds negative id", method: http.MethodGet, path: "/api/device/leds?id=-1"},
		{name: "set color bad id", method: http.MethodPost, path: "/api/device/color?id=x", body: `{"r":1,"g":2,"b":3}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var body []byte
			if test.body != "" {
				body = []byte(test.body)
			}
			response := makeAPIRequest(t, handler, test.method, test.path, localAddr, "", "", body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body)
			}
		})
	}
}
