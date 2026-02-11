package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validOID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func decodeAllMessages(t *testing.T, data []byte) []OutboundMessage {
	t.Helper()

	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	out := make([]OutboundMessage, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var msg OutboundMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			t.Fatalf("failed to decode message: %v", err)
		}
		out = append(out, msg)
	}
	return out
}

func configureLocalBackend(adapter *Adapter, storeDir string) {
	adapter.localStoreDir = storeDir
	adapter.backendKind = BackendLocal
	adapter.backend = NewLocalStoreBackend(storeDir)
}

func TestAdapterInit(t *testing.T) {
	adapter := NewAdapter()
	if adapter == nil {
		t.Fatal("failed to create adapter")
	}
	if adapter.allowMockTransfers {
		t.Fatal("mock transfers must be disabled by default")
	}
}

func TestInitResponseIsEmptyObject(t *testing.T) {
	adapter := NewAdapter()
	configureLocalBackend(adapter, t.TempDir())

	msg := InboundMessage{
		Event:               EventInit,
		Operation:           DirectionUpload,
		ConcurrentTransfers: 2,
	}

	buf := new(bytes.Buffer)
	if err := adapter.handleInit(&msg, json.NewEncoder(buf)); err != nil {
		t.Fatalf("handleInit returned error: %v", err)
	}

	if got := buf.String(); got != "{}\n" {
		t.Fatalf("expected init response to be empty object, got %q", got)
	}
}

func TestInitRejectsInvalidOperation(t *testing.T) {
	adapter := NewAdapter()
	msg := InboundMessage{
		Event:     EventInit,
		Operation: Direction("invalid"),
	}

	buf := new(bytes.Buffer)
	if err := adapter.handleInit(&msg, json.NewEncoder(buf)); err != nil {
		t.Fatalf("handleInit returned error: %v", err)
	}

	var out OutboundMessage
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &out); err != nil {
		t.Fatalf("failed to decode output: %v", err)
	}
	if out.Error == nil || out.Error.Code != 400 {
		t.Fatalf("expected protocol error with code 400, got %+v", out)
	}
}

func TestUploadFailsClosedWithoutMockMode(t *testing.T) {
	adapter := NewAdapter()
	configureLocalBackend(adapter, "")
	adapter.session = &Session{Initialized: true}

	tmpDir := t.TempDir()
	uploadPath := filepath.Join(tmpDir, "payload.bin")
	payload := []byte("payload")
	if err := os.WriteFile(uploadPath, payload, 0o600); err != nil {
		t.Fatalf("failed to create upload file: %v", err)
	}
	oid := sha256.Sum256(payload)

	msg := InboundMessage{
		Event: EventUpload,
		OID:   hex.EncodeToString(oid[:]),
		Size:  7,
		Path:  uploadPath,
	}

	buf := new(bytes.Buffer)
	if err := adapter.handleUpload(&msg, json.NewEncoder(buf)); err != nil {
		t.Fatalf("handleUpload returned error: %v", err)
	}

	var out OutboundMessage
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &out); err != nil {
		t.Fatalf("failed to decode output: %v", err)
	}
	if out.Error == nil || out.Error.Code != 501 {
		t.Fatalf("expected not-implemented error, got %+v", out)
	}
}

func TestUploadSucceedsInMockMode(t *testing.T) {
	adapter := NewAdapter()
	adapter.allowMockTransfers = true
	adapter.session = &Session{Initialized: true}

	tmpDir := t.TempDir()
	uploadPath := filepath.Join(tmpDir, "payload.bin")
	if err := os.WriteFile(uploadPath, []byte("payload"), 0o600); err != nil {
		t.Fatalf("failed to create upload file: %v", err)
	}

	msg := InboundMessage{
		Event: EventUpload,
		OID:   validOID,
		Size:  7,
		Path:  uploadPath,
	}

	buf := new(bytes.Buffer)
	if err := adapter.handleUpload(&msg, json.NewEncoder(buf)); err != nil {
		t.Fatalf("handleUpload returned error: %v", err)
	}

	out := decodeAllMessages(t, buf.Bytes())
	if len(out) != 2 {
		t.Fatalf("expected 2 output messages, got %d", len(out))
	}
	if out[0].Event != EventProgress {
		t.Fatalf("expected progress event first, got %+v", out[0])
	}
	if out[1].Event != EventComplete || out[1].Error != nil {
		t.Fatalf("expected successful completion, got %+v", out[1])
	}
}

