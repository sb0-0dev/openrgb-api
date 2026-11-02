// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file defines application settings and handles configuration persistence.

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config contains settings persisted by the local web application.
type Config struct {
	Server  ServerConfig  `toml:"server" json:"server"`
	OpenRGB OpenRGBConfig `toml:"openrgb" json:"openrgb"`
}

// ServerConfig controls the HTTP servers.
type ServerConfig struct {
	// ListenAddress is the network-facing API used by paired apps.
	ListenAddress string `toml:"listen_address" json:"listen_address"`
	// AdminAddress serves the control panel and pairing. It is required and should stay
	// on a loopback address.
	AdminAddress string `toml:"admin_address" json:"admin_address"`
	// PublicURL is the address phones should use, shown in the pairing QR code. It is
	// detected automatically on a native install, but must be set when running in Docker.
	PublicURL string `toml:"public_url" json:"public_url"`
}

// OpenRGBConfig controls the connection to the OpenRGB SDK server.
type OpenRGBConfig struct {
	Host       string `toml:"host" json:"host"`
	Port       string `toml:"port" json:"port"`
	ClientName string `toml:"client_name" json:"client_name"`
}

// DefaultConfig returns safe defaults for a local installation.
func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			ListenAddress: "127.0.0.1:8742",
			AdminAddress:  "127.0.0.1:8743",
		},
		OpenRGB: OpenRGBConfig{
			Host:       "127.0.0.1",
			Port:       "6742",
			ClientName: "openrgb-api",
		},
	}
}

// DefaultConfigPath returns the platform-specific user configuration path.
func DefaultConfigPath() (string, error) {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(configDirectory, "openrgb-api", "config.toml"), nil
}

// LoadConfig reads a TOML file, or returns defaults when it does not exist.
func LoadConfig(path string) (Config, error) {
	config := DefaultConfig()
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return config, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	if err := toml.Unmarshal(contents, &config); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config %q: %w", path, err)
	}
	return config, nil
}

// SaveConfig validates and atomically writes a TOML configuration file.
func SaveConfig(path string, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	contents, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	temporaryFile, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)

	if err := temporaryFile.Chmod(0o600); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("set config permissions: %w", err)
	}
	if _, err := temporaryFile.Write(contents); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("write config: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		// Windows cannot rename over an existing file, so retry after removing it.
		if removeErr := os.Remove(path); removeErr != nil {
			return fmt.Errorf("replace config: %w", err)
		}
		if renameErr := os.Rename(temporaryPath, path); renameErr != nil {
			return fmt.Errorf("replace config after removing old file: %w", renameErr)
		}
	}
	return nil
}

// Environment variables that override the configuration file, mainly for containers.
const (
	adminAddressEnv = "OPENRGB_API_ADMIN_ADDRESS"
	publicURLEnv    = "OPENRGB_API_PUBLIC_URL"
)

// applyEnvOverrides applies non-empty environment overrides to the configuration.
func applyEnvOverrides(config *Config) {
	if value := strings.TrimSpace(os.Getenv(adminAddressEnv)); value != "" {
		config.Server.AdminAddress = value
	}
	if value := strings.TrimSpace(os.Getenv(publicURLEnv)); value != "" {
		config.Server.PublicURL = value
	}
}

// Validate checks values that would make the application unusable.
func (c Config) Validate() error {
	listenAddress := strings.TrimSpace(c.Server.ListenAddress)
	if listenAddress == "" {
		return fmt.Errorf("server.listen_address is required")
	}
	if _, port, err := net.SplitHostPort(listenAddress); err != nil || port == "" {
		return fmt.Errorf("server.listen_address must be host:port")
	} else if _, err := net.LookupPort("tcp", port); err != nil {
		return fmt.Errorf("server.listen_address has an invalid TCP port %q: %w", port, err)
	}
	admin := strings.TrimSpace(c.Server.AdminAddress)
	if admin == "" {
		return fmt.Errorf("server.admin_address is required")
	}
	if _, _, err := net.SplitHostPort(admin); err != nil {
		return fmt.Errorf("server.admin_address must be host:port: %w", err)
	}
	if admin == strings.TrimSpace(c.Server.ListenAddress) {
		return fmt.Errorf("server.admin_address must differ from server.listen_address")
	}
	if publicURL := strings.TrimSpace(c.Server.PublicURL); publicURL != "" {
		if _, err := normalizeServerURL(publicURL); err != nil {
			return fmt.Errorf("server.public_url: %w", err)
		}
	}
	if strings.TrimSpace(c.OpenRGB.Host) == "" {
		return fmt.Errorf("openrgb.host is required")
	}
	if strings.TrimSpace(c.OpenRGB.Port) == "" {
		return fmt.Errorf("openrgb.port is required")
	}
	if strings.TrimSpace(c.OpenRGB.ClientName) == "" {
		return fmt.Errorf("openrgb.client_name is required")
	}
	return nil
}
