// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file manages OpenRGB connections and the API and control-panel servers.

package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kardianos/service"
	openrgb "github.com/sb0-0dev/openrgb-golang/client"
)

// webProgram implements the service.Program interface for managing the OpenRGB API and control panel servers.
type webProgram struct {
	config      Config
	configPath  string
	logger      service.Logger
	api         *server
	httpServer  *http.Server
	adminServer *http.Server

	stopOnce sync.Once
	stopErr  error
	// exit terminates the process after a fatal serving error; tests replace it.
	exit func(code int)
}

// Start connects to OpenRGB and starts the API and control-panel listeners. If OpenRGB is
// not reachable yet, the listeners still start and the connection is retried in the background.
func (p *webProgram) Start(s service.Service) error {
	logger, err := s.Logger(nil)
	if err != nil {
		return err
	}
	p.logger = logger

	openRGBClient, err := openrgb.NewClient(p.config.OpenRGB.Host, p.config.OpenRGB.Port, p.config.OpenRGB.ClientName)
	if err != nil {
		errorLogger.Printf("connect to OpenRGB: %v; retrying in the background", err)
		_ = p.logger.Warningf("connect to OpenRGB: %v; retrying in the background", err)
		openRGBClient = nil
	}

	api, err := newServer(openRGBClient, p.config, p.configPath)
	if err != nil {
		if openRGBClient != nil {
			_ = openRGBClient.Close()
		}
		return err
	}
	if openRGBClient == nil {
		api.reconnectOpenRGB()
	}

	adminAddress := strings.TrimSpace(p.config.Server.AdminAddress)

	apiListener, err := net.Listen("tcp", p.config.Server.ListenAddress)
	if err != nil {
		_ = api.closeOpenRGB()
		return err
	}
	adminListener, err := net.Listen("tcp", adminAddress)
	if err != nil {
		_ = apiListener.Close()
		_ = api.closeOpenRGB()
		return err
	}

	p.api = api
	p.httpServer = newHTTPServer(p.config.Server.ListenAddress, api.APIHandler())
	p.adminServer = newHTTPServer(adminAddress, api.AdminHandler())
	p.serve(p.httpServer, apiListener)
	_ = p.logger.Infof("OpenRGB API server listening on http://%s", p.config.Server.ListenAddress)
	p.serve(p.adminServer, adminListener)
	_ = p.logger.Infof("OpenRGB control panel listening on http://%s", adminAddress)
	return nil
}

// newHTTPServer applies the shared timeouts used by both HTTP listeners.
func newHTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         address,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// serve runs an HTTP server asynchronously. An unexpected serving failure releases the
// listeners and the OpenRGB connection the same way a normal stop does, and then exits
// non-zero so the service manager restarts the process.
func (p *webProgram) serve(httpServer *http.Server, listener net.Listener) {
	go func() {
		err := httpServer.Serve(listener)
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return
		}
		errorLogger.Printf("serve HTTP: %v", err)
		if p.logger != nil {
			_ = p.logger.Errorf("serve HTTP: %v", err)
		}
		if stopErr := p.Stop(nil); stopErr != nil {
			errorLogger.Printf("stop after serve failure: %v", stopErr)
		}
		exit := p.exit
		if exit == nil {
			exit = os.Exit
		}
		exit(1)
	}()
}

// Stop gracefully shuts down both listeners and closes the OpenRGB connection. It is safe
// to call more than once; later calls return the first result.
func (p *webProgram) Stop(service.Service) error {
	p.stopOnce.Do(func() {
		var shutdownErr error
		for _, httpServer := range []*http.Server{p.httpServer, p.adminServer} {
			if httpServer == nil {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			shutdownErr = errors.Join(shutdownErr, httpServer.Shutdown(ctx))
			cancel()
		}

		var closeErr error
		if p.api != nil {
			closeErr = p.api.closeOpenRGB()
		}
		p.stopErr = errors.Join(shutdownErr, closeErr)
	})
	return p.stopErr
}