func TestUploadRejectsInvalidOID(t *testing.T) {
	adapter := NewAdapter()
	adapter.allowMockTransfers = true
	adapter.session = &Session{Initialized: true}

	tmpDir := t.TempDir()
	uploadPath := filepath.Join(tmpDir, "payload.bin")
	if err := os.WriteFile(uploadPath, []byte("payload"), 0o600); err != nil {
		t.Fatalf("failed to create upload file: %v", err)
	}

	msg := InboundMessage{
		Event: EventUpload,
		OID:   "short-oid",
		Size:  7,
		Path:  uploadPath,
	}

	buf := new(bytes.Buffer)
	if err := adapter.handleUpload(&msg, json.NewEncoder(buf)); err != nil {
		t.Fatalf("handleUpload returned error: %v", err)
	}

	var out OutboundMessage
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &out); err != nil {
		t.Fatalf("failed to decode output: %v", err)
	}
	if out.Error == nil || out.Error.Code != 400 {
		t.Fatalf("expected validation error, got %+v", out)
	}
}

func TestUploadRejectsSizeMismatch(t *testing.T) {
	adapter := NewAdapter()
	adapter.allowMockTransfers = true
	adapter.session = &Session{Initialized: true}

	tmpDir := t.TempDir()
	uploadPath := filepath.Join(tmpDir, "payload.bin")
	if err := os.WriteFile(uploadPath, []byte("payload"), 0o600); err != nil {
		t.Fatalf("failed to create upload file: %v", err)
	}

	msg := InboundMessage{
		Event: EventUpload,
		OID:   validOID,
		Size:  999, // wrong size
		Path:  uploadPath,
	}

	buf := new(bytes.Buffer)
	if err := adapter.handleUpload(&msg, json.NewEncoder(buf)); err != nil {
		t.Fatalf("handleUpload returned error: %v", err)
	}

	var out OutboundMessage
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &out); err != nil {
		t.Fatalf("failed to decode output: %v", err)
	}
	if out.Error == nil || out.Error.Code != 409 {
		t.Fatalf("expected size mismatch conflict, got %+v", out)
	}
}

func TestUploadPersistsObjectToLocalStore(t *testing.T) {
	adapter := NewAdapter()
	configureLocalBackend(adapter, t.TempDir())
	adapter.session = &Session{Initialized: true}

	payload := []byte("real-upload-payload")
	oidBytes := sha256.Sum256(payload)
	oid := hex.EncodeToString(oidBytes[:])

	tmpDir := t.TempDir()
	uploadPath := filepath.Join(tmpDir, "payload.bin")
	if err := os.WriteFile(uploadPath, payload, 0o600); err != nil {
		t.Fatalf("failed to create upload file: %v", err)
	}

	msg := InboundMessage{
		Event: EventUpload,
		OID:   oid,
		Size:  int64(len(payload)),
		Path:  uploadPath,
	}

	buf := new(bytes.Buffer)
	if err := adapter.handleUpload(&msg, json.NewEncoder(buf)); err != nil {
		t.Fatalf("handleUpload returned error: %v", err)
	}

	out := decodeAllMessages(t, buf.Bytes())
	if len(out) != 2 || out[1].Error != nil {
		t.Fatalf("expected progress + successful completion, got %+v", out)
	}

	storedPath := adapter.localObjectPath(oid)
	storedBytes, err := os.ReadFile(storedPath)
	if err != nil {
		t.Fatalf("expected stored object file: %v", err)
	}
	if !bytes.Equal(storedBytes, payload) {
		t.Fatal("stored object bytes mismatch")
	}
}

