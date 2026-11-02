// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file configures the application's rotating error log.

package main

import (
	"io"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	errorLogFilename  = "openrgb-api.log"
	errorLogMaxSizeMB = 5
	errorLogBackups   = 3
)

var errorLogger = log.New(os.Stderr, "ERROR: ", log.LstdFlags|log.Lshortfile)
var accessLogger = log.New(os.Stderr, "INFO: ", log.LstdFlags|log.Lshortfile)
var errorLogWriter *lumberjack.Logger

// configureErrorLogging writes errors to stderr and to a size-limited log beside the config.
func configureErrorLogging(configPath string) error {
	logPath := filepath.Join(filepath.Dir(configPath), errorLogFilename)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return err
	}

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}

	writer := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    errorLogMaxSizeMB,
		MaxBackups: errorLogBackups,
	}
	output := io.MultiWriter(os.Stderr, writer)
	errorLogger = log.New(output, "ERROR: ", log.LstdFlags|log.Lshortfile)
	accessLogger = log.New(output, "INFO: ", log.LstdFlags|log.Lshortfile)
	errorLogWriter = writer
	return nil
}
