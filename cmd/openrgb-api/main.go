// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file parses startup options and runs the application or service command.

package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/kardianos/service"
)

type startupFlags struct {
	configPath string
	listen     string
	host       string
	port       string
	args       []string
}

// main loads configuration, configures the service, and starts it unless a service command was given.
func main() {
	// Parse startup flags and resolve the configuration path.
	flags := parseFlags()
	configPath, err := resolveConfigPath(flags.configPath)
	if err != nil {
		log.Fatalf("resolve config path: %v", err)
	}
	if err := configureErrorLogging(configPath); err != nil {
		log.Fatalf("configure error log: %v", err)
	}
	// Load the configuration from the resolved path.
	config, err := LoadConfig(configPath)
	if err != nil {
		errorLogger.Fatalf("load config: %v", err)
	}
	// Apply command-line and environment variable overrides to the configuration.
	applyFlagOverrides(&config, flags.listen, flags.host, flags.port)
	applyEnvOverrides(&config)
	if err := config.Validate(); err != nil {
		errorLogger.Fatalf("validate config: %v", err)
	}
	// Parse and set up trusted local networks for local machine recognition.
	if networks, err := parseLocalNetworks(os.Getenv(localNetworksEnv)); err != nil {
		errorLogger.Fatal(err)
	} else {
		trustedLocalNetworks = networks
	}
	// Initialize and run the web program with the configured service.
	program := &webProgram{config: config, configPath: configPath}
	managedService, err := service.New(program, newServiceConfig(configPath, flags))
	if err != nil {
		errorLogger.Fatalf("configure service: %v", err)
	}
	// Handle any service command (install, start, stop, etc.) before running the service.
	if err := handleServiceCommand(managedService, flags.args); err != nil {
		errorLogger.Fatal(err)
	}
	// A service command (install, start, ...) is complete; only run the server when none was given.
	// Otherwise, start the service normally.
	if len(flags.args) > 0 {
		return
	}
	// Run the service if no service command was given.
	if err := managedService.Run(); err != nil {
		errorLogger.Fatalf("run service: %v", err)
	}
}

// parseFlags parses startup overrides and any trailing service command.
func parseFlags() startupFlags {
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	configPath := fs.String("config", "", "path to the TOML configuration file")
	listenOverride := fs.String("listen", "", "override the HTTP listen address")
	hostOverride := fs.String("host", "", "override the OpenRGB server host")
	portOverride := fs.String("port", "", "override the OpenRGB server port")
	if err := fs.Parse(os.Args[1:]); err != nil {
		log.Fatalf("parse flags: %v", err)
	}
	return startupFlags{
		configPath: *configPath,
		listen:     *listenOverride,
		host:       *hostOverride,
		port:       *portOverride,
		args:       fs.Args(),
	}
}

// resolveConfigPath selects the default path when needed and returns an absolute path.
func resolveConfigPath(path string) (string, error) {
	if path == "" {
		defaultPath, err := DefaultConfigPath()
		if err != nil {
			return "", fmt.Errorf("find config path: %w", err)
		}
		path = defaultPath
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve config path: %w", err)
	}
	return absPath, nil
}

// newServiceConfig describes the operating-system service and its restart behavior.
func newServiceConfig(configPath string, flags startupFlags) *service.Config {
	return &service.Config{
		Name:        "openrgb-api",
		DisplayName: "OpenRGB API",
		Description: "OpenRGB local web application and API",
		Arguments:   serviceArguments(configPath, flags.listen, flags.host, flags.port),
		Option: service.KeyValue{
			"StartType":              "automatic",
			"DelayedAutoStart":       true,
			"OnFailure":              "restart",
			"OnFailureDelayDuration": "5s",
			"OnFailureResetPeriod":   60,
			"Restart":                "always",
		},
	}
}

// handleServiceCommand runs the requested service-management action, if any.
func handleServiceCommand(serviceInstance service.Service, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: openrgb-api [flags] [install|uninstall|start|stop|restart|status]")
	}
	if len(args) == 0 {
		return nil
	}

	action := args[0]
	if action == "status" {
		status, err := serviceInstance.Status()
		if err != nil {
			return fmt.Errorf("get service status: %w", err)
		}
		switch status {
		case service.StatusRunning:
			fmt.Println("running")
		case service.StatusStopped:
			fmt.Println("stopped")
		default:
			fmt.Println("unknown")
		}
		return nil
	}
	if action == "uninstall" {
		status, err := serviceInstance.Status()
		if errors.Is(err, service.ErrNotInstalled) {
			fmt.Println("service already uninstalled")
			return nil
		}
		if err != nil {
			return fmt.Errorf("get service status before uninstall: %w", err)
		}
		if status == service.StatusRunning {
			if err := service.Control(serviceInstance, "stop"); err != nil {
				return fmt.Errorf("stop service before uninstall: %w", err)
			}
		}
	}
	if action != "install" && action != "uninstall" && action != "start" && action != "stop" && action != "restart" {
		return fmt.Errorf("unknown command %q", action)
	}
	if err := service.Control(serviceInstance, action); err != nil {
		return fmt.Errorf("%s service: %w", action, err)
	}
	fmt.Printf("service %s successful\n", action)
	return nil
}

// serviceArguments builds the command-line arguments persisted by the service manager.
func serviceArguments(configPath, listen, host, port string) []string {
	arguments := []string{"-config", configPath}
	if listen != "" {
		arguments = append(arguments, "-listen", listen)
	}
	if host != "" {
		arguments = append(arguments, "-host", host)
	}
	if port != "" {
		arguments = append(arguments, "-port", port)
	}
	return arguments
}

// applyFlagOverrides applies explicitly supplied command-line values to the configuration.
func applyFlagOverrides(config *Config, listen, host, port string) {
	if listen != "" {
		config.Server.ListenAddress = listen
	}
	if host != "" {
		config.OpenRGB.Host = host
	}
	if port != "" {
		config.OpenRGB.Port = port
	}
}
