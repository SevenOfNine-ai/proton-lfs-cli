package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSpecSequenceUploadDownloadTerminate(t *testing.T) {
	adapter := NewAdapter("http://localhost:3000")
	adapter.allowMockTransfers = true

	tmpDir := t.TempDir()
	uploadPath := filepath.Join(tmpDir, "upload.bin")
	if err := os.WriteFile(uploadPath, []byte("data"), 0o600); err != nil {
		t.Fatalf("failed to create upload file: %v", err)
	}

	input := strings.Join([]string{
		`{"event":"init","operation":"upload","remote":"origin","concurrent":false,"concurrenttransfers":1}`,
		fmt.Sprintf(`{"event":"upload","oid":"%s","size":4,"path":%q,"action":null}`, validOID, uploadPath),
		fmt.Sprintf(`{"event":"download","oid":"%s","size":3,"action":null}`, validOID),
		`{"event":"terminate"}`,
	}, "\n") + "\n"

	out := new(bytes.Buffer)
	if err := adapter.Run(strings.NewReader(input), out); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	msgs := decodeAllMessages(t, out.Bytes())
	if len(msgs) != 5 {
		t.Fatalf("expected 5 responses (init + 2 upload + 2 download), got %d", len(msgs))
	}

	if msgs[0].Event != "" || msgs[0].Error != nil {
		t.Fatalf("expected empty init ack message, got %+v", msgs[0])
	}
	if msgs[1].Event != EventProgress || msgs[2].Event != EventComplete {
		t.Fatalf("unexpected upload message sequence: %+v %+v", msgs[1], msgs[2])
	}
	if msgs[3].Event != EventProgress || msgs[4].Event != EventComplete {
		t.Fatalf("unexpected download message sequence: %+v %+v", msgs[3], msgs[4])
	}
	if msgs[4].Path == "" {
		t.Fatal("download completion path must be set")
	}
	if _, err := os.Stat(msgs[4].Path); err != nil {
		t.Fatalf("download completion path does not exist: %v", err)
	}
	_ = os.Remove(msgs[4].Path)
}

func TestRunPerTransferErrorDoesNotTerminateProcess(t *testing.T) {
	adapter := NewAdapter("http://localhost:3000")
	adapter.allowMockTransfers = true

	tmpDir := t.TempDir()
	validUploadPath := filepath.Join(tmpDir, "valid.bin")
	if err := os.WriteFile(validUploadPath, []byte("value"), 0o600); err != nil {
		t.Fatalf("failed to create upload file: %v", err)
	}

	input := strings.Join([]string{
		`{"event":"init","operation":"upload","remote":"origin","concurrent":false,"concurrenttransfers":1}`,
		fmt.Sprintf(`{"event":"upload","oid":"%s","size":10,"path":"/does/not/exist"}`, validOID),
		fmt.Sprintf(`{"event":"upload","oid":"%s","size":5,"path":%q}`, validOID, validUploadPath),
		`{"event":"terminate"}`,
	}, "\n") + "\n"

	out := new(bytes.Buffer)
	if err := adapter.Run(strings.NewReader(input), out); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	msgs := decodeAllMessages(t, out.Bytes())
	if len(msgs) != 4 {
		t.Fatalf("expected 4 responses, got %d", len(msgs))
	}

	if msgs[1].Event != EventComplete || msgs[1].Error == nil {
		t.Fatalf("expected transfer-specific error completion, got %+v", msgs[1])
	}
	if msgs[2].Event != EventProgress || msgs[3].Event != EventComplete || msgs[3].Error != nil {
		t.Fatalf("expected second transfer to succeed, got %+v %+v", msgs[2], msgs[3])
	}
}

func TestRunInvalidInitReturnsProtocolErrorAndContinues(t *testing.T) {
	adapter := NewAdapter("http://localhost:3000")
	adapter.allowMockTransfers = true

	tmpDir := t.TempDir()
	uploadPath := filepath.Join(tmpDir, "upload.bin")
	if err := os.WriteFile(uploadPath, []byte("data"), 0o600); err != nil {
		t.Fatalf("failed to create upload file: %v", err)
	}

	input := strings.Join([]string{
		`{"event":"init","operation":"invalid","remote":"origin","concurrent":false,"concurrenttransfers":1}`,
		fmt.Sprintf(`{"event":"upload","oid":"%s","size":4,"path":%q}`, validOID, uploadPath),
		`{"event":"terminate"}`,
	}, "\n") + "\n"

	out := new(bytes.Buffer)
	if err := adapter.Run(strings.NewReader(input), out); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	msgs := decodeAllMessages(t, out.Bytes())
	if len(msgs) != 2 {
		t.Fatalf("expected 2 responses (protocol error + transfer error), got %d", len(msgs))
	}

	if msgs[0].Error == nil || msgs[0].Error.Code != 400 {
		t.Fatalf("expected init protocol error, got %+v", msgs[0])
	}
	if msgs[1].Event != EventComplete || msgs[1].Error == nil {
		t.Fatalf("expected transfer completion error after failed init, got %+v", msgs[1])
	}
}

