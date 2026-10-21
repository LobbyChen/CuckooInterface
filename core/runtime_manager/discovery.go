// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package runtime_manager

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const pythonInstallCacheTTL = 15 * time.Second

// pythonInstall 是目标机器上一份可用的 CPython 安装。
type pythonInstall struct {
	Home         string
	Executable   string
	Version      string
	Architecture string
	Source       string
	Compatible   bool
}

func (m *Manager) pythonInstallations(refresh bool) []pythonInstall {
	m.installsMu.Lock()
	if !refresh && m.installs != nil && time.Since(m.installsAt) < pythonInstallCacheTTL {
		ret := append([]pythonInstall(nil), m.installs...)
		m.installsMu.Unlock()
		return ret
	}
	m.installsMu.Unlock()

	installs := m.discoverPythonInstallations()

	m.installsMu.Lock()
	m.installs = append([]pythonInstall(nil), installs...)
	m.installsAt = time.Now()
	m.installsMu.Unlock()
	return installs
}

// effectivePython 返回当前实际会被 PythonKernel 使用的 Python。
// source 为 config（用户显式选择）或 system（自动探测）。
func (m *Manager) effectivePython() (pythonInstall, string, bool) {
	selected := m.selectedHome(pythonRuntime)
	installs := m.pythonInstallations(false)

	if selected != "" {
		for _, install := range installs {
			if strings.EqualFold(install.Home, selected) && install.Compatible {
				return install, "config", true
			}
		}
		if install, ok := probePythonHome(selected, "custom"); ok && install.Compatible {
			return install, "config", true
		}
	}

	for _, install := range installs {
		if install.Compatible {
			return install, "system", true
		}
	}
	return pythonInstall{}, "", false
}

func (m *Manager) discoverPythonInstallations() []pythonInstall {
	selected := m.selectedHome(pythonRuntime)

	type seed struct {
		exe    string
		source string
	}
	var seeds []seed

	if selected != "" {
		if exe := findPythonExe(selected); exe != "" {
			seeds = append(seeds, seed{exe: exe, source: "custom"})
		}
	}

	for _, name := range []string{"python.exe", "python3.exe"} {
		exe, err := exec.LookPath(name)
		if err != nil || exe == "" {
			continue
		}
		if strings.Contains(strings.ToLower(exe), "windowsapps") {
			continue
		}
		seeds = append(seeds, seed{exe: exe, source: "PATH"})
	}

	for _, exe := range pyLauncherPythons() {
		seeds = append(seeds, seed{exe: exe, source: "py"})
	}

	for _, home := range pythonHomesFromRegistry() {
		if exe := findPythonExe(home); exe != "" {
			seeds = append(seeds, seed{exe: exe, source: "registry"})
		}
	}

	seen := make(map[string]struct{})
	installs := make([]pythonInstall, 0, len(seeds))
	for _, candidate := range seeds {
		install, ok := probePythonExecutable(candidate.exe, candidate.source)
		if !ok {
			continue
		}
		key := strings.ToLower(install.Home)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		installs = append(installs, install)
	}

	sort.SliceStable(installs, func(i, j int) bool {
		if installs[i].Compatible != installs[j].Compatible {
			return installs[i].Compatible
		}
		if cmp := compareVersions(installs[i].Version, installs[j].Version); cmp != 0 {
			return cmp > 0
		}
		return strings.ToLower(installs[i].Home) < strings.ToLower(installs[j].Home)
	})
	return installs
}

func probePythonHome(home, source string) (pythonInstall, bool) {
	exe := findPythonExe(home)
	if exe == "" {
		return pythonInstall{}, false
	}
	return probePythonExecutable(exe, source)
}

func probePythonExecutable(exe, source string) (pythonInstall, bool) {
	exe, err := filepath.Abs(exe)
	if err != nil {
		return pythonInstall{}, false
	}
	info, ok := runPythonInfo(exe)
	if !ok {
		return pythonInstall{}, false
	}

	home := strings.TrimSpace(info.basePrefix)
	if home == "" {
		home = filepath.Dir(exe)
	}
	home = normalizeDir(home)
	if home == "" {
		return pythonInstall{}, false
	}

	architecture := normalizeArchitecture(info.machine)
	compatible := architecture == "x64" &&
		findPythonDll(home) != "" &&
		versionAtLeast38(info.version)
	if info.version == "" {
		compatible = false
	}

	return pythonInstall{
		Home:         home,
		Executable:   exe,
		Version:      info.version,
		Architecture: architecture,
		Source:       source,
		Compatible:   compatible,
	}, true
}

type pythonInfo struct {
	basePrefix string
	version    string
	machine    string
}

