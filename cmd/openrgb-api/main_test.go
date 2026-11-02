// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file tests startup flags, service actions, and command-line overrides.

package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestServiceArguments verifies only supplied overrides are passed to the installed service.
func TestServiceArguments(t *testing.T) {
	tests := []struct {
		name               string
		listen, host, port string
		want               []string
	}{
		{name: "config only", want: []string{"-config", "c.toml"}},
		{
			name:   "all overrides",
			listen: "0.0.0.0:9000", host: "10.0.0.2", port: "6743",
			want: []string{"-config", "c.toml", "-listen", "0.0.0.0:9000", "-host", "10.0.0.2", "-port", "6743"},
		},
		{name: "host only", host: "10.0.0.2", want: []string{"-config", "c.toml", "-host", "10.0.0.2"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := serviceArguments("c.toml", test.listen, test.host, test.port)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("serviceArguments() = %v, want %v", got, test.want)
			}
		})
	}
}

// TestApplyFlagOverrides verifies command-line flags override only the values provided.
func TestApplyFlagOverrides(t *testing.T) {
	config := DefaultConfig()
	applyFlagOverrides(&config, "", "", "")
	if config != DefaultConfig() {
		t.Fatalf("empty overrides changed config: %+v", config)
	}

	applyFlagOverrides(&config, "0.0.0.0:9000", "10.0.0.2", "6743")
	if config.Server.ListenAddress != "0.0.0.0:9000" || config.OpenRGB.Host != "10.0.0.2" || config.OpenRGB.Port != "6743" {
		t.Fatalf("overrides not applied: %+v", config)
	}
	if config.OpenRGB.ClientName != DefaultConfig().OpenRGB.ClientName {
		t.Fatal("client name should not be overridden")
	}
}

// TestResolveConfigPath verifies explicit paths become absolute and empty paths use the default.
func TestResolveConfigPath(t *testing.T) {
	got, err := resolveConfigPath(filepath.Join("some", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) || !strings.HasSuffix(got, filepath.Join("some", "config.toml")) {
		t.Fatalf("resolveConfigPath() = %q, want absolute path ending in some/config.toml", got)
	}

	defaultPath, err := resolveConfigPath("")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(defaultPath) || filepath.Base(defaultPath) != "config.toml" {
		t.Fatalf("default config path = %q, want absolute path to config.toml", defaultPath)
	}
}

// TestNewServiceConfig verifies the service name and arguments used for installation.
func TestNewServiceConfig(t *testing.T) {
	config := newServiceConfig("c.toml", startupFlags{host: "10.0.0.2"})
	if config.Name != "openrgb-api" {
		t.Fatalf("service name = %q, want openrgb-api", config.Name)
	}
	want := []string{"-config", "c.toml", "-host", "10.0.0.2"}
	if !reflect.DeepEqual(config.Arguments, want) {
		t.Fatalf("arguments = %v, want %v", config.Arguments, want)
	}
}

// TestHandleServiceCommandValidatesArguments covers argument errors that need no service manager.
func TestHandleServiceCommandValidatesArguments(t *testing.T) {
	if err := handleServiceCommand(nil, nil); err != nil {
		t.Fatalf("no command should run the service normally, got %v", err)
	}
	if err := handleServiceCommand(nil, []string{"install", "extra"}); err == nil {
		t.Fatal("expected usage error for too many arguments")
	}
	if err := handleServiceCommand(nil, []string{"bogus"}); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}