func TestRunUploadProgressOrderingAndByteSemantics(t *testing.T) {
	adapter := NewAdapter("http://localhost:3000")
	configureLocalBackend(adapter, t.TempDir())

	tmpDir := t.TempDir()
	firstPayload := []byte("upload-one")
	secondPayload := []byte("upload-two-payload")

	firstPath := filepath.Join(tmpDir, "one.bin")
	if err := os.WriteFile(firstPath, firstPayload, 0o600); err != nil {
		t.Fatalf("failed to create first upload file: %v", err)
	}
	secondPath := filepath.Join(tmpDir, "two.bin")
	if err := os.WriteFile(secondPath, secondPayload, 0o600); err != nil {
		t.Fatalf("failed to create second upload file: %v", err)
	}

	firstOID := sha256.Sum256(firstPayload)
	secondOID := sha256.Sum256(secondPayload)
	firstOIDHex := hex.EncodeToString(firstOID[:])
	secondOIDHex := strings.ToUpper(hex.EncodeToString(secondOID[:]))

	input := strings.Join([]string{
		`{"event":"init","operation":"upload","remote":"origin","concurrent":false,"concurrenttransfers":1}`,
		fmt.Sprintf(`{"event":"upload","oid":"%s","size":%d,"path":%q}`, firstOIDHex, len(firstPayload), firstPath),
		fmt.Sprintf(`{"event":"upload","oid":"%s","size":%d,"path":%q}`, secondOIDHex, len(secondPayload), secondPath),
		`{"event":"terminate"}`,
	}, "\n") + "\n"

	out := new(bytes.Buffer)
	if err := adapter.Run(strings.NewReader(input), out); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	msgs := decodeAllMessages(t, out.Bytes())
	if len(msgs) != 5 {
		t.Fatalf("expected 5 responses (init + 2x progress/complete), got %d", len(msgs))
	}

	if msgs[0].Event != "" || msgs[0].Error != nil {
		t.Fatalf("expected empty init ack, got %+v", msgs[0])
	}

	if msgs[1].Event != EventProgress || msgs[2].Event != EventComplete {
		t.Fatalf("first upload must emit progress then complete, got %+v %+v", msgs[1], msgs[2])
	}
	if msgs[1].OID != firstOIDHex || msgs[2].OID != firstOIDHex {
		t.Fatalf("first upload oid mismatch between progress/complete: %+v %+v", msgs[1], msgs[2])
	}
	if msgs[1].BytesSoFar != int64(len(firstPayload)) || msgs[1].BytesSince != int64(len(firstPayload)) {
		t.Fatalf("first upload progress bytes mismatch, got %+v", msgs[1])
	}
	if msgs[2].Error != nil {
		t.Fatalf("expected first upload completion without error, got %+v", msgs[2])
	}

	secondNormalizedOID := strings.ToLower(secondOIDHex)
	if msgs[3].Event != EventProgress || msgs[4].Event != EventComplete {
		t.Fatalf("second upload must emit progress then complete, got %+v %+v", msgs[3], msgs[4])
	}
	if msgs[3].OID != secondNormalizedOID || msgs[4].OID != secondNormalizedOID {
		t.Fatalf("second upload oid mismatch between progress/complete: %+v %+v", msgs[3], msgs[4])
	}
	if msgs[3].BytesSoFar != int64(len(secondPayload)) || msgs[3].BytesSince != int64(len(secondPayload)) {
		t.Fatalf("second upload progress bytes mismatch, got %+v", msgs[3])
	}
	if msgs[4].Error != nil {
		t.Fatalf("expected second upload completion without error, got %+v", msgs[4])
	}
}

