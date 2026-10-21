// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package logger

import "testing"

func TestSetLevelAppliesAtRuntime(t *testing.T) {
	log := NewLogger(LoggerConfig{
		Level:         "info",
		LogDir:        t.TempDir(),
		MaxSize:       10,
		MaxBackups:    2,
		MaxAge:        1,
		ConsoleOutput: false,
		CacheSize:     100,
	})
	defer log.Close()

	log.Debug("hidden-debug")
	for _, record := range log.GetCachedLogs() {
		if record.Message == "hidden-debug" {
			t.Fatalf("debug log should be filtered at info level")
		}
	}

	if err := log.SetLevel("debug"); err != nil {
		t.Fatalf("SetLevel(debug): %v", err)
	}
	log.Debug("visible-debug")

	found := false
	for _, record := range log.GetCachedLogs() {
		if record.Message == "visible-debug" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("debug log should be visible after SetLevel(debug)")
	}
}

func TestSetFileRotationUpdatesConfig(t *testing.T) {
	log := NewLogger(LoggerConfig{
		Level:         "info",
		LogDir:        t.TempDir(),
		MaxSize:       10,
		MaxBackups:    2,
		MaxAge:        1,
		ConsoleOutput: false,
		CacheSize:     100,
	})
	defer log.Close()

	if err := log.SetFileRotation(64, true); err != nil {
		t.Fatalf("SetFileRotation: %v", err)
	}
	if log.LoggerConfig.MaxSize != 64 || !log.LoggerConfig.Compress {
		t.Fatalf("rotation config not updated: %+v", log.LoggerConfig)
	}
	if err := log.SetFileRotation(0, false); err == nil {
		t.Fatalf("expected invalid max size to be rejected")
	}
}
