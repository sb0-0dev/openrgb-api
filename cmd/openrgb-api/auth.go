// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file implements device pairing, token storage, and request authentication.

package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	pairingCodeLifetime = 5 * time.Minute
	maxPairingFailures  = 5
)

var (
	errInvalidPairingCode   = errors.New("pairing code is invalid or expired")
	errPairedDeviceNotFound = errors.New("paired device not found")
)

// storedDevice represents a device that has been paired and persisted with a hashed token.
type storedDevice struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	TokenHash string    `json:"token_hash"`
}

// pairedDevice represents a device that has been paired and is returned to clients without the token hash.
type pairedDevice struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// issuedToken represents an access token issued to a paired device.
type issuedToken struct {
	AccessToken string
	Device      pairedDevice
}

// authStore manages paired devices, pairing codes, and issued tokens in memory and on disk.
type authStore struct {
	mu                sync.RWMutex
	path              string
	devices           map[string]storedDevice
	pairingCodeHash   [sha256.Size]byte
	pairingCodeExpiry time.Time
	pairingFailures   int
}

// openAuthStore loads and validates paired-device records from disk.
func openAuthStore(path string) (*authStore, error) {
	store := &authStore{path: path, devices: make(map[string]storedDevice)}
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read paired devices: %w", err)
	}
	if err := json.Unmarshal(contents, &store.devices); err != nil {
		return nil, fmt.Errorf("decode paired devices: %w", err)
	}
	// Reject records whose map key or token hash could not have been written by this app.
	for id, device := range store.devices {
		hash, err := hex.DecodeString(device.TokenHash)
		if id != device.ID || err != nil || len(hash) != sha256.Size {
			return nil, fmt.Errorf("invalid paired device record %q", id)
		}
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("restrict paired device file permissions: %w", err)
	}
	return store, nil
}

// startPairing creates a random code that expires after pairingCodeLifetime.
func (s *authStore) startPairing() (string, time.Time, error) {
	randomCode := make([]byte, 8)
	if _, err := rand.Read(randomCode); err != nil {
		return "", time.Time{}, fmt.Errorf("generate pairing code: %w", err)
	}
	code := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(randomCode)
	expiresAt := time.Now().UTC().Add(pairingCodeLifetime)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pairingCodeHash = sha256.Sum256([]byte(code))
	s.pairingCodeExpiry = expiresAt
	s.pairingFailures = 0
	return code, expiresAt, nil
}

// exchangePairingCode consumes the current code and persists a new paired device.
func (s *authStore) exchangePairingCode(code, deviceName string) (issuedToken, error) {
	code = strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
	deviceName = strings.TrimSpace(deviceName)
	if len(code) != 13 || deviceName == "" || len(deviceName) > 64 {
		return issuedToken{}, errInvalidPairingCode
	}
	codeHash := sha256.Sum256([]byte(code))

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pairingCodeExpiry.IsZero() || time.Now().After(s.pairingCodeExpiry) {
		return issuedToken{}, errInvalidPairingCode
	}
	if subtle.ConstantTimeCompare(codeHash[:], s.pairingCodeHash[:]) != 1 {
		// Burn the code after too many wrong guesses so it cannot be brute forced.
		s.pairingFailures++
		if s.pairingFailures >= maxPairingFailures {
			s.pairingCodeExpiry = time.Time{}
		}
		return issuedToken{}, errInvalidPairingCode
	}
	// Consume a valid code before issuing credentials so it can never be reused.
	s.pairingCodeExpiry = time.Time{}

	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return issuedToken{}, fmt.Errorf("generate access token: %w", err)
	}
	accessToken := base64.RawURLEncoding.EncodeToString(rawToken)
	rawID := make([]byte, 8)
	if _, err := rand.Read(rawID); err != nil {
		return issuedToken{}, fmt.Errorf("generate device ID: %w", err)
	}
	device := storedDevice{
		ID:        hex.EncodeToString(rawID),
		Name:      deviceName,
		CreatedAt: time.Now().UTC(),
		TokenHash: tokenHash(accessToken),
	}
	updatedDevices := make(map[string]storedDevice, len(s.devices)+1)
	for id, existing := range s.devices {
		updatedDevices[id] = existing
	}
	updatedDevices[device.ID] = device
	if err := s.persist(updatedDevices); err != nil {
		return issuedToken{}, err
	}
	s.devices = updatedDevices
	s.pairingCodeHash = [sha256.Size]byte{}
	s.pairingCodeExpiry = time.Time{}
	return issuedToken{
		AccessToken: accessToken,
		Device:      pairedDevice{ID: device.ID, Name: device.Name, CreatedAt: device.CreatedAt},
	}, nil
}

