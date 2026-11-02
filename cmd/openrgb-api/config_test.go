// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file tests configuration defaults, validation, and file persistence.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigValidationRejectsEachMissingField verifies every required setting is checked.
func TestConfigValidationRejectsEachMissingField(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "listen address", mutate: func(c *Config) { c.Server.ListenAddress = " " }, want: "listen_address"},
		{name: "host", mutate: func(c *Config) { c.OpenRGB.Host = "" }, want: "openrgb.host"},
		{name: "port", mutate: func(c *Config) { c.OpenRGB.Port = "\t" }, want: "openrgb.port"},
		{name: "client name", mutate: func(c *Config) { c.OpenRGB.ClientName = "" }, want: "client_name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := DefaultConfig()
			test.mutate(&config)
			err := config.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want it to mention %q", err, test.want)
			}
		})
	}
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
}

// TestSaveConfigRejectsInvalidConfigWithoutWriting verifies invalid settings never reach disk.
func TestSaveConfigRejectsInvalidConfigWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	config := DefaultConfig()
	config.OpenRGB.Host = ""
	if err := SaveConfig(path, config); err == nil {
		t.Fatal("expected validation error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config file should not exist, stat error = %v", err)
	}
}

// TestSaveConfigOverwritesExistingFile verifies saving replaces earlier settings and leaves no temporary files.
func TestSaveConfigOverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	first := DefaultConfig()
	first.OpenRGB.Host = "first"
	second := DefaultConfig()
	second.OpenRGB.Host = "second"

	if err := SaveConfig(path, first); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfig(path, second); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil || got.OpenRGB.Host != "second" {
		t.Fatalf("loaded config = %+v, err = %v, want host second", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("directory has %d entries, want only config.toml", len(entries))
	}
}

// TestLoadConfigRejectsBadFiles verifies malformed and invalid files produce errors.
func TestLoadConfigRejectsBadFiles(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		content string
	}{
		{name: "malformed TOML", content: "server = [unclosed"},
		{name: "empty host", content: "[openrgb]\nhost = \"\"\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(test.name, " ", "-")+".toml")
			writeTestFile(t, path, test.content)
			if _, err := LoadConfig(path); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestLoadConfigKeepsDefaultsForOmittedFields verifies partial files are merged with defaults.
func TestLoadConfigKeepsDefaultsForOmittedFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeTestFile(t, path, "[openrgb]\nhost = \"10.0.0.5\"\n")
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultConfig()
	want.OpenRGB.Host = "10.0.0.5"
	if got != want {
		t.Fatalf("loaded config = %+v, want %+v", got, want)
	}
}

// TestLoadConfigReportsUnreadablePath verifies non-missing read errors are not hidden as defaults.
func TestLoadConfigReportsUnreadablePath(t *testing.T) {
	if _, err := LoadConfig(t.TempDir()); err == nil {
		t.Fatal("expected error reading a directory as a config file")
	}
}

// TestDefaultConfigPath verifies the default location is under the user config directory.
func TestDefaultConfigPath(t *testing.T) {
	path, err := DefaultConfigPath()
	if err != nil {
		t.Skipf("no user config directory available: %v", err)
	}
	if filepath.Base(path) != "config.toml" || filepath.Base(filepath.Dir(path)) != "openrgb-api" {
		t.Fatalf("DefaultConfigPath() = %q, want .../openrgb-api/config.toml", path)
	}
}

// TestConfigRoundTrip verifies that the TOML file preserves application settings.
func TestConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	want := DefaultConfig()
	want.Server.ListenAddress = "127.0.0.1:9090"
	want.OpenRGB.ClientName = "test-client"

	if err := SaveConfig(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("loaded config = %+v, want %+v", got, want)
	}
}

// TestLoadMissingConfigUsesDefaults keeps first-run startup free of setup steps.
func TestLoadMissingConfigUsesDefaults(t *testing.T) {
	got, err := LoadConfig(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if got != DefaultConfig() {
		t.Fatalf("loaded config = %+v, want defaults", got)
	}
}

// TestConfigValidationRejectsMissingConnectionSettings protects startup errors.
func TestConfigValidationRejectsMissingConnectionSettings(t *testing.T) {
	config := DefaultConfig()
	config.OpenRGB.Host = ""
	if err := config.Validate(); err == nil {
		t.Fatal("expected missing host validation error")
	}
}

// TestConfigJSONShape keeps the browser API contract nested and lowercase.
func TestConfigJSONShape(t *testing.T) {
	response := configResponse{Config: DefaultConfig(), Path: "config.toml"}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	server, ok := decoded["server"].(map[string]any)
	if !ok || server["listen_address"] != "127.0.0.1:8742" {
		t.Fatalf("unexpected server JSON: %s", encoded)
	}
	openRGB, ok := decoded["openrgb"].(map[string]any)
	if !ok || openRGB["client_name"] != "openrgb-api" {
		t.Fatalf("unexpected openrgb JSON: %s", encoded)
	}
}
