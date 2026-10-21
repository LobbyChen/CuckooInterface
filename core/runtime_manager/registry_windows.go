// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package runtime_manager

import "golang.org/x/sys/windows/registry"

type registryKey = registry.Key

const (
	registryCurrentUser  = registry.CURRENT_USER
	registryLocalMachine = registry.LOCAL_MACHINE
)

func readRegistrySubKeys(root registryKey, path string) ([]string, error) {
	k, err := registry.OpenKey(root, path, registry.READ)
	if err != nil {
		return nil, err
	}
	defer k.Close()
	return k.ReadSubKeyNames(-1)
}

func readRegistryDefaultValue(root registryKey, path string) (string, error) {
	k, err := registry.OpenKey(root, path, registry.READ)
	if err != nil {
		return "", err
	}
	defer k.Close()
	value, _, err := k.GetStringValue("")
	return value, err
}
