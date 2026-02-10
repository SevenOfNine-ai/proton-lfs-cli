//go:build integration

package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestE2EMockedPipeline exercises the full Git LFS pipeline through the
// adapter, SDK service, and mock proton-drive-cli bridge subprocess.
//
// Prerequisites:
//   - SDK_BACKEND_MODE=proton-drive-cli
//   - PROTON_DRIVE_CLI_BIN points at mock-proton-drive-cli.js
//   - PROTON_PASS_CLI_BIN points at mock-pass-cli.sh
//   - PASS_MOCK_USERNAME / PASS_MOCK_PASSWORD set
func TestE2EMockedPipeline(t *testing.T) {
	root := repoRoot(t)

	// Verify mock infrastructure is available.
	mockBridge := os.Getenv("PROTON_DRIVE_CLI_BIN")
	if mockBridge == "" {
		mockBridge = filepath.Join(root, "proton-lfs-bridge", "tests", "testdata", "mock-proton-drive-cli.js")
	}
	if _, err := os.Stat(mockBridge); err != nil {
		t.Skipf("mock bridge not found at %s: %v", mockBridge, err)
	}

	mockPassCLI := os.Getenv("PROTON_PASS_CLI_BIN")
	if mockPassCLI == "" {
		mockPassCLI = filepath.Join(root, "scripts", "mock-pass-cli.sh")
	}
	if _, err := os.Stat(mockPassCLI); err != nil {
		t.Skipf("mock-pass-cli.sh not found at %s: %v", mockPassCLI, err)
	}

	// Set up mock storage directory for the bridge.
	mockStorageDir := filepath.Join(t.TempDir(), "mock-bridge-storage")

	// Set env vars required by the SDK service and bridge.
	t.Setenv("SDK_BACKEND_MODE", "proton-drive-cli")
	t.Setenv("PROTON_DRIVE_CLI_BIN", mockBridge)
	t.Setenv("MOCK_BRIDGE_STORAGE_DIR", mockStorageDir)

	// Build adapter and set up repository.
	s := setupRepositoryForUpload(t)

	// Start SDK service in proton-drive-cli mode.
	service := startSDKService(t, root)

	// Build credential and bridge environment.
	sdkEnv := append(
		s.env,
		"PROTON_PASS_CLI_BIN="+mockPassCLI,
		"PROTON_PASS_REF_ROOT=pass://Personal/Proton Git LFS",
		"PASS_MOCK_USERNAME=integration-user@proton.test",
		"PASS_MOCK_PASSWORD=integration-password",
		"PROTON_DRIVE_CLI_BIN="+mockBridge,
		"SDK_BACKEND_MODE=proton-drive-cli",
		"MOCK_BRIDGE_STORAGE_DIR="+mockStorageDir,
	)

	// Configure Git LFS to use the SDK backend pointing at our service.
	configureSDKCustomTransfer(t, s.repoPath, sdkEnv, s.gitBin, s.adapterPath, service.url)

	// Verify the OID from the tracked file.
	lsFilesOutput := mustRun(t, s.repoPath, sdkEnv, s.gitLFSBin, "ls-files", "-l")
	fields := strings.Fields(strings.TrimSpace(lsFilesOutput))
	if len(fields) == 0 {
		t.Fatalf("expected oid in git lfs ls-files output, got:\n%s", lsFilesOutput)
	}
	oid := fields[0]
	if len(oid) != 64 {
		t.Fatalf("expected 64-char oid, got: %q", oid)
	}

	// Push commits and LFS objects.
	mustRun(t, s.repoPath, sdkEnv, s.gitBin, "push", "origin", "main")
	lfsPushOutput := mustRun(t, s.repoPath, sdkEnv, s.gitLFSBin, "push", "origin", "main")
	if strings.Contains(strings.ToLower(lfsPushOutput), "error") {
		t.Fatalf("unexpected error in lfs push output:\n%s", lfsPushOutput)
	}

	// Clone into a fresh directory, skipping LFS smudge.
	cloneBase := t.TempDir()
	clonePath := filepath.Join(cloneBase, "clone")
	cloneEnv := append(sdkEnv, "GIT_LFS_SKIP_SMUDGE=1")
	mustRun(t, cloneBase, cloneEnv, s.gitBin, "clone", s.remotePath, clonePath)

	// Install LFS and configure the clone to use our adapter.
	mustRun(t, clonePath, sdkEnv, s.gitLFSBin, "install", "--local")
	configureSDKCustomTransfer(t, clonePath, sdkEnv, s.gitBin, s.adapterPath, service.url)

	// Pull LFS objects.
	out, err := runCmd(clonePath, sdkEnv, s.gitLFSBin, "pull", "origin", "main")
	if err != nil {
		logTail := sdkServiceLogTail(service)
		t.Fatalf("expected lfs pull to succeed, err: %v\noutput:\n%s\nsdk logs:\n%s", err, out, logTail)
	}

	// Verify downloaded content matches the original.
	artifactPath := filepath.Join(clonePath, "artifact.bin")
	contents, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("failed to read pulled artifact: %v", err)
	}
	if string(contents) != "proton-git-lfs-integration" {
		t.Fatalf("content mismatch: expected %q, got %q (len=%d)", "proton-git-lfs-integration", string(contents), len(contents))
	}

	t.Logf("E2E mocked pipeline: upload OID=%s, download verified, content matches", oid)
}

