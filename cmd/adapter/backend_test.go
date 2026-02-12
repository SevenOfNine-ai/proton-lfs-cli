package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDriveCLIBackendRoundTrip(t *testing.T) {
	payload := []byte("drive-cli-roundtrip")
	oidBytes := sha256.Sum256(payload)
	oid := hex.EncodeToString(oidBytes[:])

	uploadPath := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(uploadPath, payload, 0o600); err != nil {
		t.Fatalf("failed to create upload source: %v", err)
	}

	// Use exists=false so upload doesn't skip via dedup
	bc := helperBridgeClient(t, "MOCK_BRIDGE_EXISTS_RESULT=false", "MOCK_BRIDGE_DOWNLOAD_CONTENT="+string(payload))
	backend := NewDriveCLIBackend(bc, "user@proton.test", "password")
	session := &Session{Initialized: true, CreatedAt: time.Now()}

	// Initialize (auth + init)
	if err := backend.Initialize(session); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if session.Token != "direct-bridge" {
		t.Fatalf("expected sentinel token, got %q", session.Token)
	}
	if !backend.authenticated {
		t.Fatal("expected authenticated=true")
	}

	// Upload
	uploadedSize, err := backend.Upload(session, oid, uploadPath, int64(len(payload)))
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if uploadedSize != int64(len(payload)) {
		t.Fatalf("unexpected upload size: %d", uploadedSize)
	}

	// Download
	downloadPath, downloadedSize, err := backend.Download(session, oid)
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	defer os.Remove(downloadPath)

	if downloadedSize != int64(len(payload)) {
		t.Fatalf("unexpected download size: %d", downloadedSize)
	}
	downloadedBytes, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(downloadedBytes) != string(payload) {
		t.Fatal("downloaded bytes mismatch")
	}
}

func TestDriveCLIBackendInitializeRequiresCredentials(t *testing.T) {
	bc := helperBridgeClient(t)
	backend := NewDriveCLIBackend(bc, "", "")
	session := &Session{Initialized: true, CreatedAt: time.Now()}

	err := backend.Initialize(session)
	code, message := backendErrorDetails(err)
	if code != 401 {
		t.Fatalf("expected credential error code 401, got %d (%v)", code, err)
	}
	if message != "proton credentials are required for sdk backend" {
		t.Fatalf("unexpected message: %q", message)
	}
}

func TestDriveCLIBackendUploadMapsNotFoundError(t *testing.T) {
	bc := helperBridgeClient(t,
		"MOCK_BRIDGE_EXISTS_RESULT=false",
		"MOCK_BRIDGE_ERROR=not found",
		"MOCK_BRIDGE_ERROR_CODE=404",
	)
	backend := NewDriveCLIBackend(bc, "unused", "unused")
	backend.authenticated = true

	session := &Session{Initialized: true, Token: "direct-bridge"}

	_, err := backend.Upload(session, validOID, "/tmp/does-not-exist", 0)
	code, _ := backendErrorDetails(err)
	if code != 404 {
		t.Fatalf("expected mapped not-found code 404, got %d (%v)", code, err)
	}
}

func TestDriveCLIBackendDownloadMapsAuthErrorAndCleansOutput(t *testing.T) {
	bc := helperBridgeClient(t,
		"MOCK_BRIDGE_ERROR=unauthorized",
		"MOCK_BRIDGE_ERROR_CODE=401",
	)
	backend := NewDriveCLIBackend(bc, "unused", "unused")
	backend.authenticated = true

	session := &Session{Initialized: true, Token: "direct-bridge"}

	_, _, err := backend.Download(session, validOID)
	code, _ := backendErrorDetails(err)
	if code != 401 {
		t.Fatalf("expected mapped auth code 401, got %d (%v)", code, err)
	}
}

func TestDriveCLIBackendUploadDedup(t *testing.T) {
	payload := []byte("dedup-test")
	oidBytes := sha256.Sum256(payload)
	oid := hex.EncodeToString(oidBytes[:])

	uploadPath := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(uploadPath, payload, 0o600); err != nil {
		t.Fatalf("failed to create upload file: %v", err)
	}

	// Mock bridge says exists=true, so upload should be skipped
	bc := helperBridgeClient(t)
	backend := NewDriveCLIBackend(bc, "user", "pass")
	backend.authenticated = true

	session := &Session{Initialized: true, Token: "direct-bridge"}

	size, err := backend.Upload(session, oid, uploadPath, int64(len(payload)))
	if err != nil {
		t.Fatalf("Upload should succeed with dedup: %v", err)
	}
	if size != int64(len(payload)) {
		t.Fatalf("unexpected size: %d", size)
	}
}