func TestRunDownloadProgressOrderingAndByteSemantics(t *testing.T) {
	adapter := NewAdapter("http://localhost:3000")
	configureLocalBackend(adapter, t.TempDir())

	payload := []byte("download-progress-bytes")
	oid := sha256.Sum256(payload)
	oidHex := strings.ToUpper(hex.EncodeToString(oid[:]))

	objectPath := adapter.localObjectPath(strings.ToLower(oidHex))
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o755); err != nil {
		t.Fatalf("failed to create object dir: %v", err)
	}
	if err := os.WriteFile(objectPath, payload, 0o600); err != nil {
		t.Fatalf("failed to seed object: %v", err)
	}

	input := strings.Join([]string{
		`{"event":"init","operation":"download","remote":"origin","concurrent":false,"concurrenttransfers":1}`,
		fmt.Sprintf(`{"event":"download","oid":"%s","size":%d,"action":null}`, oidHex, len(payload)),
		`{"event":"terminate"}`,
	}, "\n") + "\n"

	out := new(bytes.Buffer)
	if err := adapter.Run(strings.NewReader(input), out); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	msgs := decodeAllMessages(t, out.Bytes())
	if len(msgs) != 3 {
		t.Fatalf("expected 3 responses (init + progress + complete), got %d", len(msgs))
	}
	if msgs[1].Event != EventProgress || msgs[2].Event != EventComplete {
		t.Fatalf("download must emit progress then complete, got %+v %+v", msgs[1], msgs[2])
	}

	normalizedOID := strings.ToLower(oidHex)
	if msgs[1].OID != normalizedOID || msgs[2].OID != normalizedOID {
		t.Fatalf("download oid mismatch between progress/complete: %+v %+v", msgs[1], msgs[2])
	}
	if msgs[1].BytesSoFar != int64(len(payload)) || msgs[1].BytesSince != int64(len(payload)) {
		t.Fatalf("download progress bytes mismatch, got %+v", msgs[1])
	}
	if msgs[2].Error != nil {
		t.Fatalf("expected download completion without error, got %+v", msgs[2])
	}
	if msgs[2].Path == "" {
		t.Fatal("expected download completion path")
	}
	if _, err := os.Stat(msgs[2].Path); err != nil {
		t.Fatalf("expected download completion path to exist: %v", err)
	}
	_ = os.Remove(msgs[2].Path)
}

type failAfterNWriter struct {
	successfulWrites int
	failAt           int
}

func (w *failAfterNWriter) Write(p []byte) (int, error) {
	if w.successfulWrites >= w.failAt {
		return 0, io.ErrClosedPipe
	}
	w.successfulWrites++
	return len(p), nil
}

func TestRunReturnsErrorOnPartialWriteFailure(t *testing.T) {
	adapter := NewAdapter("http://localhost:3000")
	adapter.allowMockTransfers = true

	tmpDir := t.TempDir()
	uploadPath := filepath.Join(tmpDir, "upload.bin")
	if err := os.WriteFile(uploadPath, []byte("data"), 0o600); err != nil {
		t.Fatalf("failed to create upload file: %v", err)
	}

	input := strings.Join([]string{
		`{"event":"init","operation":"upload","remote":"origin","concurrent":false,"concurrenttransfers":1}`,
		fmt.Sprintf(`{"event":"upload","oid":"%s","size":4,"path":%q}`, validOID, uploadPath),
		`{"event":"terminate"}`,
	}, "\n") + "\n"

	writer := &failAfterNWriter{failAt: 1}
	err := adapter.Run(strings.NewReader(input), writer)
	if err == nil {
		t.Fatal("expected run to fail when output writer fails after init ack")
	}
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected closed pipe error, got: %v", err)
	}
}

