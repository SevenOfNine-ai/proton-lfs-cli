//go:build integration

package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	defaultPassRefRoot = "pass://Personal/Proton Git LFS"
	envLFSBridgeURL    = "PROTON_LFS_BRIDGE_URL"
)

type sdkServiceInstance struct {
	url         string
	storagePath string
	logPath     string
	external    bool
}

func sdkPassCLIPath() string {
	passCLIBin := strings.TrimSpace(os.Getenv("PROTON_PASS_CLI_BIN"))
	if passCLIBin == "" {
		passCLIBin = "pass-cli"
	}
	return passCLIBin
}

func sdkExternalServiceURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv(envLFSBridgeURL)), "/")
}

func sdkPassRefConfig() (passRefRoot, usernameRef, passwordRef string) {
	passRefRoot = strings.TrimRight(strings.TrimSpace(os.Getenv("PROTON_PASS_REF_ROOT")), "/")
	if passRefRoot == "" {
		passRefRoot = defaultPassRefRoot
	}

	usernameRef = strings.TrimSpace(os.Getenv("PROTON_PASS_USERNAME_REF"))
	if usernameRef == "" {
		usernameRef = passRefRoot + "/username"
	}
	passwordRef = strings.TrimSpace(os.Getenv("PROTON_PASS_PASSWORD_REF"))
	if passwordRef == "" {
		passwordRef = passRefRoot + "/password"
	}

	return passRefRoot, usernameRef, passwordRef
}

func sdkCredentialEnv(t *testing.T, base []string) []string {
	t.Helper()

	passCLIBin := sdkPassCLIPath()
	if strings.Contains(passCLIBin, string(os.PathSeparator)) {
		if _, err := os.Stat(passCLIBin); err != nil {
			t.Skipf("sdk integration test skipped: PROTON_PASS_CLI_BIN=%s is not usable: %v", passCLIBin, err)
		}
	} else if _, err := exec.LookPath(passCLIBin); err != nil {
		t.Skipf("sdk integration test skipped: pass-cli binary not found: %s", passCLIBin)
	}

	passRefRoot, usernameRef, passwordRef := sdkPassRefConfig()

	return append(
		base,
		"PROTON_PASS_CLI_BIN="+passCLIBin,
		"PROTON_PASS_REF_ROOT="+passRefRoot,
		"PROTON_PASS_USERNAME_REF="+usernameRef,
		"PROTON_PASS_PASSWORD_REF="+passwordRef,
	)
}

func availableTCPPort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate tcp port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitForSDKServiceHealth(serviceURL string, timeout time.Duration) error {
	httpClient := &http.Client{Timeout: 700 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := httpClient.Get(serviceURL + "/health")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("sdk service did not become healthy at %s", serviceURL)
}

func startSDKService(t *testing.T, root string) sdkServiceInstance {
	t.Helper()

	if externalURL := sdkExternalServiceURL(); externalURL != "" {
		if err := waitForSDKServiceHealth(externalURL, 12*time.Second); err != nil {
			t.Fatalf(
				"sdk integration test expected external service from %s=%q, but health checks failed: %v",
				envLFSBridgeURL,
				externalURL,
				err,
			)
		}
		return sdkServiceInstance{
			url:      externalURL,
			external: true,
		}
	}

	backendMode := strings.ToLower(strings.TrimSpace(os.Getenv("SDK_BACKEND_MODE")))
	if backendMode == "real" || backendMode == "proton-drive-cli" {
		// proton-drive-cli bridge mode: verify the CLI is built
		driveCliBin := strings.TrimSpace(os.Getenv("PROTON_DRIVE_CLI_BIN"))
		if driveCliBin == "" {
			driveCliBin = filepath.Join(root, "submodules", "proton-drive-cli", "dist", "index.js")
		}
		if _, err := os.Stat(driveCliBin); err != nil {
			t.Skipf("sdk integration test skipped: proton-drive-cli not built at %s: %v (run: make build-drive-cli)", driveCliBin, err)
		}
	}

	nodeBin, err := findToolBinary(root, "NODE_BIN", "node")
	if err != nil {
		t.Skipf("sdk integration test skipped: %v", err)
	}

	serviceDir := filepath.Join(root, "proton-lfs-bridge")
	depCheck := exec.Command(nodeBin, "-e", "require.resolve('express')")
	depCheck.Dir = serviceDir
	if out, err := depCheck.CombinedOutput(); err != nil {
		t.Skipf(
			"sdk integration test skipped: proton-lfs-bridge dependencies missing (%v); run `yarn install` from repository root (or `make setup`) [details: %s]",
			err,
			strings.TrimSpace(string(out)),
		)
	}

	port := availableTCPPort(t)
	serviceURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	storagePath := filepath.Join(t.TempDir(), "sdk-storage")
	logPath := filepath.Join(t.TempDir(), "sdk-service.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("failed to create sdk service log file: %v", err)
	}

	cmd := exec.Command(nodeBin, "server.js")
	cmd.Dir = serviceDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(
		os.Environ(),
		"LFS_BRIDGE_PORT="+strconv.Itoa(port),
		"SDK_STORAGE_DIR="+storagePath,
		"LOG_LEVEL=debug",
		"NODE_ENV=test",
	)

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("failed to start sdk service: %v", err)
	}

	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		_ = logFile.Close()
	})

	if err := waitForSDKServiceHealth(serviceURL, 12*time.Second); err != nil {
		logBytes, _ := os.ReadFile(logPath)
		t.Fatalf("%v\nlogs:\n%s", err, string(logBytes))
	}

	return sdkServiceInstance{
		url:         serviceURL,
		storagePath: storagePath,
		logPath:     logPath,
		external:    false,
	}
}

