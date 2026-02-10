package main

import (
	"os"
	"strconv"
	"strings"
)

const (
	BackendLocal = "local"
	BackendSDK   = "sdk"
)

const (
	DefaultLFSBridgeURL      = "http://localhost:3000"
	DefaultConcurrentWorkers = 4
	DefaultPassCLIBinary     = "pass-cli"
	DefaultPassRefRoot       = "pass://Personal/Proton Git LFS"
)

const (
	EnvLFSBridgeURL       = "LFS_BRIDGE_URL"
	EnvBackend            = "PROTON_LFS_BACKEND"
	EnvAllowMockTransfers = "ADAPTER_ALLOW_MOCK_TRANSFERS"
	EnvLocalStoreDir      = "PROTON_LFS_LOCAL_STORE_DIR"
	EnvPassCLIBin         = "PROTON_PASS_CLI_BIN"
	EnvPassRefRoot        = "PROTON_PASS_REF_ROOT"
	EnvPassUsernameRef    = "PROTON_PASS_USERNAME_REF"
	EnvPassPasswordRef    = "PROTON_PASS_PASSWORD_REF"
)

func envTrim(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func envOrDefault(key, fallback string) string {
	if value := envTrim(key); value != "" {
		return value
	}
	return fallback
}

func envBoolOrDefault(key string, fallback bool) bool {
	value := envTrim(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func passRefRootFromEnv() string {
	root := envOrDefault(EnvPassRefRoot, DefaultPassRefRoot)
	return normalizePassRefRoot(root)
}

func normalizePassRefRoot(root string) string {
	root = strings.TrimSpace(root)
	root = strings.TrimRight(root, "/")
	return root
}

func defaultPassUsernameRef(root string) string {
	if root == "" {
		return ""
	}
	return root + "/username"
}

func defaultPassPasswordRef(root string) string {
	if root == "" {
		return ""
	}
	return root + "/password"
}