func TestRunUploadMultiChunkProgressMonotonic(t *testing.T) {
	adapter := NewAdapter("http://localhost:3000")
	configureLocalBackend(adapter, t.TempDir())

	size := int(progressChunkSize*2 + 17)
	payload := bytes.Repeat([]byte("a"), size)
	oid := sha256.Sum256(payload)
	oidHex := hex.EncodeToString(oid[:])

	uploadPath := filepath.Join(t.TempDir(), "chunked-upload.bin")
	if err := os.WriteFile(uploadPath, payload, 0o600); err != nil {
		t.Fatalf("failed to create upload file: %v", err)
	}

	input := strings.Join([]string{
		`{"event":"init","operation":"upload","remote":"origin","concurrent":false,"concurrenttransfers":1}`,
		fmt.Sprintf(`{"event":"upload","oid":"%s","size":%d,"path":%q}`, oidHex, size, uploadPath),
		`{"event":"terminate"}`,
	}, "\n") + "\n"

	out := new(bytes.Buffer)
	if err := adapter.Run(strings.NewReader(input), out); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	msgs := decodeAllMessages(t, out.Bytes())
	expectedProgressCount := int((int64(size) + progressChunkSize - 1) / progressChunkSize)
	expectedTotal := 1 + expectedProgressCount + 1 // init ack + progress + complete
	if len(msgs) != expectedTotal {
		t.Fatalf("expected %d responses, got %d", expectedTotal, len(msgs))
	}

	lastBytes := int64(0)
	for i := 1; i <= expectedProgressCount; i++ {
		msg := msgs[i]
		if msg.Event != EventProgress {
			t.Fatalf("expected progress at index %d, got %+v", i, msg)
		}
		if msg.OID != oidHex {
			t.Fatalf("expected progress oid %s, got %s", oidHex, msg.OID)
		}
		if msg.BytesSoFar <= lastBytes {
			t.Fatalf("bytesSoFar must be monotonic, prev=%d current=%d", lastBytes, msg.BytesSoFar)
		}
		expectedDelta := msg.BytesSoFar - lastBytes
		if msg.BytesSince != expectedDelta {
			t.Fatalf("bytesSinceLast mismatch at index %d: got %d expected %d", i, msg.BytesSince, expectedDelta)
		}
		lastBytes = msg.BytesSoFar
	}
	if lastBytes != int64(size) {
		t.Fatalf("expected final bytesSoFar=%d, got %d", size, lastBytes)
	}

	complete := msgs[len(msgs)-1]
	if complete.Event != EventComplete || complete.Error != nil || complete.OID != oidHex {
		t.Fatalf("unexpected completion message: %+v", complete)
	}
}

func TestRunDownloadMultiChunkProgressMonotonic(t *testing.T) {
	adapter := NewAdapter("http://localhost:3000")
	configureLocalBackend(adapter, t.TempDir())

	size := int(progressChunkSize*3 + 11)
	payload := bytes.Repeat([]byte("b"), size)
	oid := sha256.Sum256(payload)
	oidHex := hex.EncodeToString(oid[:])

	objectPath := adapter.localObjectPath(oidHex)
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o755); err != nil {
		t.Fatalf("failed to create object dir: %v", err)
	}
	if err := os.WriteFile(objectPath, payload, 0o600); err != nil {
		t.Fatalf("failed to seed object: %v", err)
	}

	input := strings.Join([]string{
		`{"event":"init","operation":"download","remote":"origin","concurrent":false,"concurrenttransfers":1}`,
		fmt.Sprintf(`{"event":"download","oid":"%s","size":%d,"action":null}`, oidHex, size),
		`{"event":"terminate"}`,
	}, "\n") + "\n"

	out := new(bytes.Buffer)
	if err := adapter.Run(strings.NewReader(input), out); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	msgs := decodeAllMessages(t, out.Bytes())
	expectedProgressCount := int((int64(size) + progressChunkSize - 1) / progressChunkSize)
	expectedTotal := 1 + expectedProgressCount + 1 // init ack + progress + complete
	if len(msgs) != expectedTotal {
		t.Fatalf("expected %d responses, got %d", expectedTotal, len(msgs))
	}

	lastBytes := int64(0)
	for i := 1; i <= expectedProgressCount; i++ {
		msg := msgs[i]
		if msg.Event != EventProgress {
			t.Fatalf("expected progress at index %d, got %+v", i, msg)
		}
		if msg.OID != oidHex {
			t.Fatalf("expected progress oid %s, got %s", oidHex, msg.OID)
		}
		if msg.BytesSoFar <= lastBytes {
			t.Fatalf("bytesSoFar must be monotonic, prev=%d current=%d", lastBytes, msg.BytesSoFar)
		}
		expectedDelta := msg.BytesSoFar - lastBytes
		if msg.BytesSince != expectedDelta {
			t.Fatalf("bytesSinceLast mismatch at index %d: got %d expected %d", i, msg.BytesSince, expectedDelta)
		}
		lastBytes = msg.BytesSoFar
	}
	if lastBytes != int64(size) {
		t.Fatalf("expected final bytesSoFar=%d, got %d", size, lastBytes)
	}

	complete := msgs[len(msgs)-1]
	if complete.Event != EventComplete || complete.Error != nil || complete.OID != oidHex {
		t.Fatalf("unexpected completion message: %+v", complete)
	}
	if complete.Path == "" {
		t.Fatal("expected completion path for download")
	}
	_ = os.Remove(complete.Path)
}