func configureSDKCustomTransfer(t *testing.T, repoPath string, env []string, gitBin, adapterPath, serviceURL string) {
	t.Helper()

	sdkArgs := fmt.Sprintf("--backend=sdk --bridge-url=%s", serviceURL)
	mustRun(t, repoPath, env, gitBin, "config", "lfs.customtransfer.proton.path", adapterPath)
	mustRun(t, repoPath, env, gitBin, "config", "lfs.customtransfer.proton.args", sdkArgs)
	mustRun(t, repoPath, env, gitBin, "config", "lfs.customtransfer.proton.concurrent", "false")
	mustRun(t, repoPath, env, gitBin, "config", "lfs.customtransfer.proton.direction", "both")
	mustRun(t, repoPath, env, gitBin, "config", "lfs.standalonetransferagent", "proton")
}

func parsePassCLISecret(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
		if value := jsonStringValue(decoded); value != "" {
			return value
		}
	}

	lines := strings.Split(trimmed, "\n")
	nonEmpty := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			if key == "value" || key == "secret" {
				line = strings.TrimSpace(parts[1])
			}
		}
		nonEmpty = append(nonEmpty, line)
	}

	if len(nonEmpty) == 0 {
		return ""
	}
	if len(nonEmpty) == 1 {
		return strings.TrimSpace(nonEmpty[0])
	}
	return strings.TrimSpace(nonEmpty[len(nonEmpty)-1])
}

func jsonStringValue(v any) string {
	switch typed := v.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"value", "secret", "content", "data", "text"} {
			if raw, ok := typed[key]; ok {
				if value := jsonStringValue(raw); value != "" {
					return value
				}
			}
		}
	}
	return ""
}

func sdkReadPassCLISecret(t *testing.T, passCLIBin, reference string) string {
	t.Helper()

	cmd := exec.Command(passCLIBin, "item", "view", "--output", "json", reference)
	out, err := cmd.CombinedOutput()
	if err == nil {
		if value := parsePassCLISecret(string(out)); value != "" {
			return value
		}
	}

	cmd = exec.Command(passCLIBin, "item", "view", reference)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Skipf("sdk integration test skipped: unable to resolve pass reference %q: %v [%s]", reference, err, strings.TrimSpace(string(out)))
	}
	value := parsePassCLISecret(string(out))
	if value == "" {
		t.Skipf("sdk integration test skipped: empty secret value for pass reference %q", reference)
	}
	return value
}

