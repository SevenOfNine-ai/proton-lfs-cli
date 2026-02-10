package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	Version                 = "1.0.0"
	Name                    = "git-lfs-proton-adapter"
	progressChunkSize int64 = 64 * 1024
)

var (
	// Populated by build pipeline for release artifacts.
	GitCommit = "dev"
	BuildTime = "unknown"

	oidPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// Event types (from Git LFS custom transfer protocol)
const (
	EventInit      = "init"
	EventUpload    = "upload"
	EventDownload  = "download"
	EventProgress  = "progress"
	EventComplete  = "complete"
	EventTerminate = "terminate"
)

// Direction of transfer operation
type Direction string

const (
	DirectionUpload   Direction = "upload"
	DirectionDownload Direction = "download"
)

// Adapter manages the transfer session with Git LFS
type Adapter struct {
	sdkServiceURL      string
	logger             *log.Logger
	session            *Session
	currentOperation   Direction
	concurrentWorkers  int
	allowMockTransfers bool
	localStoreDir      string
	backendKind        string
	backend            TransferBackend
	protonUsername     string
	protonPassword     string
	protonPassCLIBin   string
	protonPassUserRef  string
	protonPassPassRef  string
}

// Message received from Git LFS
type InboundMessage struct {
	Event               string     `json:"event"`
	Operation           Direction  `json:"operation,omitempty"`
	Remote              string     `json:"remote,omitempty"`
	Concurrent          bool       `json:"concurrent,omitempty"`
	ConcurrentTransfers int        `json:"concurrenttransfers,omitempty"`
	OID                 string     `json:"oid,omitempty"`
	Size                int64      `json:"size,omitempty"`
	Path                string     `json:"path,omitempty"`
	Action              *ActionSet `json:"action,omitempty"`
}

// Message sent to Git LFS
type OutboundMessage struct {
	Event      string     `json:"event,omitempty"`
	OID        string     `json:"oid,omitempty"`
	Path       string     `json:"path,omitempty"`
	BytesSoFar int64      `json:"bytesSoFar,omitempty"`
	BytesSince int64      `json:"bytesSinceLast,omitempty"`
	Error      *ErrorInfo `json:"error,omitempty"`
}

// ActionSet contains transfer metadata from Git LFS batch API
type ActionSet struct {
	Href      string            `json:"href,omitempty"`
	ExpiresAt string            `json:"expiresAt,omitempty"`
	Header    map[string]string `json:"header,omitempty"`
}

// ErrorInfo represents an error response
type ErrorInfo struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Session manages authentication with Proton Drive
type Session struct {
	Initialized bool
	Token       string
	CreatedAt   time.Time
}

// NewAdapter creates a new adapter instance
func NewAdapter(sdkURL string) *Adapter {
	passRefRoot := passRefRootFromEnv()
	passUserRef := envOrDefault(EnvPassUsernameRef, defaultPassUsernameRef(passRefRoot))
	passPassRef := envOrDefault(EnvPassPasswordRef, defaultPassPasswordRef(passRefRoot))

	adapter := &Adapter{
		sdkServiceURL:      sdkURL,
		logger:             log.New(os.Stderr, Name+": ", log.LstdFlags),
		currentOperation:   "",
		concurrentWorkers:  DefaultConcurrentWorkers,
		allowMockTransfers: false,
		localStoreDir:      envTrim(EnvLocalStoreDir),
		backendKind:        BackendLocal,
		protonPassCLIBin:   envTrim(EnvPassCLIBin),
		protonPassUserRef:  passUserRef,
		protonPassPassRef:  passPassRef,
	}
	if adapter.protonPassCLIBin == "" {
		adapter.protonPassCLIBin = DefaultPassCLIBinary
	}
	adapter.backend = NewLocalStoreBackend(adapter.localStoreDir)
	return adapter
}

// Run starts the adapter's main message loop
func (a *Adapter) Run(r io.Reader, w io.Writer) error {
	decoder := json.NewDecoder(r)
	encoder := json.NewEncoder(w)

	for {
		var msg InboundMessage
		err := decoder.Decode(&msg)
		if err != nil {
			if err == io.EOF {
				return nil // Clean shutdown
			}
			return a.sendProtocolError(encoder, 1, "failed to decode message: "+err.Error())
		}

		if err := a.handleMessage(&msg, encoder); err != nil {
			a.logger.Printf("Error handling message: %v", err)
			return err
		}
	}
}

// handleMessage processes a single message from Git LFS
func (a *Adapter) handleMessage(msg *InboundMessage, enc *json.Encoder) error {
	switch msg.Event {
	case EventInit:
		return a.handleInit(msg, enc)
	case EventUpload:
		return a.handleUpload(msg, enc)
	case EventDownload:
		return a.handleDownload(msg, enc)
	case EventTerminate:
		return a.handleTerminate(msg, enc)
	default:
		return a.sendProtocolError(enc, 400, "unknown event: "+msg.Event)
	}
}

// handleInit initializes the transfer session
func (a *Adapter) handleInit(msg *InboundMessage, enc *json.Encoder) error {
	a.logger.Printf("Initializing adapter for %s operation", msg.Operation)

	if msg.Operation != DirectionUpload && msg.Operation != DirectionDownload {
		return a.sendProtocolError(enc, 400, "invalid operation for init")
	}

	a.currentOperation = msg.Operation
	a.concurrentWorkers = msg.ConcurrentTransfers
	if a.concurrentWorkers <= 0 {
		a.concurrentWorkers = DefaultConcurrentWorkers
	}

	// Initialize session with Proton LFS bridge
	a.session = &Session{
		Initialized: true,
		CreatedAt:   time.Now(),
	}

	if a.allowMockTransfers {
		return enc.Encode(OutboundMessage{})
	}

	if a.backend == nil {
		return a.sendProtocolError(enc, 500, "transfer backend is not configured")
	}
	if err := a.backend.Initialize(a.session); err != nil {
		a.session = nil
		code, message := backendErrorDetails(err)
		return a.sendProtocolError(enc, code, message)
	}

	// Send empty response to indicate success
	return enc.Encode(OutboundMessage{})
}

// handleUpload processes a file upload request
func (a *Adapter) handleUpload(msg *InboundMessage, enc *json.Encoder) error {
	a.logger.Printf("Upload request: OID=%s Size=%d Path=%s", msg.OID, msg.Size, msg.Path)

	if err := a.validateTransferRequest(msg, true); err != nil {
		return a.sendTransferError(enc, msg.OID, 400, err.Error())
	}

	if a.allowMockTransfers {
		return a.handleMockUpload(msg, enc)
	}

	if a.backend == nil {
		return a.sendTransferError(enc, msg.OID, 500, "transfer backend is not configured")
	}

	normalizedOID := strings.ToLower(msg.OID)
	hash, sourceSize, err := calculateFileSHA256(msg.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return a.sendTransferError(enc, msg.OID, 404, "upload source file not found")
		}
		return a.sendTransferError(enc, msg.OID, 500, "failed to read upload source file")
	}
	if msg.Size > 0 && sourceSize != msg.Size {
		return a.sendTransferError(enc, msg.OID, 409, "upload size does not match transfer request")
	}
	if hash != normalizedOID {
		return a.sendTransferError(enc, msg.OID, 409, "upload content hash does not match oid")
	}

	storedSize, err := a.backend.Upload(a.session, normalizedOID, msg.Path, sourceSize)
	if err != nil {
		code, message := backendErrorDetails(err)
		return a.sendTransferError(enc, msg.OID, code, message)
	}

	if err := a.sendProgressSequence(enc, normalizedOID, storedSize); err != nil {
		return err
	}

	return enc.Encode(OutboundMessage{
		Event: EventComplete,
		OID:   normalizedOID,
	})
}