// TestE2EMockedAPIRoundTrip exercises the SDK service HTTP API directly
// with the mock bridge, verifying upload→download content fidelity.
func TestE2EMockedAPIRoundTrip(t *testing.T) {
	root := repoRoot(t)

	mockBridge := os.Getenv("PROTON_DRIVE_CLI_BIN")
	if mockBridge == "" {
		mockBridge = filepath.Join(root, "proton-lfs-bridge", "tests", "testdata", "mock-proton-drive-cli.js")
	}
	if _, err := os.Stat(mockBridge); err != nil {
		t.Skipf("mock bridge not found at %s: %v", mockBridge, err)
	}

	mockPassCLI := os.Getenv("PROTON_PASS_CLI_BIN")
	if mockPassCLI == "" {
		mockPassCLI = filepath.Join(root, "scripts", "mock-pass-cli.sh")
	}
	if _, err := os.Stat(mockPassCLI); err != nil {
		t.Skipf("mock-pass-cli.sh not found at %s: %v", mockPassCLI, err)
	}

	// Override env for proton-drive-cli mode.
	t.Setenv("SDK_BACKEND_MODE", "proton-drive-cli")
	t.Setenv("PROTON_DRIVE_CLI_BIN", mockBridge)
	t.Setenv("MOCK_BRIDGE_STORAGE_DIR", filepath.Join(t.TempDir(), "mock-bridge-storage"))
	t.Setenv("PROTON_PASS_CLI_BIN", mockPassCLI)
	t.Setenv("PASS_MOCK_USERNAME", "integration-user@proton.test")
	t.Setenv("PASS_MOCK_PASSWORD", "integration-password")

	service := startSDKService(t, root)

	username, password := sdkResolvedCredentials(t)

	client := &http.Client{Timeout: 15 * time.Second}
	token := sdkInitToken(t, client, service, username, password)

	// Create test content and compute SHA-256 OID.
	sourceBytes := []byte("e2e-mock-api-roundtrip-content")
	hash := sha256.Sum256(sourceBytes)
	oid := hex.EncodeToString(hash[:])

	uploadPath := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(uploadPath, sourceBytes, 0o600); err != nil {
		t.Fatalf("failed to create upload file: %v", err)
	}

	uploadResp, uploadStatus := sdkJSONRequest(t, client, http.MethodPost, service.url+"/upload", map[string]string{
		"token": token,
		"oid":   oid,
		"path":  uploadPath,
	})
	if uploadStatus != http.StatusOK {
		t.Fatalf("expected /upload 200, got %d: %s", uploadStatus, string(uploadResp))
	}

	// Download and verify.
	downloadPath := filepath.Join(t.TempDir(), "download.bin")
	downloadResp, downloadStatus := sdkJSONRequest(t, client, http.MethodPost, service.url+"/download", map[string]string{
		"token":      token,
		"oid":        oid,
		"outputPath": downloadPath,
	})
	if downloadStatus != http.StatusOK {
		t.Fatalf("expected /download 200, got %d: %s", downloadStatus, string(downloadResp))
	}

	downloadedBytes, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(downloadedBytes) != string(sourceBytes) {
		t.Fatalf("content mismatch: expected %q, got %q", string(sourceBytes), string(downloadedBytes))
	}
}
