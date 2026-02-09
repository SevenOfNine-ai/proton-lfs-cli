package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func jsonHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestSDKServiceBackendRoundTripWithMockTransport(t *testing.T) {
	payload := []byte("sdk-backend-roundtrip")
	oidBytes := sha256.Sum256(payload)
	oid := hex.EncodeToString(oidBytes[:])

	uploadPath := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(uploadPath, payload, 0o600); err != nil {
		t.Fatalf("failed to create upload source: %v", err)
	}

	var initCalled bool
	var uploadCalled bool
	var downloadCalled bool

	client := NewSDKClient("http://sdk.local")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/init":
				initCalled = true
				var req map[string]string
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("failed to decode init request: %v", err)
				}
				if req["username"] != "user@proton.test" || req["password"] != "password" {
					t.Fatalf("unexpected init payload: %#v", req)
				}
				return jsonHTTPResponse(http.StatusOK, `{"token":"token-123"}`), nil
			case "/upload":
				uploadCalled = true
				var req map[string]string
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("failed to decode upload request: %v", err)
				}
				if req["token"] != "token-123" || req["oid"] != oid || req["path"] != uploadPath {
					t.Fatalf("unexpected upload payload: %#v", req)
				}
				return jsonHTTPResponse(http.StatusOK, `{}`), nil
			case "/download":
				downloadCalled = true
				var req map[string]string
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("failed to decode download request: %v", err)
				}
				if req["token"] != "token-123" || req["oid"] != oid {
					t.Fatalf("unexpected download payload: %#v", req)
				}
				if err := os.WriteFile(req["outputPath"], payload, 0o600); err != nil {
					t.Fatalf("failed to materialize mocked download output: %v", err)
				}
				return jsonHTTPResponse(http.StatusOK, `{}`), nil
			default:
				t.Fatalf("unexpected endpoint called: %s", r.URL.Path)
				return nil, nil
			}
		}),
	}

	backend := NewSDKServiceBackend(client, "user@proton.test", "password")
	session := &Session{Initialized: true}

	if err := backend.Initialize(session); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if session.Token != "token-123" {
		t.Fatalf("expected initialized token, got %q", session.Token)
	}

	uploadedSize, err := backend.Upload(session, oid, uploadPath, int64(len(payload)))
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if uploadedSize != int64(len(payload)) {
		t.Fatalf("unexpected upload size: %d", uploadedSize)
	}

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
		t.Fatalf("failed to read downloaded bytes: %v", err)
	}
	if string(downloadedBytes) != string(payload) {
		t.Fatal("downloaded bytes mismatch")
	}

	if !initCalled || !uploadCalled || !downloadCalled {
		t.Fatalf("expected all sdk endpoints to be called, got init=%v upload=%v download=%v", initCalled, uploadCalled, downloadCalled)
	}
}

func TestSDKServiceBackendInitializeRequiresCredentials(t *testing.T) {
	backend := NewSDKServiceBackend(NewSDKClient("http://sdk.local"), "", "")
	session := &Session{Initialized: true}

	err := backend.Initialize(session)
	code, message := backendErrorDetails(err)
	if code != 401 {
		t.Fatalf("expected credential error code 401, got %d (%v)", code, err)
	}
	if message != "proton credentials are required for sdk backend" {
		t.Fatalf("unexpected message: %q", message)
	}
}

func TestSDKServiceBackendUploadMapsNotFoundError(t *testing.T) {
	client := NewSDKClient("http://sdk.local")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/upload" {
				t.Fatalf("unexpected endpoint: %s", r.URL.Path)
			}
			return jsonHTTPResponse(http.StatusNotFound, `{"error":"File not found"}`), nil
		}),
	}

	backend := NewSDKServiceBackend(client, "unused", "unused")
	session := &Session{Initialized: true, Token: "token-123"}

	_, err := backend.Upload(session, validOID, "/tmp/does-not-exist", 0)
	code, _ := backendErrorDetails(err)
	if code != 404 {
		t.Fatalf("expected mapped not-found code 404, got %d (%v)", code, err)
	}
}

func TestSDKServiceBackendDownloadMapsAuthErrorAndCleansOutput(t *testing.T) {
	var requestedOutputPath string

	client := NewSDKClient("http://sdk.local")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/download" {
				t.Fatalf("unexpected endpoint: %s", r.URL.Path)
			}
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("failed to decode request: %v", err)
			}
			requestedOutputPath = req["outputPath"]
			return jsonHTTPResponse(http.StatusUnauthorized, `{"error":"Invalid or expired session"}`), nil
		}),
	}

	backend := NewSDKServiceBackend(client, "unused", "unused")
	session := &Session{Initialized: true, Token: "expired-token"}

	_, _, err := backend.Download(session, validOID)
	code, _ := backendErrorDetails(err)
	if code != 401 {
		t.Fatalf("expected mapped auth code 401, got %d (%v)", code, err)
	}
	if requestedOutputPath == "" {
		t.Fatal("expected outputPath to be sent in download request")
	}
	if _, statErr := os.Stat(requestedOutputPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected temporary download output to be cleaned up, stat err=%v", statErr)
	}
}
