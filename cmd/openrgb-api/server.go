// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file assembles API and control-panel routes with their middleware.

package main

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	openrgb "github.com/sb0-0dev/openrgb-golang/client"
)

// webFiles bundles the configuration UI into the executable.
//
//go:embed web/*
var webFiles embed.FS

// server owns the web application's dependencies and mutable configuration.
type server struct {
	openRGB         *openrgb.Client
	openRGBMu       sync.RWMutex
	reconnectMu     sync.Mutex
	reconnecting    bool
	reconnectCtx    context.Context
	cancelReconnect context.CancelFunc
	openRGBConfig   OpenRGBConfig
	configPath      string
	auth            *authStore

	configMu sync.RWMutex
	config   Config

}

// newServer creates the HTTP application around an existing OpenRGB client.
func newServer(openRGB *openrgb.Client, config Config, configPath string) (*server, error) {
	auth, err := openAuthStore(filepath.Join(filepath.Dir(configPath), "paired-devices.json"))
	if err != nil {
		return nil, err
	}
	reconnectCtx, cancelReconnect := context.WithCancel(context.Background())
	return &server{
		openRGB:         openRGB,
		openRGBConfig:   config.OpenRGB,
		reconnectCtx:    reconnectCtx,
		cancelReconnect: cancelReconnect,
		config:          config,
		configPath:      configPath,
		auth:            auth,
	}, nil
}

// openRGBClient returns the current OpenRGB client, or nil if none is available. It is safe for concurrent use.
func (s *server) openRGBClient() *openrgb.Client {
	s.openRGBMu.RLock()
	defer s.openRGBMu.RUnlock()
	return s.openRGB
}

// reconnectOpenRGB attempts to establish a new connection to the OpenRGB server if the current one is lost.
func (s *server) reconnectOpenRGB() {
	s.reconnectMu.Lock()
	if s.reconnecting {
		s.reconnectMu.Unlock()
		return
	}
	s.reconnecting = true
	s.reconnectMu.Unlock()

	go func() {
		defer func() {
			s.reconnectMu.Lock()
			s.reconnecting = false
			s.reconnectMu.Unlock()
		}()

		delay := time.Second
		for {
			select {
			case <-s.reconnectCtx.Done():
				return
			case <-time.After(delay):
			}

			client, err := openrgb.NewClient(
				s.openRGBConfig.Host,
				s.openRGBConfig.Port,
				s.openRGBConfig.ClientName,
			)
			if err != nil {
				errorLogger.Printf("reconnect to OpenRGB: %v", err)
				if delay < 30*time.Second {
					delay *= 2
					if delay > 30*time.Second {
						delay = 30 * time.Second
					}
				}
				continue
			}

			s.openRGBMu.Lock()
			select {
			case <-s.reconnectCtx.Done():
				s.openRGBMu.Unlock()
				_ = client.Close()
				return
			default:
			}
			oldClient := s.openRGB
			s.openRGB = client
			s.openRGBMu.Unlock()
			if oldClient != nil {
				time.AfterFunc(30*time.Second, func() {
					if err := oldClient.Close(); err != nil {
						errorLogger.Printf("close replaced OpenRGB client: %v", err)
					}
				})
			}
			return
		}
	}()
}

// closeOpenRGB closes the current OpenRGB client and cancels any ongoing reconnection attempts.
func (s *server) closeOpenRGB() error {
	s.cancelReconnect()
	client := s.openRGBClient()
	if client == nil {
		return nil
	}
	return client.Close()
}

// APIHandler builds the network-facing API. It has no web page, pairing management or
// configuration endpoints, and every request except a pairing exchange needs a bearer token.
func (s *server) APIHandler() http.Handler {
	mux := http.NewServeMux()
	s.registerAPIRoutes(mux)
	return loggingMiddleware(openRGBReconnectMiddleware(s, recoveryMiddleware(bearerAuthMiddleware(s.auth, mux))))
}

// AdminHandler builds the control panel served on the loopback-only admin listener.
func (s *server) AdminHandler() http.Handler {
	mux := http.NewServeMux()
	s.registerAPIRoutes(mux)
	s.registerAdminRoutes(mux)
	return loggingMiddleware(openRGBReconnectMiddleware(s, recoveryMiddleware(adminGuardMiddleware(mux))))
}

// registerAdminRoutes adds the control panel, pairing management and configuration endpoints.
func (s *server) registerAdminRoutes(mux *http.ServeMux) {
	staticFiles, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	mux.HandleFunc("GET /", s.index)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFiles))))
	mux.HandleFunc("GET /docs", s.swaggerUI)
	mux.HandleFunc("GET /openapi.yaml", s.openAPISpec)
	mux.HandleFunc("POST /api/pairing/start", s.startPairing)
	mux.HandleFunc("GET /api/pairing/devices", s.listPairedDevices)
	mux.HandleFunc("DELETE /api/pairing/devices/{id}", s.revokePairedDevice)
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("PUT /api/config", s.updateConfig)

}

// registerAPIRoutes adds the endpoints used by paired mobile apps.
func (s *server) registerAPIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /api/pairing/exchange", s.exchangePairingCode)
	mux.HandleFunc("GET /api/version", s.version)
	mux.HandleFunc("GET /api/controllers", s.controllers)
	mux.HandleFunc("POST /api/server/rescan", s.rescan)
	mux.HandleFunc("GET /api/server/profiles", s.profiles)
	mux.HandleFunc("POST /api/server/profiles/load", s.loadProfile)
	mux.HandleFunc("POST /api/server/profiles/save", s.saveProfile)
	mux.HandleFunc("POST /api/server/profiles/delete", s.deleteProfile)
	mux.HandleFunc("GET /api/device/leds", s.getDeviceLeds)
	mux.HandleFunc("POST /api/device/leds", s.updateDeviceLeds)
	mux.HandleFunc("GET /api/device/zones", s.getDeviceZones)
	mux.HandleFunc("POST /api/device/zones/resize", s.resizeDeviceZones)
	mux.HandleFunc("GET /api/device/segments", s.getDeviceSegments)
	mux.HandleFunc("POST /api/device/segments", s.addDeviceSegments)
	mux.HandleFunc("DELETE /api/device/segments", s.clearDeviceSegments)
	mux.HandleFunc("POST /api/device/refresh", s.refreshDevice)
	mux.HandleFunc("GET /api/device/modes", s.getDeviceModes)
	mux.HandleFunc("POST /api/device/mode", s.setDeviceMode)
	mux.HandleFunc("POST /api/device/color", s.setDeviceColor)
	mux.HandleFunc("GET /api/server/effects", s.getEffects)
	mux.HandleFunc("POST /api/server/effect", s.setEffect)
}
