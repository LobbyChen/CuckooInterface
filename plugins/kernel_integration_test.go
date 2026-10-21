// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
//go:build windows

package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestPythonKernelIntegration 需要预先构建的 Kernel 目录：
//
//	CUCKOO_KERNEL_TEST_ROOT/
//	  binary/export.dll
//	  (target machine Python via PATH or CUCKOO_PYTHON_HOME)
//	  sdk/cuckoo_sdk/...
//	  plugins/PythonBaseDemo/
//	  plugins/PythonActiveDemo/
//
// 通过 CUCKOO_KERNEL_TEST_ROOT 启用。
func TestPythonKernelIntegration(t *testing.T) {
	root := os.Getenv("CUCKOO_KERNEL_TEST_ROOT")
	if root == "" {
		t.Skip("set CUCKOO_KERNEL_TEST_ROOT to run the PythonKernel integration test")
	}

	var mu sync.Mutex
	var logs []string
	var events []string
	api := &HostAPI{
		EmitEvent: func(name, payload string) {
			mu.Lock()
			events = append(events, name+" "+payload)
			mu.Unlock()
		},
		LogInfo: func(msg string) {
			mu.Lock()
			logs = append(logs, msg)
			mu.Unlock()
		},
		LogError: func(msg string) {
			mu.Lock()
			logs = append(logs, "ERROR "+msg)
			mu.Unlock()
			t.Logf("kernel error: %s", msg)
		},
		GetPluginConfig: func(string) string { return "{}" },
		Panic:           func(reason string) { t.Errorf("kernel panic: %s", reason) },
	}
	InjectHostAPI(api)

	kernel, err := NewKernelWithDLL(filepath.Join(root, "binary", "export.dll"))
	if err != nil {
		t.Fatalf("load kernel: %v", err)
	}
	defer kernel.Destroy()

	if err := kernel.InitRuntime(api); err != nil {
		t.Fatalf("init runtime: %v", err)
	}
	defer kernel.ShutdownRuntime()

	state, err := kernel.GetRuntimeState()
	if err != nil {
		t.Fatalf("runtime state: %v", err)
	}
	t.Logf("state: %s", state)
	var stateObj map[string]any
	if err := json.Unmarshal([]byte(state), &stateObj); err != nil {
		t.Fatalf("state is not JSON: %v", err)
	}
	if stateObj["state"] != "ready" {
		t.Fatalf("unexpected runtime state: %v", stateObj["state"])
	}
	source, _ := stateObj["source"].(string)
	if source != "system" && source != "config" {
		t.Fatalf("expected a system/configured Python runtime, got source=%v home=%v", stateObj["source"], stateObj["home"])
	}

	panel, err := kernel.GetSettingsPanel()
	if err != nil {
		t.Fatalf("settings panel: %v", err)
	}
	if !strings.Contains(panel, "runtime.version") {
		t.Fatalf("unexpected settings panel: %s", panel)
	}

	ops, err := kernel.GetOperations()
	if err != nil {
		t.Fatalf("operations: %v", err)
	}
	t.Logf("operations: %s", ops)

	if resp, err := kernel.SetSetting("log.callbacks", "true"); err != nil {
		t.Fatalf("set setting: %v", err)
	} else {
		t.Logf("set setting response: %s", resp)
	}
	if resp, err := kernel.InvokeOperation("runtime.probe", "{}"); err != nil {
		t.Fatalf("invoke operation: %v", err)
	} else {
		t.Logf("probe response: %s", resp)
	}

	baseDir := filepath.Join(root, "plugins", "PythonBaseDemo")
	baseManifest, err := os.ReadFile(filepath.Join(baseDir, "META-INF.json"))
	if err != nil {
		t.Fatalf("read base manifest: %v", err)
	}
	baseDesc, err := kernel.LoadPlugin(baseDir, string(baseManifest))
	if err != nil {
		t.Fatalf("load base plugin: %v", err)
	}
	if baseDesc.PluginID != "com.cuckoo.plugin.python.base_demo" {
		t.Fatalf("unexpected base plugin id: %s", baseDesc.PluginID)
	}
	mu.Lock()
	foundEvent := false
	for _, event := range events {
		if strings.HasPrefix(event, "python.timer.tick ") {
			foundEvent = true
		}
	}
	mu.Unlock()
	if !foundEvent {
		t.Fatalf("base plugin did not emit python.timer.tick, events=%v", events)
	}

	activeDir := filepath.Join(root, "plugins", "PythonActiveDemo")
	activeManifest, err := os.ReadFile(filepath.Join(activeDir, "META-INF.json"))
	if err != nil {
		t.Fatalf("read active manifest: %v", err)
	}
	activeDesc, err := kernel.LoadPlugin(activeDir, string(activeManifest))
	if err != nil {
		t.Fatalf("load active plugin: %v", err)
	}
	if len(activeDesc.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(activeDesc.Listeners))
	}
	kernel.TriggerCallback(activeDesc.Handle(), &activeDesc.Listeners[0],
		`{"source":"test","count":1}`)

	mu.Lock()
	defer mu.Unlock()
	// Base 插件在加载时已经 emit 一次；回调内又 emit 了 active.tick_handled。
	foundHandled := false
	for _, event := range events {
		if strings.HasPrefix(event, "python.active.tick_handled ") {
			foundHandled = true
		}
	}
	if !foundHandled {
		t.Fatalf("active plugin did not emit derived event, events=%v", events)
	}
	t.Logf("logs: %v", logs)
}