// handleDownload processes a file download request
func (a *Adapter) handleDownload(msg *InboundMessage, enc *json.Encoder) error {
	a.logger.Printf("Download request: OID=%s Size=%d", msg.OID, msg.Size)

	if err := a.validateTransferRequest(msg, false); err != nil {
		return a.sendTransferError(enc, msg.OID, 400, err.Error())
	}

	if a.allowMockTransfers {
		return a.handleMockDownload(msg, enc)
	}

	if a.backend == nil {
		return a.sendTransferError(enc, msg.OID, 500, "transfer backend is not configured")
	}

	normalizedOID := strings.ToLower(msg.OID)
	stagedPath, stagedSize, err := a.backend.Download(a.session, normalizedOID)
	if err != nil {
		code, message := backendErrorDetails(err)
		return a.sendTransferError(enc, msg.OID, code, message)
	}

	objectHash, objectSize, err := calculateFileSHA256(stagedPath)
	if err != nil {
		_ = os.Remove(stagedPath)
		return a.sendTransferError(enc, msg.OID, 500, "failed to validate downloaded object")
	}
	if objectHash != normalizedOID {
		_ = os.Remove(stagedPath)
		return a.sendTransferError(enc, msg.OID, 500, "downloaded object hash mismatch")
	}
	if msg.Size > 0 && objectSize != msg.Size {
		_ = os.Remove(stagedPath)
		return a.sendTransferError(enc, msg.OID, 409, "downloaded object size does not match transfer request")
	}
	if stagedSize != objectSize {
		stagedSize = objectSize
	}

	if err := a.sendProgressSequence(enc, normalizedOID, stagedSize); err != nil {
		_ = os.Remove(stagedPath)
		return err
	}

	return enc.Encode(OutboundMessage{
		Event: EventComplete,
		OID:   normalizedOID,
		Path:  stagedPath,
	})
}