// validateToken checks a bearer token against every stored token hash.
func (s *authStore) validateToken(accessToken string) bool {
	if accessToken == "" {
		return false
	}
	hash := tokenHash(accessToken)
	s.mu.RLock()
	defer s.mu.RUnlock()
	valid := 0
	// Check every record so the position of a matching device does not affect timing.
	for _, device := range s.devices {
		valid |= subtle.ConstantTimeCompare([]byte(hash), []byte(device.TokenHash))
	}
	return valid == 1
}

// listDevices returns public device metadata sorted by pairing time.
func (s *authStore) listDevices() []pairedDevice {
	s.mu.RLock()
	defer s.mu.RUnlock()
	devices := make([]pairedDevice, 0, len(s.devices))
	for _, device := range s.devices {
		devices = append(devices, pairedDevice{ID: device.ID, Name: device.Name, CreatedAt: device.CreatedAt})
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].CreatedAt.Before(devices[j].CreatedAt) })
	return devices
}

// revokeDevice removes a paired device and persists the change.
func (s *authStore) revokeDevice(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.devices[id]; !exists {
		return errPairedDeviceNotFound
	}
	updatedDevices := make(map[string]storedDevice, len(s.devices)-1)
	for deviceID, device := range s.devices {
		if deviceID != id {
			updatedDevices[deviceID] = device
		}
	}
	if err := s.persist(updatedDevices); err != nil {
		return err
	}
	s.devices = updatedDevices
	return nil
}

// persist atomically replaces the paired-device file with restrictive permissions.
func (s *authStore) persist(devices map[string]storedDevice) error {
	contents, err := json.MarshalIndent(devices, "", "  ")
	if err != nil {
		return fmt.Errorf("encode paired devices: %w", err)
	}
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create paired device directory: %w", err)
	}
	temporaryFile, err := os.CreateTemp(directory, ".paired-devices-*.json")
	if err != nil {
		return fmt.Errorf("create paired device file: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)
	if err := temporaryFile.Chmod(0o600); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("restrict paired device file permissions: %w", err)
	}
	if _, err := temporaryFile.Write(contents); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("write paired devices: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("close paired device file: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		// Windows cannot rename over an existing file, so retry after removing it.
		if removeErr := os.Remove(s.path); removeErr != nil {
			return fmt.Errorf("replace paired device file: %w", err)
		}
		if renameErr := os.Rename(temporaryPath, s.path); renameErr != nil {
			return fmt.Errorf("replace paired device file after removing old file: %w", renameErr)
		}
	}
	return nil
}

// tokenHash returns the hexadecimal SHA-256 digest used to store an access token.
func tokenHash(accessToken string) string {
	hash := sha256.Sum256([]byte(accessToken))
	return hex.EncodeToString(hash[:])
}

// bearerAuthMiddleware requires a valid bearer token for every request except a pairing
// exchange and the health check. It never trusts the client's address or Host header.
func bearerAuthMiddleware(auth *authStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/api/pairing/exchange" {
			next.ServeHTTP(w, r)
			return
		}
		requireBearer(auth, next, w, r)
	})
}