func TestDownloadFromLocalStore(t *testing.T) {
	adapter := NewAdapter()
	configureLocalBackend(adapter, t.TempDir())
	adapter.session = &Session{Initialized: true}

	payload := []byte("download-roundtrip")
	oidBytes := sha256.Sum256(payload)
	oid := hex.EncodeToString(oidBytes[:])

	objectPath := adapter.localObjectPath(oid)
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o755); err != nil {
		t.Fatalf("failed to prepare object dir: %v", err)
	}
	if err := os.WriteFile(objectPath, payload, 0o600); err != nil {
		t.Fatalf("failed to seed object: %v", err)
	}

	msg := InboundMessage{
		Event: EventDownload,
		OID:   oid,
		Size:  int64(len(payload)),
	}

	buf := new(bytes.Buffer)
	if err := adapter.handleDownload(&msg, json.NewEncoder(buf)); err != nil {
		t.Fatalf("handleDownload returned error: %v", err)
	}

	out := decodeAllMessages(t, buf.Bytes())
	if len(out) != 2 || out[1].Error != nil {
		t.Fatalf("expected progress + successful completion, got %+v", out)
	}
	if out[1].Path == "" {
		t.Fatal("expected completion path")
	}

	downloadedBytes, err := os.ReadFile(out[1].Path)
	if err != nil {
		t.Fatalf("expected downloaded file: %v", err)
	}
	if !bytes.Equal(downloadedBytes, payload) {
		t.Fatal("downloaded object bytes mismatch")
	}
	_ = os.Remove(out[1].Path)
}

func TestDownloadRejectsCorruptStoredObject(t *testing.T) {
	adapter := NewAdapter()
	configureLocalBackend(adapter, t.TempDir())
	adapter.session = &Session{Initialized: true}

	objectPath := adapter.localObjectPath(validOID)
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o755); err != nil {
		t.Fatalf("failed to prepare object dir: %v", err)
	}
	if err := os.WriteFile(objectPath, []byte("wrong"), 0o600); err != nil {
		t.Fatalf("failed to seed corrupt object: %v", err)
	}

	msg := InboundMessage{
		Event: EventDownload,
		OID:   validOID,
		Size:  5,
	}

	buf := new(bytes.Buffer)
	if err := adapter.handleDownload(&msg, json.NewEncoder(buf)); err != nil {
		t.Fatalf("handleDownload returned error: %v", err)
	}

	var out OutboundMessage
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &out); err != nil {
		t.Fatalf("failed to decode output: %v", err)
	}
	if out.Error == nil || out.Error.Code != 500 {
		t.Fatalf("expected stored-object hash mismatch error, got %+v", out)
	}
}

func TestDownloadFailsClosedWithoutMockMode(t *testing.T) {
	adapter := NewAdapter()
	configureLocalBackend(adapter, "")
	adapter.session = &Session{Initialized: true}

	msg := InboundMessage{
		Event: EventDownload,
		OID:   validOID,
		Size:  32,
	}

	buf := new(bytes.Buffer)
	if err := adapter.handleDownload(&msg, json.NewEncoder(buf)); err != nil {
		t.Fatalf("handleDownload returned error: %v", err)
	}

	var out OutboundMessage
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &out); err != nil {
		t.Fatalf("failed to decode output: %v", err)
	}
	if out.Error == nil || out.Error.Code != 501 {
		t.Fatalf("expected not-implemented error, got %+v", out)
	}
}

func TestDownloadSucceedsInMockMode(t *testing.T) {
	adapter := NewAdapter()
	adapter.allowMockTransfers = true
	adapter.session = &Session{Initialized: true}

	msg := InboundMessage{
		Event: EventDownload,
		OID:   validOID,
		Size:  16,
	}

	buf := new(bytes.Buffer)
	if err := adapter.handleDownload(&msg, json.NewEncoder(buf)); err != nil {
		t.Fatalf("handleDownload returned error: %v", err)
	}

	out := decodeAllMessages(t, buf.Bytes())
	if len(out) != 2 {
		t.Fatalf("expected 2 output messages, got %d", len(out))
	}

	complete := out[1]
	if complete.Event != EventComplete || complete.Error != nil {
		t.Fatalf("expected successful completion, got %+v", complete)
	}
	if complete.Path == "" {
		t.Fatal("expected download path in completion event")
	}

	info, err := os.Stat(complete.Path)
	if err != nil {
		t.Fatalf("expected downloaded temp file: %v", err)
	}
	if info.Size() != 16 {
		t.Fatalf("expected temp file size 16, got %d", info.Size())
	}
	_ = os.Remove(complete.Path)
}