func runPythonInfo(exe string) (pythonInfo, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	const script = "import platform,sys; print(sys.base_prefix); print(sys.version.split()[0]); print(platform.machine())"
	cmd := exec.CommandContext(ctx, exe, "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return pythonInfo{}, false
	}

	lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	if len(lines) < 3 {
		return pythonInfo{}, false
	}
	return pythonInfo{
		basePrefix: strings.TrimSpace(lines[0]),
		version:    strings.TrimSpace(lines[1]),
		machine:    strings.TrimSpace(lines[2]),
	}, true
}

func findPythonExe(home string) string {
	home = strings.TrimSpace(home)
	if home == "" {
		return ""
	}
	if strings.EqualFold(filepath.Ext(home), ".exe") {
		if fileExists(home) {
			return home
		}
		return ""
	}
	for _, name := range []string{"python.exe", "python3.exe"} {
		path := filepath.Join(home, name)
		if fileExists(path) {
			return path
		}
	}
	return ""
}

func findPythonDll(home string) string {
	stable := filepath.Join(home, "python3.dll")
	if fileExists(stable) {
		return stable
	}
	matches, _ := filepath.Glob(filepath.Join(home, "python3*.dll"))
	for _, match := range matches {
		if strings.EqualFold(filepath.Base(match), "python3.dll") {
			continue
		}
		return match
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func normalizeDir(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.Clean(abs)
	for len(abs) > 3 && (strings.HasSuffix(abs, `\`) || strings.HasSuffix(abs, `/`)) {
		abs = abs[:len(abs)-1]
	}
	return abs
}

func normalizeArchitecture(machine string) string {
	switch strings.ToLower(strings.TrimSpace(machine)) {
	case "amd64", "x86_64", "x64":
		return "x64"
	case "x86", "i386", "i686":
		return "x86"
	case "arm64", "aarch64":
		return "arm64"
	default:
		return strings.TrimSpace(machine)
	}
}

func versionAtLeast38(version string) bool {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) < 2 {
		return false
	}
	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	if errMajor != nil || errMinor != nil {
		return false
	}
	return major > 3 || (major == 3 && minor >= 8)
}

func compareVersions(a, b string) int {
	parse := func(version string) []int {
		parts := strings.Split(version, ".")
		out := make([]int, 0, len(parts))
		for _, part := range parts {
			value, err := strconv.Atoi(part)
			if err != nil {
				break
			}
			out = append(out, value)
		}
		return out
	}
	av, bv := parse(a), parse(b)
	for i := 0; i < len(av) || i < len(bv); i++ {
		var ai, bi int
		if i < len(av) {
			ai = av[i]
		}
		if i < len(bv) {
			bi = bv[i]
		}
		if ai != bi {
			if ai > bi {
				return 1
			}
			return -1
		}
	}
	return 0
}

func pyLauncherPythons() []string {
	var launcher string
	for _, name := range []string{"py.exe", "py"} {
		if path, err := exec.LookPath(name); err == nil && path != "" {
			launcher = path
			break
		}
	}
	if launcher == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, launcher, "-0p")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	pattern := regexp.MustCompile(`(?i)([A-Z]:\\.*python\.exe)`)
	var result []string
	for _, line := range strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n") {
		if match := pattern.FindStringSubmatch(line); len(match) == 2 {
			result = append(result, strings.TrimSpace(match[1]))
		}
	}
	return result
}

func pythonHomesFromRegistry() []string {
	var homes []string
	for _, root := range []struct {
		key  registryKey
		path string
	}{
		{registryCurrentUser, `SOFTWARE\Python\PythonCore`},
		{registryLocalMachine, `SOFTWARE\Python\PythonCore`},
		{registryLocalMachine, `SOFTWARE\WOW6432Node\Python\PythonCore`},
	} {
		subKeys, err := readRegistrySubKeys(root.key, root.path)
		if err != nil {
			continue
		}
		sort.Sort(sort.Reverse(sort.StringSlice(subKeys)))
		for _, version := range subKeys {
			home, err := readRegistryDefaultValue(root.key, root.path+`\`+version+`\InstallPath`)
			if err == nil && home != "" {
				homes = append(homes, home)
			}
		}
	}
	return homes
}

func pythonSourceLabel(source string) string {
	switch source {
	case "PATH":
		return "PATH"
	case "py":
		return "py 启动器"
	case "registry":
		return "注册表"
	case "custom":
		return "手动选择"
	default:
		return source
	}
}

func formatPythonInstall(install pythonInstall) string {
	version := install.Version
	if version == "" {
		version = "版本未知"
	}
	arch := install.Architecture
	if arch == "" {
		arch = "架构未知"
	}
	status := ""
	if !install.Compatible {
		status = " [不兼容]"
	}
	return fmt.Sprintf("Python %s (%s) · %s · %s%s",
		version, arch, pythonSourceLabel(install.Source), install.Home, status)
}