// TestLuaKernelIntegration 需要预先构建的 LuaKernel 目录：
//
//	CUCKOO_LUA_KERNEL_TEST_ROOT/
//	  binary/export.dll
//	  plugins/LuaBaseDemo/
//	  plugins/LuaActiveDemo/
func TestLuaKernelIntegration(t *testing.T) {
	root := os.Getenv("CUCKOO_LUA_KERNEL_TEST_ROOT")
	if root == "" {
		t.Skip("set CUCKOO_LUA_KERNEL_TEST_ROOT to run the LuaKernel integration test")
	}

	var mu sync.Mutex
	var events []string
	api := &HostAPI{
		EmitEvent: func(name, payload string) {
			mu.Lock()
			events = append(events, name+" "+payload)
			mu.Unlock()
		},
		LogInfo: func(string) {},
		LogError: func(msg string) {
			t.Logf("kernel error: %s", msg)
		},
		GetPluginConfig: func(string) string { return "{}" },
		Panic:           func(reason string) { t.Errorf("kernel panic: %s", reason) },
	}
	InjectHostAPI(api)

	kernel, err := NewKernelWithDLL(filepath.Join(root, "binary", "export.dll"))
	if err != nil {
		t.Fatalf("load kernel: %v", err)
	}
	defer kernel.Destroy()

	if err := kernel.InitRuntime(api); err != nil {
		t.Fatalf("init runtime: %v", err)
	}
	defer kernel.ShutdownRuntime()

	state, err := kernel.GetRuntimeState()
	if err != nil {
		t.Fatalf("runtime state: %v", err)
	}
	t.Logf("state: %s", state)
	if !strings.Contains(state, `"runtime":"lua"`) ||
		!strings.Contains(state, `"state":"ready"`) {
		t.Fatalf("unexpected runtime state: %s", state)
	}

	panel, err := kernel.GetSettingsPanel()
	if err != nil {
		t.Fatalf("settings panel: %v", err)
	}
	if !strings.Contains(panel, "runtime.version") {
		t.Fatalf("unexpected settings panel: %s", panel)
	}
	ops, err := kernel.GetOperations()
	if err != nil {
		t.Fatalf("operations: %v", err)
	}
	t.Logf("operations: %s", ops)
	if resp, err := kernel.SetSetting("log.callbacks", "true"); err != nil {
		t.Fatalf("set setting: %v", err)
	} else {
		t.Logf("set setting response: %s", resp)
	}

	baseDir := filepath.Join(root, "plugins", "LuaBaseDemo")
	baseManifest, err := os.ReadFile(filepath.Join(baseDir, "META-INF.json"))
	if err != nil {
		t.Fatalf("read base manifest: %v", err)
	}
	if _, err := kernel.LoadPlugin(baseDir, string(baseManifest)); err != nil {
		t.Fatalf("load base plugin: %v", err)
	}

	activeDir := filepath.Join(root, "plugins", "LuaActiveDemo")
	activeManifest, err := os.ReadFile(filepath.Join(activeDir, "META-INF.json"))
	if err != nil {
		t.Fatalf("read active manifest: %v", err)
	}
	activeDesc, err := kernel.LoadPlugin(activeDir, string(activeManifest))
	if err != nil {
		t.Fatalf("load active plugin: %v", err)
	}
	if len(activeDesc.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(activeDesc.Listeners))
	}
	kernel.TriggerCallback(activeDesc.Handle(), &activeDesc.Listeners[0],
		`{"source":"test","count":1}`)

	mu.Lock()
	defer mu.Unlock()
	foundHandled := false
	for _, event := range events {
		if strings.HasPrefix(event, "lua.active.tick_handled ") {
			foundHandled = true
		}
	}
	if !foundHandled {
		t.Fatalf("active plugin did not emit derived event, events=%v", events)
	}
}
