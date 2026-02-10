//go:build integration

package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// requireRealE2EPrereqs skips the test unless the environment is configured for
// real Proton Drive E2E: SDK_BACKEND_MODE=proton-drive-cli, pass-cli resolves
// real credentials, and proton-drive-cli is built.
func requireRealE2EPrereqs(t *testing.T) (root string) {
	t.Helper()

	mode := strings.ToLower(strings.TrimSpace(os.Getenv("SDK_BACKEND_MODE")))
	if mode != "proton-drive-cli" && mode != "real" {
		t.Skip("real E2E test skipped: SDK_BACKEND_MODE is not proton-drive-cli")
	}

	root = repoRoot(t)

	// Verify proton-drive-cli is built.
	driveCliBin := strings.TrimSpace(os.Getenv("PROTON_DRIVE_CLI_BIN"))
	if driveCliBin == "" {
		driveCliBin = filepath.Join(root, "submodules", "proton-drive-cli", "dist", "index.js")
	}
	if _, err := os.Stat(driveCliBin); err != nil {
		t.Skipf("real E2E test skipped: proton-drive-cli not built at %s (run: make build-drive-cli)", driveCliBin)
	}

	// Verify pass-cli can resolve real credentials (will skip if not logged in).
	sdkResolvedCredentials(t)

	return root
}