func sdkResolvedCredentials(t *testing.T) (string, string) {
	t.Helper()

	passCLIBin := sdkPassCLIPath()
	if strings.Contains(passCLIBin, string(os.PathSeparator)) {
		if _, err := os.Stat(passCLIBin); err != nil {
			t.Skipf("sdk integration test skipped: PROTON_PASS_CLI_BIN=%s is not usable: %v", passCLIBin, err)
		}
	} else if _, err := exec.LookPath(passCLIBin); err != nil {
		t.Skipf("sdk integration test skipped: pass-cli binary not found: %s", passCLIBin)
	}

	_, usernameRef, passwordRef := sdkPassRefConfig()
	username := sdkReadPassCLISecret(t, passCLIBin, usernameRef)
	password := sdkReadPassCLISecret(t, passCLIBin, passwordRef)
	return username, password
}

func sdkJSONRequest(t *testing.T, client *http.Client, method, endpoint string, body any) ([]byte, int) {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request payload for %s: %v", endpoint, err)
		}
		reqBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, endpoint, reqBody)
	if err != nil {
		t.Fatalf("failed to create request for %s: %v", endpoint, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed for %s: %v", endpoint, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body for %s: %v", endpoint, err)
	}
	return payload, resp.StatusCode
}

func sdkServiceLogTail(service sdkServiceInstance) string {
	if service.external || strings.TrimSpace(service.logPath) == "" {
		return ""
	}
	logBytes, err := os.ReadFile(service.logPath)
	if err != nil || len(logBytes) == 0 {
		return ""
	}
	trimmed := strings.TrimSpace(string(logBytes))
	if trimmed == "" {
		return ""
	}
	return trimmed
}

func sdkFatalWithLogs(t *testing.T, service sdkServiceInstance, format string, args ...any) {
	t.Helper()
	baseMessage := fmt.Sprintf(format, args...)
	if logs := sdkServiceLogTail(service); logs != "" {
		t.Fatalf("%s\nsdk service logs:\n%s", baseMessage, logs)
	}
	t.Fatalf("%s", baseMessage)
}

func sdkInitToken(t *testing.T, client *http.Client, service sdkServiceInstance, username, password string) string {
	t.Helper()

	payload, status := sdkJSONRequest(t, client, http.MethodPost, service.url+"/init", map[string]string{
		"username": username,
		"password": password,
	})
	if status != http.StatusOK {
		sdkFatalWithLogs(t, service, "expected /init to return 200, got %d: %s", status, strings.TrimSpace(string(payload)))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("failed to parse /init response: %v (%s)", err, strings.TrimSpace(string(payload)))
	}
	if strings.TrimSpace(result.Token) == "" {
		t.Fatalf("expected non-empty token from /init response: %s", strings.TrimSpace(string(payload)))
	}
	return strings.TrimSpace(result.Token)
}

func TestSDKServiceAPIContractRoundTrip(t *testing.T) {
	root := repoRoot(t)
	service := startSDKService(t, root)
	username, password := sdkResolvedCredentials(t)
	client := &http.Client{Timeout: 15 * time.Second}

	token := sdkInitToken(t, client, service, username, password)

	sourceBytes := []byte("proton-lfs-bridge-api-contract-roundtrip")
	oidHash := sha256.Sum256(sourceBytes)
	oid := hex.EncodeToString(oidHash[:])

	uploadPath := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(uploadPath, sourceBytes, 0o600); err != nil {
		t.Fatalf("failed to create upload payload: %v", err)
	}

	uploadResp, uploadStatus := sdkJSONRequest(t, client, http.MethodPost, service.url+"/upload", map[string]string{
		"token": token,
		"oid":   oid,
		"path":  uploadPath,
	})
	if uploadStatus != http.StatusOK {
		t.Fatalf("expected /upload to return 200, got %d: %s", uploadStatus, strings.TrimSpace(string(uploadResp)))
	}

	downloadPath := filepath.Join(t.TempDir(), "download.bin")
	downloadResp, downloadStatus := sdkJSONRequest(t, client, http.MethodPost, service.url+"/download", map[string]string{
		"token":      token,
		"oid":        oid,
		"outputPath": downloadPath,
	})
	if downloadStatus != http.StatusOK {
		t.Fatalf("expected /download to return 200, got %d: %s", downloadStatus, strings.TrimSpace(string(downloadResp)))
	}

	downloadedBytes, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if !bytes.Equal(downloadedBytes, sourceBytes) {
		t.Fatalf("downloaded bytes mismatch; expected %q got %q", string(sourceBytes), string(downloadedBytes))
	}

	refreshResp, refreshStatus := sdkJSONRequest(t, client, http.MethodPost, service.url+"/refresh", map[string]string{
		"token": token,
	})
	if refreshStatus != http.StatusOK {
		t.Fatalf("expected /refresh to return 200, got %d: %s", refreshStatus, strings.TrimSpace(string(refreshResp)))
	}

	var refreshResult struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(refreshResp, &refreshResult); err != nil {
		t.Fatalf("failed to parse /refresh response: %v (%s)", err, strings.TrimSpace(string(refreshResp)))
	}
	if strings.TrimSpace(refreshResult.Token) == "" {
		t.Fatalf("expected non-empty token from /refresh: %s", strings.TrimSpace(string(refreshResp)))
	}

	listPayload, listStatus := sdkJSONRequest(
		t,
		client,
		http.MethodGet,
		fmt.Sprintf("%s/list?token=%s&folder=LFS", service.url, url.QueryEscape(refreshResult.Token)),
		nil,
	)
	if listStatus != http.StatusOK {
		t.Fatalf("expected /list to return 200, got %d: %s", listStatus, strings.TrimSpace(string(listPayload)))
	}

	var listResult struct {
		Files []map[string]any `json:"files"`
	}
	if err := json.Unmarshal(listPayload, &listResult); err != nil {
		t.Fatalf("failed to parse /list response: %v (%s)", err, strings.TrimSpace(string(listPayload)))
	}
	if listResult.Files == nil {
		t.Fatalf("expected /list response to include files array: %s", strings.TrimSpace(string(listPayload)))
	}
}

