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
	CredentialProviderPassCLI       = "pass-cli"
	CredentialProviderGitCredential = "git-credential"
)

const (
	DefaultDriveCLIBin       = "submodules/proton-drive-cli/dist/index.js"
	DefaultStorageBase       = "LFS"
	DefaultPassCLIBinary     = "pass-cli"
	DefaultPassRefRoot       = "pass://Personal/Proton Git LFS"
	DefaultCredentialProvider = CredentialProviderPassCLI
)

const (
	EnvDriveCLIBin       = "PROTON_DRIVE_CLI_BIN"
	EnvNodeBin           = "NODE_BIN"
	EnvStorageBase       = "LFS_STORAGE_BASE"
	EnvAppVersion        = "PROTON_APP_VERSION"
	EnvBackend            = "PROTON_LFS_BACKEND"
	EnvAllowMockTransfers = "ADAPTER_ALLOW_MOCK_TRANSFERS"
	EnvLocalStoreDir      = "PROTON_LFS_LOCAL_STORE_DIR"
	EnvPassCLIBin         = "PROTON_PASS_CLI_BIN"
	EnvPassRefRoot        = "PROTON_PASS_REF_ROOT"
	EnvPassUsernameRef    = "PROTON_PASS_USERNAME_REF"
	EnvPassPasswordRef    = "PROTON_PASS_PASSWORD_REF"
	EnvCredentialProvider = "PROTON_CREDENTIAL_PROVIDER"
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