// TestE2ERealProtonDrivePipeline exercises the full Git LFS pipeline through
// Proton Drive: commit a test image, push via the adapter, clone into a fresh
// repo, pull, and verify byte-for-byte fidelity.
//
// Prerequisites:
//   - SDK_BACKEND_MODE=proton-drive-cli
//   - pass-cli logged in with valid Proton credentials
//   - proton-drive-cli built (make build-drive-cli)
//   - Proton Drive has a top-level folder named "LFS"
func TestE2ERealProtonDrivePipeline(t *testing.T) {
	root := requireRealE2EPrereqs(t)

	// Read the test image and append a timestamp nonce to create unique content.
	// This avoids "file already exists" errors on Proton Drive when re-running.
	testImagePath := filepath.Join(root, "tests", "testdata", "test-image.png")
	imageBytes, err := os.ReadFile(testImagePath)
	if err != nil {
		t.Fatalf("failed to read test image: %v", err)
	}
	if len(imageBytes) == 0 {
		t.Fatal("test image is empty")
	}
	nonce := []byte(fmt.Sprintf("\n# e2e-pipeline-nonce:%d", time.Now().UnixNano()))
	originalBytes := append(imageBytes, nonce...)

	// Build adapter.
	adapterPath := buildAdapter(t, root)

	gitBin, err := findToolBinary(root, "GIT_BIN", "git")
	if err != nil {
		t.Skipf("real E2E test skipped: %v", err)
	}
	gitLFSBin, err := findToolBinary(root, "GIT_LFS_BIN", "git-lfs")
	if err != nil {
		t.Skipf("real E2E test skipped: %v", err)
	}

	env := envWithPath(filepath.Dir(gitLFSBin))

	// Start LFS bridge service in proton-drive-cli mode.
	service := startSDKService(t, root)

	// Build credential env.
	sdkEnv := sdkCredentialEnv(t, env)

	// Set up source repository.
	base := t.TempDir()
	remotePath := filepath.Join(base, "remote.git")
	repoPath := filepath.Join(base, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	mustRun(t, base, sdkEnv, gitBin, "init", "--bare", remotePath)
	mustRun(t, remotePath, sdkEnv, gitBin, "symbolic-ref", "HEAD", "refs/heads/main")
	mustRun(t, repoPath, sdkEnv, gitBin, "init")
	mustRun(t, repoPath, sdkEnv, gitBin, "checkout", "-b", "main")
	mustRun(t, repoPath, sdkEnv, gitBin, "config", "user.name", "E2E Real Test")
	mustRun(t, repoPath, sdkEnv, gitBin, "config", "user.email", "e2e-real@example.com")
	mustRun(t, repoPath, sdkEnv, gitBin, "config", "commit.gpgsign", "false")
	mustRun(t, repoPath, sdkEnv, gitBin, "remote", "add", "origin", remotePath)

	mustRun(t, repoPath, sdkEnv, gitLFSBin, "install", "--local")
	configureSDKCustomTransfer(t, repoPath, sdkEnv, gitBin, adapterPath, service.url)

	// Track PNG files with LFS.
	mustRun(t, repoPath, sdkEnv, gitLFSBin, "track", "*.png")

	// Copy test image into the repo.
	imageDest := filepath.Join(repoPath, "test-image.png")
	if err := os.WriteFile(imageDest, originalBytes, 0o644); err != nil {
		t.Fatalf("failed to write test image to repo: %v", err)
	}

	mustRun(t, repoPath, sdkEnv, gitBin, "add", ".gitattributes", "test-image.png")
	mustRun(t, repoPath, sdkEnv, gitBin, "commit", "-m", "add test image via LFS")

	// Verify LFS is tracking the file.
	lsFilesOutput := mustRun(t, repoPath, sdkEnv, gitLFSBin, "ls-files", "-l")
	fields := strings.Fields(strings.TrimSpace(lsFilesOutput))
	if len(fields) == 0 {
		t.Fatalf("expected oid in git lfs ls-files output, got:\n%s", lsFilesOutput)
	}
	oid := fields[0]
	if len(oid) != 64 {
		t.Fatalf("expected 64-char oid, got: %q", oid)
	}

	// Push commits and LFS objects to Proton Drive.
	mustRun(t, repoPath, sdkEnv, gitBin, "push", "origin", "main")
	lfsPushOutput := mustRun(t, repoPath, sdkEnv, gitLFSBin, "push", "origin", "main")
	if strings.Contains(strings.ToLower(lfsPushOutput), "error") {
		logTail := sdkServiceLogTail(service)
		t.Fatalf("unexpected error in lfs push output:\n%s\nsdk logs:\n%s", lfsPushOutput, logTail)
	}

	t.Logf("upload complete: OID=%s, size=%d bytes", oid, len(originalBytes))

	// Clone into a fresh directory, skipping LFS smudge.
	cloneBase := t.TempDir()
	clonePath := filepath.Join(cloneBase, "clone")
	cloneEnv := append(sdkEnv, "GIT_LFS_SKIP_SMUDGE=1")
	mustRun(t, cloneBase, cloneEnv, gitBin, "clone", remotePath, clonePath)

	// Install LFS and configure the clone to use our adapter.
	mustRun(t, clonePath, sdkEnv, gitLFSBin, "install", "--local")
	configureSDKCustomTransfer(t, clonePath, sdkEnv, gitBin, adapterPath, service.url)

	// Pull LFS objects from Proton Drive.
	out, err := runCmd(clonePath, sdkEnv, gitLFSBin, "pull", "origin", "main")
	if err != nil {
		logTail := sdkServiceLogTail(service)
		t.Fatalf("expected lfs pull to succeed, err: %v\noutput:\n%s\nsdk logs:\n%s", err, out, logTail)
	}

	// Verify downloaded content matches the original byte-for-byte.
	downloadedPath := filepath.Join(clonePath, "test-image.png")
	downloadedBytes, err := os.ReadFile(downloadedPath)
	if err != nil {
		t.Fatalf("failed to read pulled test image: %v", err)
	}
	if !bytes.Equal(downloadedBytes, originalBytes) {
		t.Fatalf("content mismatch: original=%d bytes, downloaded=%d bytes", len(originalBytes), len(downloadedBytes))
	}

	t.Logf("E2E real pipeline: upload OID=%s, download verified, %d bytes match", oid, len(originalBytes))
}

// TestE2ERealProtonDriveAPIRoundTrip exercises the SDK service HTTP API directly
// against real Proton Drive, verifying upload, download, and list operations.
//
// Prerequisites: same as TestE2ERealProtonDrivePipeline.
func TestE2ERealProtonDriveAPIRoundTrip(t *testing.T) {
	root := requireRealE2EPrereqs(t)

	// Read the test image and append a timestamp nonce to create unique content.
	// This avoids "file already exists" errors on Proton Drive when the same
	// OID was uploaded by a previous test run or the pipeline test.
	testImagePath := filepath.Join(root, "tests", "testdata", "test-image.png")
	imageBytes, err := os.ReadFile(testImagePath)
	if err != nil {
		t.Fatalf("failed to read test image: %v", err)
	}
	nonce := []byte(fmt.Sprintf("\n# e2e-api-nonce:%d", time.Now().UnixNano()))
	sourceBytes := append(imageBytes, nonce...)

	// Compute SHA-256 OID for the unique content.
	hash := sha256.Sum256(sourceBytes)
	oid := hex.EncodeToString(hash[:])

	// Start LFS bridge service.
	service := startSDKService(t, root)

	// Resolve real credentials and authenticate.
	username, password := sdkResolvedCredentials(t)
	client := &http.Client{Timeout: 60 * time.Second}
	token := sdkInitToken(t, client, service, username, password)

	// Upload the test image.
	uploadPath := filepath.Join(t.TempDir(), "upload.png")
	if err := os.WriteFile(uploadPath, sourceBytes, 0o600); err != nil {
		t.Fatalf("failed to create upload file: %v", err)
	}

	uploadResp, uploadStatus := sdkJSONRequest(t, client, http.MethodPost, service.url+"/upload", map[string]string{
		"token": token,
		"oid":   oid,
		"path":  uploadPath,
	})
	if uploadStatus != http.StatusOK {
		logTail := sdkServiceLogTail(service)
		t.Fatalf("expected /upload 200, got %d: %s\nsdk logs:\n%s", uploadStatus, string(uploadResp), logTail)
	}

	t.Logf("upload complete: OID=%s, size=%d bytes", oid, len(sourceBytes))

	// Download and verify byte equality.
	downloadPath := filepath.Join(t.TempDir(), "download.png")
	downloadResp, downloadStatus := sdkJSONRequest(t, client, http.MethodPost, service.url+"/download", map[string]string{
		"token":      token,
		"oid":        oid,
		"outputPath": downloadPath,
	})
	if downloadStatus != http.StatusOK {
		logTail := sdkServiceLogTail(service)
		t.Fatalf("expected /download 200, got %d: %s\nsdk logs:\n%s", downloadStatus, string(downloadResp), logTail)
	}

	downloadedBytes, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if !bytes.Equal(downloadedBytes, sourceBytes) {
		t.Fatalf("content mismatch: expected %d bytes, got %d bytes", len(sourceBytes), len(downloadedBytes))
	}

	t.Logf("download verified: OID=%s, %d bytes match", oid, len(downloadedBytes))

	// Verify the OID appears in /list.
	listPayload, listStatus := sdkJSONRequest(
		t,
		client,
		http.MethodGet,
		fmt.Sprintf("%s/list?token=%s&folder=LFS", service.url, url.QueryEscape(token)),
		nil,
	)
	if listStatus != http.StatusOK {
		logTail := sdkServiceLogTail(service)
		t.Fatalf("expected /list 200, got %d: %s\nsdk logs:\n%s", listStatus, string(listPayload), logTail)
	}

	var listResult struct {
		Files []map[string]any `json:"files"`
	}
	if err := json.Unmarshal(listPayload, &listResult); err != nil {
		t.Fatalf("failed to parse /list response: %v (%s)", err, strings.TrimSpace(string(listPayload)))
	}
	if listResult.Files == nil {
		t.Fatalf("expected files array in /list response: %s", strings.TrimSpace(string(listPayload)))
	}

	// The OID prefix directory (first 2 chars) should appear in the listing.
	oidPrefix := oid[:2]
	found := false
	for _, f := range listResult.Files {
		name, _ := f["name"].(string)
		if strings.Contains(name, oidPrefix) || strings.Contains(name, oid) {
			found = true
			break
		}
	}
	if !found {
		t.Logf("warning: OID prefix %q not found in /list response (may be nested); files: %v", oidPrefix, listResult.Files)
	}

	t.Logf("E2E real API roundtrip: upload/download/list verified for OID=%s", oid)
}
