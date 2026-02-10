package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TransferBackend defines the storage/runtime backend used by adapter transfers.
type TransferBackend interface {
	Initialize(session *Session) error
	Upload(session *Session, oid, sourcePath string, expectedSize int64) (int64, error)
	Download(session *Session, oid string) (string, int64, error)
}

// BackendError maps backend-specific failures to protocol-safe transfer errors.
type BackendError struct {
	Code    int
	Message string
	Err     error
}

func (e *BackendError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func (e *BackendError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newBackendError(code int, message string, err error) error {
	return &BackendError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

func backendErrorDetails(err error) (int, string) {
	if err == nil {
		return 500, "transfer backend error"
	}
	var backendErr *BackendError
	if errors.As(err, &backendErr) {
		return backendErr.Code, backendErr.Message
	}
	return 500, "transfer backend error"
}

type LocalStoreBackend struct {
	storeDir string
}

func NewLocalStoreBackend(storeDir string) *LocalStoreBackend {
	return &LocalStoreBackend{
		storeDir: strings.TrimSpace(storeDir),
	}
}

func (b *LocalStoreBackend) Initialize(session *Session) error {
	if err := b.validateSession(session); err != nil {
		return err
	}
	if b.storeDir == "" {
		return newBackendError(501, "local store backend is not configured", nil)
	}
	if err := os.MkdirAll(b.storeDir, 0o700); err != nil {
		return newBackendError(500, "failed to prepare local object store", err)
	}
	return nil
}

func (b *LocalStoreBackend) Upload(session *Session, oid, sourcePath string, expectedSize int64) (int64, error) {
	if err := b.Initialize(session); err != nil {
		return 0, err
	}

	objectPath := b.objectPath(oid)
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o700); err != nil {
		return 0, newBackendError(500, "failed to prepare local object directory", err)
	}
	if err := copyFile(sourcePath, objectPath); err != nil {
		return 0, newBackendError(500, "failed to persist object in local store", err)
	}

	hash, size, err := calculateFileSHA256(objectPath)
	if err != nil {
		_ = os.Remove(objectPath)
		return 0, newBackendError(500, "failed to verify stored object", err)
	}
	if hash != oid {
		_ = os.Remove(objectPath)
		return 0, newBackendError(500, "stored object hash mismatch", nil)
	}
	if expectedSize > 0 && size != expectedSize {
		_ = os.Remove(objectPath)
		return 0, newBackendError(409, "stored object size does not match transfer request", nil)
	}
	return size, nil
}

func (b *LocalStoreBackend) Download(session *Session, oid string) (string, int64, error) {
	if err := b.validateSession(session); err != nil {
		return "", 0, err
	}
	if b.storeDir == "" {
		return "", 0, newBackendError(501, "local store backend is not configured", nil)
	}

	objectPath := b.objectPath(oid)
	hash, size, err := calculateFileSHA256(objectPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", 0, newBackendError(404, "object not found in local store", err)
		}
		return "", 0, newBackendError(500, "failed to read object from local store", err)
	}
	if hash != oid {
		return "", 0, newBackendError(500, "stored object hash mismatch", nil)
	}

	tmpFile, err := os.CreateTemp("", "git-lfs-proton-download-*")
	if err != nil {
		return "", 0, newBackendError(500, "failed to create temporary download file", err)
	}
	tmpPath := tmpFile.Name()

	if err := copyIntoOpenFile(objectPath, tmpFile); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return "", 0, newBackendError(500, "failed to stage object for download", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, newBackendError(500, "failed to finalize staged download object", err)
	}

	return tmpPath, size, nil
}

func (b *LocalStoreBackend) validateSession(session *Session) error {
	if session == nil || !session.Initialized {
		return newBackendError(500, "session not initialized", nil)
	}
	return nil
}

func (b *LocalStoreBackend) objectPath(oid string) string {
	return filepath.Join(b.storeDir, oid[:2], oid[2:4], oid)
}

type SDKServiceBackend struct {
	client   *SDKClient
	username string
	password string
}

func NewSDKServiceBackend(client *SDKClient, username, password string) *SDKServiceBackend {
	return &SDKServiceBackend{
		client:   client,
		username: strings.TrimSpace(username),
		password: strings.TrimSpace(password),
	}
}

func (b *SDKServiceBackend) Initialize(session *Session) error {
	if session == nil || !session.Initialized {
		return newBackendError(500, "session not initialized", nil)
	}
	if b.client == nil {
		return newBackendError(500, "sdk backend client is not configured", nil)
	}
	if b.username == "" || b.password == "" {
		return newBackendError(401, "proton credentials are required for sdk backend", nil)
	}

	token, err := b.client.InitializeSession(b.username, b.password)
	if err != nil {
		return mapSDKError(err, "failed to initialize sdk session")
	}
	session.Token = token
	return nil
}

func (b *SDKServiceBackend) Upload(session *Session, oid, sourcePath string, expectedSize int64) (int64, error) {
	if session == nil || !session.Initialized {
		return 0, newBackendError(500, "session not initialized", nil)
	}
	if session.Token == "" {
		return 0, newBackendError(401, "sdk session token is not initialized", nil)
	}
	if b.client == nil {
		return 0, newBackendError(500, "sdk backend client is not configured", nil)
	}

	if err := b.client.UploadFile(session.Token, oid, sourcePath); err != nil {
		return 0, mapSDKError(err, "sdk upload failed")
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, newBackendError(404, "upload source file not found", err)
		}
		return 0, newBackendError(500, "failed to stat upload source file", err)
	}
	if expectedSize > 0 && info.Size() != expectedSize {
		return 0, newBackendError(409, "upload size does not match transfer request", nil)
	}
	return info.Size(), nil
}

func (b *SDKServiceBackend) Download(session *Session, oid string) (string, int64, error) {
	if session == nil || !session.Initialized {
		return "", 0, newBackendError(500, "session not initialized", nil)
	}
	if session.Token == "" {
		return "", 0, newBackendError(401, "sdk session token is not initialized", nil)
	}
	if b.client == nil {
		return "", 0, newBackendError(500, "sdk backend client is not configured", nil)
	}

	tmpFile, err := os.CreateTemp("", "git-lfs-proton-download-*")
	if err != nil {
		return "", 0, newBackendError(500, "failed to create temporary download file", err)
	}
	tmpPath := tmpFile.Name()
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, newBackendError(500, "failed to create temporary download file", err)
	}

	if err := b.client.DownloadFile(session.Token, oid, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, mapSDKError(err, "sdk download failed")
	}

	info, err := os.Stat(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		if errors.Is(err, os.ErrNotExist) {
			return "", 0, newBackendError(500, "sdk backend did not materialize download output", err)
		}
		return "", 0, newBackendError(500, "failed to stat downloaded object", err)
	}

	return tmpPath, info.Size(), nil
}

func mapSDKError(err error, fallbackMessage string) error {
	if err == nil {
		return nil
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))

	switch {
	case strings.Contains(msg, "invalid or expired session"),
		strings.Contains(msg, "unauthorized"),
		strings.Contains(msg, "401"):
		return newBackendError(401, "sdk session is invalid or expired", err)
	case strings.Contains(msg, "not found"),
		strings.Contains(msg, "404"):
		return newBackendError(404, "object not found in sdk backend", err)
	case strings.Contains(msg, "timeout"),
		strings.Contains(msg, "timed out"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "no such host"),
		strings.Contains(msg, "dial tcp"):
		return newBackendError(503, "sdk service is unavailable", err)
	default:
		return newBackendError(502, fallbackMessage, err)
	}
}
