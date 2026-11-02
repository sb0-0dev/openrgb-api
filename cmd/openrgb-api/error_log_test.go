// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file tests rotating error log setup and output.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigureErrorLoggingWritesBesideConfig verifies errors reach the configured log file.
func TestConfigureErrorLoggingWritesBesideConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings", "config.toml")
	previousLogger := errorLogger
	previousWriter := errorLogWriter
	t.Cleanup(func() {
		if errorLogWriter != nil {
			if err := errorLogWriter.Close(); err != nil {
				t.Errorf("close error log: %v", err)
			}
		}
		errorLogger = previousLogger
		errorLogWriter = previousWriter
	})

	if err := configureErrorLogging(configPath); err != nil {
		t.Fatalf("configureErrorLogging() error = %v", err)
	}
	errorLogger.Print("test error")

	contents, err := os.ReadFile(filepath.Join(filepath.Dir(configPath), errorLogFilename))
	if err != nil {
		t.Fatalf("read error log: %v", err)
	}
	if !strings.Contains(string(contents), "test error") {
		t.Fatalf("error log does not contain the written message: %q", contents)
	}
}
