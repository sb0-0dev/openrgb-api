// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file tests device pairing, token validation, and local request guards.

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestGeneratePairingQR ensures that the generated QR code is valid and has the expected size.
func TestGeneratePairingQR(t *testing.T) {
	imageData, err := generatePairingQR("http://192.168.1.20:8742", "JBSWY3DPEHPK3", time.Now().UTC().Add(pairingCodeLifetime))
	if err != nil {
		t.Fatal(err)
	}
	encodedImage, ok := strings.CutPrefix(imageData, "data:image/png;base64,")
	if !ok {
		t.Fatalf("QR image has unexpected data URL: %q", imageData[:min(len(imageData), 32)])
	}
	pngData, err := base64.StdEncoding.DecodeString(encodedImage)
	if err != nil {
		t.Fatalf("decode QR image: %v", err)
	}
	image, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("decode generated QR PNG: %v", err)
	}
	if image.Bounds().Dx() != 320 || image.Bounds().Dy() != 320 {
		t.Fatalf("QR image size = %v, want 320x320", image.Bounds())
	}
}

// TestIsLoopbackHost verifies that the function correctly identifies loopback hosts.
func TestIsLoopbackHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{host: "localhost:8742", want: true},
		{host: "127.0.0.1:8742", want: true},
		{host: "0.0.0.0:8742", want: true},
		{host: "192.168.1.20:8742", want: false},
		{host: "openrgb-host:8742", want: false},
	}
	for _, test := range tests {
		if got := isLoopbackHost(test.host); got != test.want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", test.host, got, test.want)
		}
	}
}

// TestPairingIssuesPersistentSingleUseToken ensures that pairing issues a single-use token that persists across reloads and can be revoked.
func TestPairingIssuesPersistentSingleUseToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "paired-devices.json")
	store, err := openAuthStore(path)
	if err != nil {
		t.Fatal(err)
	}
	code, _, err := store.startPairing()
	if err != nil {
		t.Fatal(err)
	}

	issued, err := store.exchangePairingCode(code, "My Phone")
	if err != nil {
		t.Fatal(err)
	}
	if !store.validateToken(issued.AccessToken) {
		t.Fatal("newly issued token was rejected")
	}
	if _, err := store.exchangePairingCode(code, "Second Phone"); err == nil {
		t.Fatal("pairing code was accepted more than once")
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), issued.AccessToken) {
		t.Fatal("paired device file contains the bearer token in plaintext")
	}

	reloaded, err := openAuthStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.validateToken(issued.AccessToken) {
		t.Fatal("token did not survive reloading the paired device file")
	}
	if err := reloaded.revokeDevice(issued.Device.ID); err != nil {
		t.Fatal(err)
	}
	if reloaded.validateToken(issued.AccessToken) {
		t.Fatal("revoked token was still accepted")
	}
}