// adminGuardMiddleware protects the control panel listener. It needs no token because the
// listener is meant to be reachable only from local machine, but it still rejects clients
// that are not local, Host headers that are not loopback names (DNS rebinding), and
// cross-origin browser requests.
func adminGuardMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLocalMachineRequest(r) || !isLoopbackHost(r.Host) {
			writeError(w, http.StatusForbidden, errors.New("the control panel is only available from this computer"))
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !isSameOrigin(origin, r) {
			writeError(w, http.StatusForbidden, errors.New("cross-origin request is not allowed"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireBearer forwards requests with a valid bearer token and rejects all others.
func requireBearer(auth *authStore, next http.Handler, w http.ResponseWriter, r *http.Request) {
	scheme, accessToken, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if ok && strings.EqualFold(scheme, "Bearer") && auth.validateToken(strings.TrimSpace(accessToken)) {
		next.ServeHTTP(w, r)
		return
	}
	w.Header().Set("WWW-Authenticate", "Bearer realm=\"openrgb-api\"")
	writeError(w, http.StatusUnauthorized, errors.New("bearer token required"))
}

// isLocalMachineRequest recognizes loopback, configured trusted networks, and local interface IPs.
func isLocalMachineRequest(r *http.Request) bool {
	requestIP := remoteIP(r)
	if requestIP == nil {
		return false
	}
	if requestIP.IsLoopback() {
		return true
	}
	for _, network := range trustedLocalNetworks {
		if network.Contains(requestIP) {
			return true
		}
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, address := range addresses {
		interfaceIP, _, err := net.ParseCIDR(address.String())
		if err == nil && interfaceIP.Equal(requestIP) {
			return true
		}
	}
	return false
}

// trustedLocalNetworks lists networks whose clients count as the local machine.
// It exists for containers, where the host's browser reaches the server through
// the container network gateway instead of a loopback address.
var trustedLocalNetworks []*net.IPNet

// localNetworksEnv names the environment variable holding a comma-separated CIDR list.
const localNetworksEnv = "OPENRGB_API_LOCAL_NETWORKS"

// parseLocalNetworks parses the comma-separated CIDR ranges trusted for local control-panel access.
func parseLocalNetworks(value string) ([]*net.IPNet, error) {
	var networks []*net.IPNet
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		_, network, err := net.ParseCIDR(part)
		if err != nil {
			return nil, fmt.Errorf("parse %s entry %q: %w", localNetworksEnv, part, err)
		}
		networks = append(networks, network)
	}
	return networks, nil
}

// isLocalBrowserRequest requires both a local client address and a local Host name.
func isLocalBrowserRequest(r *http.Request) bool {
	return isLocalMachineRequest(r) && isLocalHost(r.Host)
}

// remoteIP parses the request's peer address, including IPv6 addresses with zone identifiers.
func remoteIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if zone := strings.LastIndexByte(host, '%'); zone >= 0 {
		host = host[:zone]
	}
	return net.ParseIP(host)
}

// isLocalHost reports whether a Host header names localhost, this computer, or one of its IPs.
func isLocalHost(hostPort string) bool {
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		host = strings.Trim(hostPort, "[]")
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return true
		}
		if ip.IsUnspecified() {
			return false
		}
		addresses, err := net.InterfaceAddrs()
		if err != nil {
			return false
		}
		for _, address := range addresses {
			interfaceIP, _, err := net.ParseCIDR(address.String())
			if err == nil && interfaceIP.Equal(ip) {
				return true
			}
		}
		return false
	}
	computerName, err := os.Hostname()
	return err == nil && strings.EqualFold(host, computerName)
}

// isLoopbackHost accepts loopback names and IPs, plus unspecified IPs used by local listeners.
func isLoopbackHost(hostPort string) bool {
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		host = strings.Trim(hostPort, "[]")
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

// isSameOrigin verifies that a browser Origin matches the request scheme and Host.
func isSameOrigin(origin string, r *http.Request) bool {
	parsedOrigin, err := url.Parse(origin)
	if err != nil || parsedOrigin.Host == "" || parsedOrigin.User != nil || (parsedOrigin.Path != "" && parsedOrigin.Path != "/") {
		return false
	}
	requestScheme := "http"
	if r.TLS != nil {
		requestScheme = "https"
	}
	return strings.EqualFold(parsedOrigin.Scheme, requestScheme) && strings.EqualFold(parsedOrigin.Host, r.Host)
}