func TestUploadRejectsPathTraversal(t *testing.T) {
	adapter := NewAdapter()
	adapter.allowMockTransfers = true
	adapter.session = &Session{Initialized: true}

	cases := []struct {
		name string
		path string
	}{
		{"dot-dot segment", "/tmp/../etc/passwd"},
		{"dot-dot at start", "../secret"},
		{"dot-dot at end", "/tmp/foo/.."},
		{"backslash traversal", "/tmp/foo\\..\\bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := &InboundMessage{
				Event: EventUpload,
				OID:   validOID,
				Size:  1,
				Path:  tc.path,
			}
			err := adapter.validateTransferRequest(msg, true)
			if err == nil {
				t.Fatal("expected path traversal error")
			}
			if err.Error() != "path traversal not allowed" {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateFilePathAcceptsLegitPaths(t *testing.T) {
	cases := []string{
		"/tmp/git-lfs-objects/ab/cd/abcdef1234",
		"/home/user/.git/lfs/tmp/upload-1234",
		"relative/path/to/file.bin",
		"/var/folders/mr/some-deep/path",
	}
	for _, p := range cases {
		if err := validateFilePath(p); err != nil {
			t.Fatalf("path %q should be accepted, got: %v", p, err)
		}
	}
}

func TestUnknownEventHandling(t *testing.T) {
	adapter := NewAdapter()
	msg := InboundMessage{Event: "invalid-event"}

	buf := new(bytes.Buffer)
	if err := adapter.handleMessage(&msg, json.NewEncoder(buf)); err != nil {
		t.Fatalf("handleMessage returned error: %v", err)
	}

	var out OutboundMessage
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &out); err != nil {
		t.Fatalf("failed to decode output: %v", err)
	}
	if out.Error == nil || out.Error.Code != 400 {
		t.Fatalf("expected unknown-event error, got %+v", out)
	}
}

func TestPrintUsageContainsAllSections(t *testing.T) {
	var buf bytes.Buffer
	printUsage(&buf)
	output := buf.String()

	sections := []string{
		"NAME",
		"SYNOPSIS",
		"DESCRIPTION",
		"PROTOCOL COMPLIANCE",
		"BACKENDS",
		"CREDENTIAL PROVIDERS",
		"SECURITY",
		"FLAGS",
		"ENVIRONMENT VARIABLES",
		"EXAMPLES",
	}
	for _, section := range sections {
		if !strings.Contains(output, section) {
			t.Errorf("help output missing section %q", section)
		}
	}

	// Verify key content details
	details := []string{
		"git-lfs-proton-adapter",
		"lfs.standalonetransferagent",
		"proton-drive-cli",
		"PROTON_LFS_BACKEND",
		"--backend sdk",
	}
	for _, detail := range details {
		if !strings.Contains(output, detail) {
			t.Errorf("help output missing detail %q", detail)
		}
	}
}

func TestCleanupStaleTempFiles(t *testing.T) {
	// Create a temp file with the adapter prefix that looks stale
	staleFile, err := os.CreateTemp("", "git-lfs-proton-stale-test-*")
	if err != nil {
		t.Fatal(err)
	}
	stalePath := staleFile.Name()
	staleFile.Close()

	// Backdate the file to make it old enough to be cleaned up
	oldTime := time.Now().Add(-20 * time.Minute)
	os.Chtimes(stalePath, oldTime, oldTime)

	// Create a fresh temp file that should NOT be cleaned up (too new)
	freshFile, err := os.CreateTemp("", "git-lfs-proton-fresh-test-*")
	if err != nil {
		t.Fatal(err)
	}
	freshPath := freshFile.Name()
	freshFile.Close()
	defer os.Remove(freshPath)

	removed := cleanupStaleTempFiles(10 * time.Minute)

	if removed < 1 {
		t.Fatal("expected at least one stale file to be removed")
	}

	// Stale file should be gone
	if _, err := os.Stat(stalePath); err == nil {
		os.Remove(stalePath) // cleanup anyway
		t.Fatal("stale file should have been removed")
	}

	// Fresh file should still exist
	if _, err := os.Stat(freshPath); err != nil {
		t.Fatal("fresh file should still exist")
	}
}