func TestDriveCLIBackendGitCredentialMode(t *testing.T) {
	bc := helperBridgeClient(t)
	backend := &DriveCLIBackend{
		bridge:             bc,
		credentialProvider: CredentialProviderGitCredential,
	}

	session := &Session{Initialized: true, CreatedAt: time.Now()}
	if err := backend.Initialize(session); err != nil {
		t.Fatalf("Initialize with git-credential failed: %v", err)
	}
	if session.Token != "direct-bridge" {
		t.Fatalf("expected sentinel token, got %q", session.Token)
	}
}

func TestMapBridgeError(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantCode int
	}{
		{"401 prefix", "[401] unauthorized", 401},
		{"404 prefix", "[404] not found", 404},
		{"503 prefix", "[503] service unavailable", 503},
		{"text 401", "unauthorized access", 401},
		{"text 404", "object not found", 404},
		{"text timeout", "request timed out", 503},
		{"text connection refused", "connection refused", 503},
		{"407 prefix", "[407] captcha verification required", 407},
		{"429 prefix", "[429] rate limited", 429},
		{"text captcha", "captcha verification required", 407},
		{"text rate limit", "rate limit exceeded", 429},
		{"concurrency limit", "bridge concurrency limit reached (10)", 503},
		{"unknown error", "something unexpected", 502},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := mapBridgeError(errors.New(tc.input), "fallback")
			code, _ := backendErrorDetails(err)
			if code != tc.wantCode {
				t.Fatalf("expected code %d, got %d for input %q", tc.wantCode, code, tc.input)
			}
		})
	}
}

func TestMapBridgeErrorNil(t *testing.T) {
	if err := mapBridgeError(nil, "fallback"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestDriveCLIBackendUploadNotAuthenticated(t *testing.T) {
	bc := helperBridgeClient(t)
	backend := NewDriveCLIBackend(bc, "user", "pass")
	// NOT authenticated

	session := &Session{Initialized: true, Token: "direct-bridge"}
	_, err := backend.Upload(session, validOID, "/tmp/test", 0)
	code, _ := backendErrorDetails(err)
	if code != 401 {
		t.Fatalf("expected 401, got %d (%v)", code, err)
	}
}

func TestDriveCLIBackendDownloadNotAuthenticated(t *testing.T) {
	bc := helperBridgeClient(t)
	backend := NewDriveCLIBackend(bc, "user", "pass")
	// NOT authenticated

	session := &Session{Initialized: true, Token: "direct-bridge"}
	_, _, err := backend.Download(session, validOID)
	code, _ := backendErrorDetails(err)
	if code != 401 {
		t.Fatalf("expected 401, got %d (%v)", code, err)
	}
}

func TestDriveCLIBackendInitializeCaptchaError(t *testing.T) {
	bc := helperBridgeClient(t,
		"MOCK_BRIDGE_ERROR=captcha verification required",
		"MOCK_BRIDGE_ERROR_CODE=407",
	)
	backend := NewDriveCLIBackend(bc, "user@proton.test", "password")
	session := &Session{Initialized: true, CreatedAt: time.Now()}
	err := backend.Initialize(session)
	code, _ := backendErrorDetails(err)
	if code != 407 {
		t.Fatalf("expected 407, got %d (%v)", code, err)
	}
}

func TestDriveCLIBackendInitializeRateLimitError(t *testing.T) {
	bc := helperBridgeClient(t,
		"MOCK_BRIDGE_ERROR=rate limited by proton api",
		"MOCK_BRIDGE_ERROR_CODE=429",
	)
	backend := NewDriveCLIBackend(bc, "user@proton.test", "password")
	session := &Session{Initialized: true, CreatedAt: time.Now()}
	err := backend.Initialize(session)
	code, _ := backendErrorDetails(err)
	if code != 429 {
		t.Fatalf("expected 429, got %d (%v)", code, err)
	}
}

func TestDriveCLIBackendZeroCredentials(t *testing.T) {
	backend := NewDriveCLIBackend(nil, "secret-user", "secret-pass")
	backend.ZeroCredentials()

	// All bytes should be zero
	for _, b := range backend.username {
		if b != 0 {
			t.Fatal("username byte not zeroed")
		}
	}
	for _, b := range backend.password {
		if b != 0 {
			t.Fatal("password byte not zeroed")
		}
	}
}
