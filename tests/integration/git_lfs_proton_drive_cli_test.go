//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestProtonDriveCliSDKServiceHealth verifies that the SDK service starts
// correctly with the proton-drive-cli bridge backend.
func TestProtonDriveCliSDKServiceHealth(t *testing.T) {
	root := repoRoot(t)
	service := startSDKService(t, root)
	client := &http.Client{Timeout: 10 * time.Second}

	payload, status := sdkJSONRequest(t, client, http.MethodGet, service.url+"/health", nil)
	if status != http.StatusOK {
		sdkFatalWithLogs(t, service, "expected /health to return 200, got %d: %s", status, strings.TrimSpace(string(payload)))
	}

	var healthResult struct {
		Status      string `json:"status"`
		BackendMode string `json:"backendMode"`
	}
	if err := json.Unmarshal(payload, &healthResult); err != nil {
		t.Fatalf("failed to parse /health response: %v", err)
	}
	if healthResult.Status != "ok" {
		t.Fatalf("expected health status 'ok', got: %q", healthResult.Status)
	}
}

// TestProtonDriveCliAuthFlow verifies that authentication works through
// the proton-drive-cli bridge subprocess.
func TestProtonDriveCliAuthFlow(t *testing.T) {
	root := repoRoot(t)
	service := startSDKService(t, root)
	username, password := sdkResolvedCredentials(t)
	client := &http.Client{Timeout: 30 * time.Second}

	// Test successful auth
	token := sdkInitToken(t, client, service, username, password)
	if token == "" {
		t.Fatal("expected non-empty token from /init")
	}

	// Test invalid credentials
	_, status := sdkJSONRequest(t, client, http.MethodPost, service.url+"/init", map[string]string{
		"username": "",
		"password": "",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty credentials, got %d", status)
	}
}

// TestProtonDriveCliUploadDownloadRoundTrip verifies upload and download through
// the proton-drive-cli bridge.
func TestProtonDriveCliUploadDownloadRoundTrip(t *testing.T) {
	root := repoRoot(t)
	service := startSDKService(t, root)
	username, password := sdkResolvedCredentials(t)
	client := &http.Client{Timeout: 30 * time.Second}

	token := sdkInitToken(t, client, service, username, password)

	// Create test file
	testContent := []byte("proton-drive-cli-integration-test-content")
	uploadPath := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(uploadPath, testContent, 0o600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Use a deterministic OID
	oid := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	// Upload
	uploadResp, uploadStatus := sdkJSONRequest(t, client, http.MethodPost, service.url+"/upload", map[string]string{
		"token": token,
		"oid":   oid,
		"path":  uploadPath,
	})
	if uploadStatus != http.StatusOK {
		sdkFatalWithLogs(t, service, "expected /upload 200, got %d: %s", uploadStatus, strings.TrimSpace(string(uploadResp)))
	}

	// Download
	downloadPath := filepath.Join(t.TempDir(), "download.bin")
	downloadResp, downloadStatus := sdkJSONRequest(t, client, http.MethodPost, service.url+"/download", map[string]string{
		"token":      token,
		"oid":        oid,
		"outputPath": downloadPath,
	})
	if downloadStatus != http.StatusOK {
		sdkFatalWithLogs(t, service, "expected /download 200, got %d: %s", downloadStatus, strings.TrimSpace(string(downloadResp)))
	}

	// Verify downloaded content matches
	downloaded, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(downloaded) != string(testContent) {
		t.Fatalf("content mismatch: expected %q, got %q", string(testContent), string(downloaded))
	}
}

// TestProtonDriveCliListFiles verifies the file listing works through
// the proton-drive-cli bridge.
func TestProtonDriveCliListFiles(t *testing.T) {
	root := repoRoot(t)
	service := startSDKService(t, root)
	username, password := sdkResolvedCredentials(t)
	client := &http.Client{Timeout: 30 * time.Second}

	token := sdkInitToken(t, client, service, username, password)

	listResp, listStatus := sdkJSONRequest(
		t,
		client,
		http.MethodGet,
		fmt.Sprintf("%s/list?token=%s&folder=LFS", service.url, token),
		nil,
	)
	if listStatus != http.StatusOK {
		sdkFatalWithLogs(t, service, "expected /list 200, got %d: %s", listStatus, strings.TrimSpace(string(listResp)))
	}

	var listResult struct {
		Files []map[string]any `json:"files"`
	}
	if err := json.Unmarshal(listResp, &listResult); err != nil {
		t.Fatalf("failed to parse /list response: %v", err)
	}
	if listResult.Files == nil {
		t.Fatalf("expected files array in /list response")
	}
}

// TestProtonDriveCliTokenRefresh verifies the token refresh works through
// the proton-drive-cli bridge.
func TestProtonDriveCliTokenRefresh(t *testing.T) {
	root := repoRoot(t)
	service := startSDKService(t, root)
	username, password := sdkResolvedCredentials(t)
	client := &http.Client{Timeout: 30 * time.Second}

	token := sdkInitToken(t, client, service, username, password)

	refreshResp, refreshStatus := sdkJSONRequest(t, client, http.MethodPost, service.url+"/refresh", map[string]string{
		"token": token,
	})
	if refreshStatus != http.StatusOK {
		sdkFatalWithLogs(t, service, "expected /refresh 200, got %d: %s", refreshStatus, strings.TrimSpace(string(refreshResp)))
	}

	var refreshResult struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(refreshResp, &refreshResult); err != nil {
		t.Fatalf("failed to parse /refresh response: %v", err)
	}
	if strings.TrimSpace(refreshResult.Token) == "" {
		t.Fatalf("expected non-empty token from /refresh")
	}
}

// TestProtonDriveCliErrorMapping verifies that errors from the TypeScript bridge
// are correctly mapped to HTTP status codes.
func TestProtonDriveCliErrorMapping(t *testing.T) {
	root := repoRoot(t)
	service := startSDKService(t, root)
	username, password := sdkResolvedCredentials(t)
	client := &http.Client{Timeout: 30 * time.Second}

	token := sdkInitToken(t, client, service, username, password)

	// Upload with invalid OID
	_, uploadStatus := sdkJSONRequest(t, client, http.MethodPost, service.url+"/upload", map[string]string{
		"token": token,
		"oid":   "invalid-oid",
		"path":  "/tmp/nonexistent.bin",
	})
	// Should fail with 400 (invalid OID) or 404 (file not found)
	if uploadStatus == http.StatusOK {
		t.Fatal("expected upload with invalid OID to fail")
	}

	// Download with missing token
	_, downloadStatus := sdkJSONRequest(t, client, http.MethodPost, service.url+"/download", map[string]string{
		"token":      "invalid-token",
		"oid":        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"outputPath": "/tmp/output.bin",
	})
	if downloadStatus == http.StatusOK {
		t.Fatal("expected download with invalid token to fail")
	}
}