func TestGitLFSCustomTransferSDKBackendRoundTrip(t *testing.T) {
	s := setupRepositoryForUpload(t)

	service := startSDKService(t, s.root)
	sdkEnv := sdkCredentialEnv(t, s.env)

	configureSDKCustomTransfer(t, s.repoPath, sdkEnv, s.gitBin, s.adapterPath, service.url)

	lsFilesOutput := mustRun(t, s.repoPath, sdkEnv, s.gitLFSBin, "ls-files", "-l")
	fields := strings.Fields(strings.TrimSpace(lsFilesOutput))
	if len(fields) == 0 {
		t.Fatalf("expected oid in git lfs ls-files output, got:\n%s", lsFilesOutput)
	}
	oid := fields[0]
	if len(oid) != 64 {
		t.Fatalf("expected oid in git lfs ls-files output, got: %q", oid)
	}

	mustRun(t, s.repoPath, sdkEnv, s.gitBin, "push", "origin", "main")
	lfsPushOutput := mustRun(t, s.repoPath, sdkEnv, s.gitLFSBin, "push", "origin", "main")
	if strings.Contains(strings.ToLower(lfsPushOutput), "error") {
		t.Fatalf("unexpected error in lfs push output:\n%s", lfsPushOutput)
	}

	if !service.external {
		storedPath := filepath.Join(service.storagePath, oid[:2], oid[2:])
		if _, err := os.Stat(storedPath); err != nil {
			t.Fatalf("expected uploaded object in sdk storage, path=%s err=%v", storedPath, err)
		}
	}

	cloneBase := t.TempDir()
	clonePath := filepath.Join(cloneBase, "clone")
	cloneEnv := append(sdkEnv, "GIT_LFS_SKIP_SMUDGE=1")
	mustRun(t, cloneBase, cloneEnv, s.gitBin, "clone", s.remotePath, clonePath)

	mustRun(t, clonePath, sdkEnv, s.gitLFSBin, "install", "--local")
	configureSDKCustomTransfer(t, clonePath, sdkEnv, s.gitBin, s.adapterPath, service.url)

	out, err := runCmd(clonePath, sdkEnv, s.gitLFSBin, "pull", "origin", "main")
	if err != nil {
		t.Fatalf("expected lfs pull to succeed, err: %v\noutput:\n%s", err, out)
	}

	artifactPath := filepath.Join(clonePath, "artifact.bin")
	contents, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("failed to read pulled artifact: %v", err)
	}
	if string(contents) != "proton-git-lfs-integration" {
		t.Fatalf("unexpected pulled artifact bytes: %q", string(contents))
	}
}