// TestPairingCodeBurnedAfterFailedAttempts ensures that a pairing code is invalidated after too many failed attempts.
func TestPairingCodeBurnedAfterFailedAttempts(t *testing.T) {
	store, err := openAuthStore(filepath.Join(t.TempDir(), "paired-devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	code, _, err := store.startPairing()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxPairingFailures; i++ {
		if _, err := store.exchangePairingCode("AAAAAAAAAAAAA", "Phone"); err == nil {
			t.Fatal("wrong code was accepted")
		}
	}
	if _, err := store.exchangePairingCode(code, "Phone"); err == nil {
		t.Fatal("pairing code still valid after too many failed attempts")
	}
	code, _, err = store.startPairing()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.exchangePairingCode(code, "Phone"); err != nil {
		t.Fatalf("fresh code rejected: %v", err)
	}
}

// TestAPIMiddlewareRequiresBearerOutsideLoopback verifies that the API middleware enforces bearer token authentication for non-loopback requests.
func TestAPIMiddlewareRequiresBearerOutsideLoopback(t *testing.T) {
	store, err := openAuthStore(filepath.Join(t.TempDir(), "paired-devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	code, _, err := store.startPairing()
	if err != nil {
		t.Fatal(err)
	}
	issued, err := store.exchangePairingCode(code, "Test Phone")
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := bearerAuthMiddleware(store, next)

	request := httptest.NewRequest(http.MethodGet, "/api/controllers", nil)
	request.RemoteAddr = "192.0.2.10:5000"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated remote request status = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/controllers", nil)
	request.RemoteAddr = "192.0.2.10:5000"
	request.Header.Set("Authorization", "Bearer "+issued.AccessToken)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("authenticated remote request status = %d, want %d", response.Code, http.StatusNoContent)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/pairing/start", nil)
	request.RemoteAddr = "127.0.0.1:5000"
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("loopback request without a token status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

// TestPairingHTTPFlowAndTokenRevocation tests the full HTTP pairing flow and ensures that tokens can be revoked.
func TestPairingHTTPFlowAndTokenRevocation(t *testing.T) {
	store, err := openAuthStore(filepath.Join(t.TempDir(), "paired-devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	api := &server{config: DefaultConfig(), configPath: "test-config.toml", auth: store}
	apiHandler := api.APIHandler()
	adminHandler := api.AdminHandler()

	response := makeAPIRequest(t, adminHandler, http.MethodPost, "/api/pairing/start", "127.0.0.1:5050", "http://127.0.0.1:8742", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("start pairing status = %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}
	var pairing pairingCodeResponse
	if err := json.Unmarshal(response.Body.Bytes(), &pairing); err != nil {
		t.Fatal(err)
	}
	response = makeAPIRequest(t, apiHandler, http.MethodPost, "/api/pairing/start", "127.0.0.1:5050", "", "", nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("start pairing on the API listener status = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	requestBody, err := json.Marshal(pairingExchangeRequest{Code: pairing.Code, DeviceName: "Living Room Phone"})
	if err != nil {
		t.Fatal(err)
	}
	response = makeAPIRequest(t, apiHandler, http.MethodPost, "/api/pairing/exchange", "192.168.1.40:5050", "", "", requestBody)
	if response.Code != http.StatusCreated {
		t.Fatalf("exchange pairing status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body)
	}
	var issued pairingExchangeResponse
	if err := json.Unmarshal(response.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	if issued.AccessToken == "" || issued.TokenType != "Bearer" {
		t.Fatalf("unexpected token response: %+v", issued)
	}

	// The API listener does not serve configuration, even to paired devices.
	response = makeAPIRequest(t, apiHandler, http.MethodGet, "/api/config", "192.168.1.40:5050", "", "", nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated config status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	response = makeAPIRequest(t, apiHandler, http.MethodGet, "/api/config", "192.168.1.40:5050", "", issued.AccessToken, nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("authenticated config status = %d, want %d: %s", response.Code, http.StatusNotFound, response.Body)
	}

	response = makeAPIRequest(t, adminHandler, http.MethodGet, "/api/pairing/devices", "127.0.0.1:5050", "http://127.0.0.1:8742", "", nil)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), issued.AccessToken) {
		t.Fatalf("paired device listing status/body unexpected: %d %s", response.Code, response.Body)
	}
	response = makeAPIRequest(t, adminHandler, http.MethodDelete, "/api/pairing/devices/"+issued.Device.ID, "127.0.0.1:5050", "http://127.0.0.1:8742", "", nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("revoke device status = %d, want %d: %s", response.Code, http.StatusNoContent, response.Body)
	}
	response = makeAPIRequest(t, apiHandler, http.MethodGet, "/api/config", "192.168.1.40:5050", "", issued.AccessToken, nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token config status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

// TestOpenAuthStoreRejectsInvalidFiles verifies corrupt or tampered stores fail to load.
func TestOpenAuthStoreRejectsInvalidFiles(t *testing.T) {
	validHash := strings.Repeat("ab", 32)
	tests := []struct {
		name    string
		content string
	}{
		{name: "malformed JSON", content: "{bad"},
		{name: "id mismatch", content: `{"a":{"id":"b","name":"x","token_hash":"` + validHash + `"}}`},
		{name: "short token hash", content: `{"a":{"id":"a","name":"x","token_hash":"abcd"}}`},
		{name: "non-hex token hash", content: `{"a":{"id":"a","name":"x","token_hash":"` + strings.Repeat("zz", 32) + `"}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "paired-devices.json")
			writeTestFile(t, path, test.content)
			if _, err := openAuthStore(path); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestPairingExchangeValidation verifies malformed codes and device names are rejected without consuming the code.
func TestPairingExchangeValidation(t *testing.T) {
	store, err := openAuthStore(filepath.Join(t.TempDir(), "paired-devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.exchangePairingCode("ABCDEFGHIJKLM", "Phone"); !errors.Is(err, errInvalidPairingCode) {
		t.Fatalf("exchange without active code error = %v, want %v", err, errInvalidPairingCode)
	}

	code, _, err := store.startPairing()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.exchangePairingCode(code, "   "); !errors.Is(err, errInvalidPairingCode) {
		t.Fatalf("blank device name error = %v, want %v", err, errInvalidPairingCode)
	}
	if _, err := store.exchangePairingCode("short", "Phone"); !errors.Is(err, errInvalidPairingCode) {
		t.Fatalf("short code error = %v, want %v", err, errInvalidPairingCode)
	}
	if _, err := store.exchangePairingCode(code, "Phone"); err != nil {
		t.Fatalf("valid code rejected after validation failures: %v", err)
	}
}

// TestPairingCodeIsCaseAndSeparatorInsensitive verifies codes typed with dashes or lower case still work.
func TestPairingCodeIsCaseAndSeparatorInsensitive(t *testing.T) {
	store, err := openAuthStore(filepath.Join(t.TempDir(), "paired-devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	code, _, err := store.startPairing()
	if err != nil {
		t.Fatal(err)
	}
	typed := strings.ToLower(code[:4] + "-" + code[4:8] + " " + code[8:])
	if _, err := store.exchangePairingCode(typed, "Phone"); err != nil {
		t.Fatalf("formatted code rejected: %v", err)
	}
}

// TestExpiredPairingCodeIsRejected verifies codes cannot be used after their lifetime.
func TestExpiredPairingCodeIsRejected(t *testing.T) {
	store, err := openAuthStore(filepath.Join(t.TempDir(), "paired-devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	code, _, err := store.startPairing()
	if err != nil {
		t.Fatal(err)
	}
	store.pairingCodeExpiry = time.Now().Add(-time.Second)
	if _, err := store.exchangePairingCode(code, "Phone"); !errors.Is(err, errInvalidPairingCode) {
		t.Fatalf("expired code error = %v, want %v", err, errInvalidPairingCode)
	}
}

// TestListDevicesOrderedByPairingTime verifies devices are listed oldest first without token data.
func TestListDevicesOrderedByPairingTime(t *testing.T) {
	store, err := openAuthStore(filepath.Join(t.TempDir(), "paired-devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"First", "Second"} {
		code, _, err := store.startPairing()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.exchangePairingCode(code, name); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	devices := store.listDevices()
	if len(devices) != 2 || devices[0].Name != "First" || devices[1].Name != "Second" {
		t.Fatalf("listDevices() = %+v, want First then Second", devices)
	}
}

// TestRevokeUnknownDevice verifies revoking a missing device reports a not-found error.
func TestRevokeUnknownDevice(t *testing.T) {
	store, err := openAuthStore(filepath.Join(t.TempDir(), "paired-devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.revokeDevice("missing"); !errors.Is(err, errPairedDeviceNotFound) {
		t.Fatalf("revokeDevice() error = %v, want %v", err, errPairedDeviceNotFound)
	}
}

// TestValidateTokenRejectsEmptyAndUnknownTokens verifies only issued tokens are accepted.
func TestValidateTokenRejectsEmptyAndUnknownTokens(t *testing.T) {
	store, err := openAuthStore(filepath.Join(t.TempDir(), "paired-devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	if store.validateToken("") || store.validateToken("not-a-token") {
		t.Fatal("token accepted without a paired device")
	}
}

// TestIsLocalHost verifies which Host headers identify this computer.
func TestIsLocalHost(t *testing.T) {
	computerName, _ := os.Hostname()
	tests := []struct {
		host string
		want bool
	}{
		{host: "localhost:8742", want: true},
		{host: "app.localhost", want: true},
		{host: "127.0.0.1:8742", want: true},
		{host: "[::1]:8742", want: true},
		{host: "0.0.0.0:8742", want: false},
		{host: "8.8.8.8:8742", want: false},
		{host: "evil.example.com", want: false},
	}
	if computerName != "" {
		tests = append(tests, struct {
			host string
			want bool
		}{host: strings.ToUpper(computerName) + ":8742", want: true})
	}
	for _, test := range tests {
		if got := isLocalHost(test.host); got != test.want {
			t.Errorf("isLocalHost(%q) = %v, want %v", test.host, got, test.want)
		}
	}
}

// TestIsSameOrigin verifies the CSRF origin check accepts only the exact request origin.
func TestIsSameOrigin(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8742/api/config", nil)
	tests := []struct {
		origin string
		want   bool
	}{
		{origin: "http://127.0.0.1:8742", want: true},
		{origin: "http://127.0.0.1:8742/", want: true},
		{origin: "HTTP://127.0.0.1:8742", want: true},
		{origin: "https://127.0.0.1:8742", want: false},
		{origin: "http://127.0.0.1:9090", want: false},
		{origin: "http://evil.example.com", want: false},
		{origin: "http://user@127.0.0.1:8742", want: false},
		{origin: "http://127.0.0.1:8742/path", want: false},
		{origin: "null", want: false},
		{origin: "://bad", want: false},
	}
	for _, test := range tests {
		if got := isSameOrigin(test.origin, request); got != test.want {
			t.Errorf("isSameOrigin(%q) = %v, want %v", test.origin, got, test.want)
		}
	}
}

// TestRemoteIP verifies client addresses are parsed from RemoteAddr in all supported forms.
func TestRemoteIP(t *testing.T) {
	tests := []struct {
		remoteAddr string
		want       string
	}{
		{remoteAddr: "192.168.1.5:4000", want: "192.168.1.5"},
		{remoteAddr: "[::1]:4000", want: "::1"},
		{remoteAddr: "10.0.0.1", want: "10.0.0.1"},
		{remoteAddr: "[fe80::1%eth0]:4000", want: "fe80::1"},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.RemoteAddr = test.remoteAddr
		got := remoteIP(request)
		if got == nil || got.String() != test.want {
			t.Errorf("remoteIP(%q) = %v, want %s", test.remoteAddr, got, test.want)
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "not-an-ip"
	if remoteIP(request) != nil {
		t.Error("remoteIP should be nil for an unparseable address")
	}
}

// TestIsLocalBrowserRequest verifies both the client address and Host header must be local.
func TestIsLocalBrowserRequest(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		host       string
		want       bool
	}{
		{name: "loopback client and host", remoteAddr: "127.0.0.1:5000", host: "127.0.0.1:8742", want: true},
		{name: "loopback client with rebinding host", remoteAddr: "127.0.0.1:5000", host: "evil.example.com", want: false},
		{name: "remote client with local host", remoteAddr: "192.168.1.40:5000", host: "127.0.0.1:8742", want: false},
		{name: "unparseable client", remoteAddr: "garbage", host: "127.0.0.1:8742", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = test.remoteAddr
			request.Host = test.host
			if got := isLocalBrowserRequest(request); got != test.want {
				t.Fatalf("isLocalBrowserRequest() = %v, want %v", got, test.want)
			}
		})
	}
}

// TestTrustedLocalNetworks verifies configured networks count as the local machine.
func TestTrustedLocalNetworks(t *testing.T) {
	networks, err := parseLocalNetworks(" 172.17.0.0/16, ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseLocalNetworks("not-a-cidr"); err == nil {
		t.Fatal("expected an error for an invalid CIDR")
	}
	trustedLocalNetworks = networks
	t.Cleanup(func() { trustedLocalNetworks = nil })

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Host = "localhost:8742"
	request.RemoteAddr = "172.17.0.1:5000"
	if !isLocalBrowserRequest(request) {
		t.Error("gateway address in a trusted network should be local")
	}
	request.RemoteAddr = "192.168.1.40:5000"
	if isLocalBrowserRequest(request) {
		t.Error("address outside the trusted networks should not be local")
	}
	request.RemoteAddr = "172.17.0.1:5000"
	request.Host = "evil.example.com"
	if isLocalBrowserRequest(request) {
		t.Error("a non-local Host header must still be rejected")
	}
}

// TestAdminAndAPIHandlersSeparateTrust verifies the API listener never trusts Host or
// address, while the admin listener serves only loopback-named local requests.
func TestAdminAndAPIHandlersSeparateTrust(t *testing.T) {
	s := newTestServer(t)
	api := s.APIHandler()
	admin := s.AdminHandler()

	for _, path := range []string{"/", "/api/pairing/start", "/api/config", "/api/pairing/devices"} {
		method := http.MethodGet
		if path == "/api/pairing/start" {
			method = http.MethodPost
		}
		response := makeAPIRequest(t, api, method, path, "127.0.0.1:5050", "", "", nil)
		if response.Code != http.StatusUnauthorized && response.Code != http.StatusNotFound && response.Code != http.StatusMethodNotAllowed {
			t.Errorf("API listener %s %s = %d, want it to be refused", method, path, response.Code)
		}
	}
	if response := makeAPIRequest(t, api, http.MethodGet, "/health", "192.168.1.40:5050", "", "", nil); response.Code != http.StatusOK {
		t.Errorf("API /health = %d, want 200", response.Code)
	}

	local := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	local.RemoteAddr = "127.0.0.1:5050"
	local.Host = "localhost:8743"
	recorder := httptest.NewRecorder()
	admin.ServeHTTP(recorder, local)
	if recorder.Code != http.StatusOK {
		t.Errorf("admin local request = %d, want 200", recorder.Code)
	}

	tests := []struct {
		name, remote, host, origin string
	}{
		{"remote client", "192.168.1.40:5050", "localhost:8743", ""},
		{"rebinding host", "127.0.0.1:5050", "evil.example.com:8743", ""},
		{"LAN host name", "127.0.0.1:5050", "192.168.1.183:8743", ""},
		{"cross origin", "127.0.0.1:5050", "localhost:8743", "http://evil.example.com"},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, "/api/config", nil)
		request.RemoteAddr = test.remote
		request.Host = test.host
		if test.origin != "" {
			request.Header.Set("Origin", test.origin)
		}
		recorder := httptest.NewRecorder()
		admin.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Errorf("admin %s = %d, want 403", test.name, recorder.Code)
		}
	}
}

// TestLANServerURLs verifies phone URLs are derived from the listen address.
func TestLANServerURLs(t *testing.T) {
	addresses := []net.Addr{
		&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
		&net.IPNet{IP: net.ParseIP("192.168.1.183"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(8, 32)},
		&net.IPNet{IP: net.ParseIP("8.8.8.8"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
	}
	if got := lanServerURLs("0.0.0.0:8742", addresses); !slices.Equal(got, []string{"http://10.0.0.5:8742", "http://192.168.1.183:8742"}) {
		t.Errorf("wildcard URLs = %v", got)
	}
	if got := lanServerURLs("127.0.0.1:8742", addresses); got != nil {
		t.Errorf("loopback URLs = %v, want none", got)
	}
	if got := lanServerURLs("192.168.1.183:9000", addresses); !slices.Equal(got, []string{"http://192.168.1.183:9000"}) {
		t.Errorf("specific URLs = %v", got)
	}
}

// TestStartPairingServerURL verifies the QR address comes from configuration, not the Host header.
func TestStartPairingServerURL(t *testing.T) {
	s := newTestServer(t)
	s.config.Server.PublicURL = "http://192.168.1.183:8742/"
	handler := s.AdminHandler()

	start := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/pairing/start", strings.NewReader(body))
		request.RemoteAddr = "127.0.0.1:5050"
		request.Host = "localhost:8743"
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}

	recorder := start("")
	var pairing pairingCodeResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &pairing); err != nil || recorder.Code != http.StatusOK {
		t.Fatalf("start = %d %s", recorder.Code, recorder.Body.String())
	}
	if pairing.ServerURL != "http://192.168.1.183:8742" || pairing.QRImage == "" {
		t.Errorf("pairing = %+v, want normalized public URL with a QR code", pairing)
	}
	if recorder := start(`{"server_url":"http://evil.example.com"}`); recorder.Code != http.StatusBadRequest {
		t.Errorf("unlisted server_url = %d, want 400", recorder.Code)
	}
}

// TestConfigAddressValidation verifies admin and public URL settings are checked.
func TestConfigAddressValidation(t *testing.T) {
	config := DefaultConfig()
	config.Server.AdminAddress = config.Server.ListenAddress
	if config.Validate() == nil {
		t.Error("admin address equal to the listen address should be rejected")
	}
	config = DefaultConfig()
	config.Server.AdminAddress = "not-an-address"
	if config.Validate() == nil {
		t.Error("admin address without a port should be rejected")
	}
	config = DefaultConfig()
	config.Server.PublicURL = "ftp://example.com"
	if config.Validate() == nil {
		t.Error("non-http public URL should be rejected")
	}
	config.Server.PublicURL = "http://192.168.1.10:8742"
	if err := config.Validate(); err != nil {
		t.Errorf("valid public URL rejected: %v", err)
	}
	config.Server.AdminAddress = ""
	if config.Validate() == nil {
		t.Error("empty admin address should be rejected")
	}
}

// TestBearerMiddlewareRejectsBadSchemes verifies bearer scheme enforcement and open paths.
func TestBearerMiddlewareRejectsBadSchemes(t *testing.T) {
	store, err := openAuthStore(filepath.Join(t.TempDir(), "paired-devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	handler := bearerAuthMiddleware(store, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	response := makeAPIRequest(t, handler, http.MethodGet, "/api/config", "127.0.0.1:5050", "", "", nil)
	if response.Code != http.StatusUnauthorized {
		t.Errorf("loopback without token status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	response = makeAPIRequest(t, handler, http.MethodGet, "/health", "192.168.1.40:5050", "", "", nil)
	if response.Code != http.StatusNoContent {
		t.Errorf("health path status = %d, want %d", response.Code, http.StatusNoContent)
	}
	response = makeAPIRequest(t, handler, http.MethodPost, "/api/pairing/exchange", "192.168.1.40:5050", "", "", nil)
	if response.Code != http.StatusNoContent {
		t.Errorf("pairing exchange status = %d, want %d", response.Code, http.StatusNoContent)
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8742/api/config", nil)
	request.RemoteAddr = "192.168.1.40:5050"
	request.Header.Set("Authorization", "Basic abc")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized || recorder.Header().Get("WWW-Authenticate") == "" {
		t.Errorf("non-bearer scheme status = %d, WWW-Authenticate = %q, want 401 with challenge", recorder.Code, recorder.Header().Get("WWW-Authenticate"))
	}
}

// makeAPIRequest is a helper function to simulate API requests with specified headers and body.
func makeAPIRequest(t *testing.T, handler http.Handler, method, path, remoteAddr, origin, token string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "http://127.0.0.1:8742"+path, strings.NewReader(string(body)))
	request.RemoteAddr = remoteAddr
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