// handleTerminate closes the transfer session
func (a *Adapter) handleTerminate(_ *InboundMessage, _ *json.Encoder) error {
	a.logger.Println("Terminating adapter")
	a.session = nil
	return nil
}

func (a *Adapter) validateTransferRequest(msg *InboundMessage, requirePath bool) error {
	if a.session == nil || !a.session.Initialized {
		return errors.New("session not initialized")
	}
	if msg.Size < 0 {
		return errors.New("invalid transfer size")
	}
	if !oidPattern.MatchString(strings.ToLower(msg.OID)) {
		return errors.New("invalid oid format")
	}
	if requirePath && strings.TrimSpace(msg.Path) == "" {
		return errors.New("missing upload path")
	}
	return nil
}

func (a *Adapter) sendTransferError(enc *json.Encoder, oid string, code int, message string) error {
	a.logger.Printf("Error [%d]: %s", code, message)
	return enc.Encode(OutboundMessage{
		Event: EventComplete,
		OID:   oid,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
		},
	})
}

func (a *Adapter) sendProtocolError(enc *json.Encoder, code int, message string) error {
	a.logger.Printf("Protocol error [%d]: %s", code, message)
	return enc.Encode(OutboundMessage{
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
		},
	})
}

func (a *Adapter) sendProgress(enc *json.Encoder, oid string, size int64) error {
	return enc.Encode(OutboundMessage{
		Event:      EventProgress,
		OID:        oid,
		BytesSoFar: size,
		BytesSince: size,
	})
}

func (a *Adapter) sendProgressSequence(enc *json.Encoder, oid string, totalSize int64) error {
	if totalSize <= 0 {
		return a.sendProgress(enc, oid, 0)
	}

	var bytesSoFar int64
	for bytesSoFar < totalSize {
		nextBytes := bytesSoFar + progressChunkSize
		if nextBytes > totalSize {
			nextBytes = totalSize
		}
		if err := enc.Encode(OutboundMessage{
			Event:      EventProgress,
			OID:        oid,
			BytesSoFar: nextBytes,
			BytesSince: nextBytes - bytesSoFar,
		}); err != nil {
			return err
		}
		bytesSoFar = nextBytes
	}
	return nil
}

func (a *Adapter) localObjectPath(oid string) string {
	return filepath.Join(a.localStoreDir, oid[:2], oid[2:])
}

func (a *Adapter) handleMockUpload(msg *InboundMessage, enc *json.Encoder) error {
	info, err := os.Stat(msg.Path)
	if err != nil {
		return a.sendTransferError(enc, msg.OID, 404, "upload source file not found")
	}
	if msg.Size > 0 && info.Size() != msg.Size {
		return a.sendTransferError(enc, msg.OID, 409, "upload size does not match transfer request")
	}

	time.Sleep(100 * time.Millisecond)

	if err := a.sendProgressSequence(enc, strings.ToLower(msg.OID), info.Size()); err != nil {
		return err
	}

	return enc.Encode(OutboundMessage{
		Event: EventComplete,
		OID:   strings.ToLower(msg.OID),
	})
}

func (a *Adapter) handleMockDownload(msg *InboundMessage, enc *json.Encoder) error {
	tmpFile, err := a.createTempFile()
	if err != nil {
		return a.sendTransferError(enc, msg.OID, 500, "failed to create temp file: "+err.Error())
	}
	defer tmpFile.Close()

	if msg.Size > 0 {
		if err := tmpFile.Truncate(msg.Size); err != nil {
			return a.sendTransferError(enc, msg.OID, 500, "failed to allocate mock download file: "+err.Error())
		}
	}

	if err := a.sendProgressSequence(enc, strings.ToLower(msg.OID), msg.Size); err != nil {
		return err
	}

	return enc.Encode(OutboundMessage{
		Event: EventComplete,
		OID:   strings.ToLower(msg.OID),
		Path:  tmpFile.Name(),
	})
}

func calculateFileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func copyFile(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	tmpPath := fmt.Sprintf("%s.tmp-%d", dstPath, time.Now().UnixNano())
	dst, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := dst.Sync(); err != nil {
		_ = dst.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func copyIntoOpenFile(srcPath string, dst *os.File) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return dst.Sync()
}

// createTempFile creates a temporary file for downloads
func (a *Adapter) createTempFile() (*os.File, error) {
	return os.CreateTemp("", "git-lfs-proton-*")
}

func main() {
	sdkURL := flag.String("bridge-url", envOrDefault(EnvLFSBridgeURL, DefaultLFSBridgeURL), "URL to Proton LFS bridge service")
	defaultBackend := envTrim(EnvBackend)
	if defaultBackend == "" {
		defaultBackend = BackendLocal
	}
	backend := flag.String("backend", defaultBackend, "Transfer backend to use: local or sdk")
	allowMockTransfers := flag.Bool("allow-mock-transfers", envBoolOrDefault(EnvAllowMockTransfers, false), "Allow mock upload/download behavior (simulation only)")
	localStoreDir := flag.String("local-store-dir", envTrim(EnvLocalStoreDir), "Local object store directory used for standalone transfers")
	defaultPassCLIBin := envTrim(EnvPassCLIBin)
	if defaultPassCLIBin == "" {
		defaultPassCLIBin = DefaultPassCLIBinary
	}
	defaultPassRefRoot := passRefRootFromEnv()
	defaultPassUserRef := envOrDefault(EnvPassUsernameRef, defaultPassUsernameRef(defaultPassRefRoot))
	defaultPassPassRef := envOrDefault(EnvPassPasswordRef, defaultPassPasswordRef(defaultPassRefRoot))
	protonPassCLIBin := flag.String("proton-pass-cli", defaultPassCLIBin, "Path to pass-cli binary used to resolve sdk credentials")
	protonPassUserRef := flag.String("proton-pass-username-ref", defaultPassUserRef, "pass-cli secret reference for Proton username (e.g. pass://Vault/Item/username)")
	protonPassPassRef := flag.String("proton-pass-password-ref", defaultPassPassRef, "pass-cli secret reference for Proton password (e.g. pass://Vault/Item/password)")
	debug := flag.Bool("debug", false, "Enable debug logging")
	showVersion := flag.Bool("version", false, "Print version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("%s %s (commit=%s build_time=%s)\n", Name, Version, GitCommit, BuildTime)
		return
	}

	adapter := NewAdapter(*sdkURL)
	adapter.allowMockTransfers = *allowMockTransfers
	adapter.localStoreDir = strings.TrimSpace(*localStoreDir)
	adapter.backendKind = strings.ToLower(strings.TrimSpace(*backend))
	if adapter.backendKind == "" {
		adapter.backendKind = BackendLocal
	}
	adapter.protonPassCLIBin = strings.TrimSpace(*protonPassCLIBin)
	if adapter.protonPassCLIBin == "" {
		adapter.protonPassCLIBin = DefaultPassCLIBinary
	}
	adapter.protonPassUserRef = strings.TrimSpace(*protonPassUserRef)
	adapter.protonPassPassRef = strings.TrimSpace(*protonPassPassRef)

	if adapter.backendKind == BackendSDK {
		if err := adapter.resolveSDKCredentials(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to resolve sdk credentials: %v\n", err)
			os.Exit(2)
		}
	}

	switch adapter.backendKind {
	case BackendLocal:
		adapter.backend = NewLocalStoreBackend(adapter.localStoreDir)
	case BackendSDK:
		adapter.backend = NewSDKServiceBackend(
			NewSDKClient(adapter.sdkServiceURL),
			adapter.protonUsername,
			adapter.protonPassword,
		)
	default:
		fmt.Fprintf(os.Stderr, "invalid backend %q (supported: local, sdk)\n", adapter.backendKind)
		os.Exit(2)
	}

	if !*debug {
		adapter.logger.SetOutput(io.Discard)
	}

	// Read from stdin, write to stdout
	if err := adapter.Run(os.Stdin, os.Stdout); err != nil && err != io.EOF {
		adapter.logger.Fatalf("Adapter error: %v", err)
	}
}
