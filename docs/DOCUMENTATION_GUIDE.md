# Go Documentation Guide

## Overview

Go has built-in documentation support via `godoc` and `go doc`. This guide shows how to write effective documentation comments for all main functions and CLI commands.

## Quick Start

```bash

# View documentation for a package

go doc ./cmd/adapter

# View documentation for a function

go doc ./cmd/adapter.handleUploadBatch

# Serve documentation locally (Go 1.21+)

go run golang.org/x/pkgsite/cmd/pkgsite@latest -open .

# Or use traditional godoc (Go <1.21)

godoc -http=:6060

# Open http://localhost:6060

```

## Go Doc Comment Syntax

### Package Documentation

```go
// Package adapter implements the Git LFS custom transfer protocol for Proton Drive.
//
// The adapter acts as a bridge between Git LFS and Proton Drive, providing
// encrypted storage for Git LFS objects. It supports two backend modes:
//
//   - local: Local filesystem storage for testing
//   - sdk: Proton Drive integration via proton-drive-cli subprocess
//
// # Environment Variables
//
// The adapter can be configured via environment variables:
//
//   - PROTON_LFS_BACKEND: Backend mode (local or sdk, default: local)
//   - PROTON_CREDENTIAL_PROVIDER: Credential provider (pass-cli or git-credential)
//   - PROTON_LFS_STATUS_FILE: Path to status file (default: ~/.proton-git-lfs/status.json)
//
// # Example Usage
//
// Configure Git LFS to use the adapter:
//
//    git config lfs.standalonetransferagent proton
//    git config lfs.customtransfer.proton.path git-lfs-proton-adapter
//
// # Protocol
//
// The adapter implements the Git LFS custom transfer protocol (v3) using
// JSON messages over stdin/stdout. See:
// https://github.com/git-lfs/git-lfs/blob/main/docs/custom-transfers.md
package adapter

```

### Function Documentation

```go
// UploadFile uploads a local file to Proton Drive with the given OID.
//
// The function encrypts the file locally before uploading to Proton Drive.
// It uses the configured credential provider to authenticate and writes
// status updates to the status file.
//
// Parameters:
//   - oid: SHA-256 hash of the file (64 hex characters)
//   - localPath: Path to the local file to upload
//   - remotePath: Destination path in Proton Drive (e.g., /LFS/ab/c1/abc123...)
//
// Returns:
//   - error: nil on success, or an error describing the failure
//
// The function performs the following steps:
//  1. Validates OID format (must be 64 hex characters)
//  2. Checks if file exists and is readable
//  3. Spawns proton-drive-cli subprocess with credential provider
//  4. Sends upload request via stdin (JSON)
//  5. Reads response from stdout
//  6. Updates status file with result
//
// Errors:
//   - Returns ErrInvalidOID if OID format is invalid
//   - Returns ErrFileNotFound if local file doesn't exist
//   - Returns ErrRateLimited if Proton API rate limit is exceeded
//   - Returns ErrAuthRequired if authentication fails
//   - Returns ErrCaptchaRequired if CAPTCHA verification is needed
//
// Example:
//
//    err := UploadFile("abc123...", "/tmp/file.bin", "/LFS/ab/c1/abc123...")
//    if err != nil {
//        log.Fatalf("Upload failed: %v", err)
//    }
//
// See also:
//   - DownloadFile for downloading files
//   - validateOID for OID validation logic
func UploadFile(oid, localPath, remotePath string) error {
    // Implementation
}

```

### CLI Command Documentation

```go
// loginCommand handles the login command for the tray app CLI.
//
// Usage:
//
//    proton-git-lfs-tray login [--credential-provider PROVIDER]
//
// Flags:
//   - --credential-provider: Credential provider to use (pass-cli or git-credential)
//
// The command authenticates with Proton Drive using the configured credential
// provider. Credentials are resolved via:
//   - pass-cli: Searches Proton Pass vaults for proton.me login
//   - git-credential: Uses Git Credential Manager (Keychain/SecretService)
//
// Exit codes:
//   - 0: Login successful
//   - 1: Login failed (invalid credentials, network error, etc.)
//   - 2: CAPTCHA required (user must complete verification)
//   - 3: Rate limited (too many attempts)
//
// Example:
//
//    # Login with pass-cli (default)
//    proton-git-lfs-tray login
//
//    # Login with git-credential
//    proton-git-lfs-tray login --credential-provider git-credential
//
// The command verifies the credentials by calling proton-drive-cli's verify
// command, which performs a lightweight API call to check authentication.
func loginCommand(credentialProvider string) error {
    // Implementation
}

```

### Type/Struct Documentation

```go
// StatusReport represents the runtime state of the Git LFS adapter.
//
// The status is persisted to a JSON file (~/.proton-git-lfs/status.json)
// and polled by the system tray app every 5 seconds to update the UI.
//
// Fields:
//   - State: Current adapter state (idle, ok, error, transferring, etc.)
//   - LastOid: OID of the last processed object
//   - LastOp: Last operation performed (upload, download)
//   - Timestamp: When the status was last updated (RFC3339 format)
//   - ErrorCode: Machine-readable error code (if State is error)
//   - ErrorDetail: Human-readable error message (if State is error)
//   - RetryCount: Number of retry attempts for the current operation
//
// States:
//   - idle: No operations in progress
//   - ok: Last operation succeeded
//   - error: Last operation failed
//   - transferring: Operation in progress
//   - rate_limited: Rate limit active
//   - auth_required: Authentication needed
//   - captcha: CAPTCHA verification required
//
// Example:
//
//    status := StatusReport{
//        State:     "ok",
//        LastOid:   "abc123...",
//        LastOp:    "upload",
//        Timestamp: time.Now(),
//    }
//    err := WriteStatus(&status)
type StatusReport struct {
    State       string    `json:"state"`
    LastOid     string    `json:"lastOid"`
    LastOp      string    `json:"lastOp"`
    Timestamp   time.Time `json:"timestamp"`
    ErrorCode   string    `json:"errorCode,omitempty"`
    ErrorDetail string    `json:"errorDetail,omitempty"`
    RetryCount  int       `json:"retryCount,omitempty"`
}

```

### Method Documentation

```go
// Execute executes an operation with circuit breaker protection.
//
// The circuit breaker prevents cascading failures by tracking operation
// success/failure rates and transitioning between three states:
//
//   - CLOSED: Normal operation (failures tracked)
//   - OPEN: All requests rejected (circuit tripped)
//   - HALF_OPEN: Testing recovery (limited requests allowed)
//
// Parameters:
//   - operation: Function to execute (must return error)
//
// Returns:
//   - error: nil on success, or error from operation/circuit breaker
//
// The circuit opens after reaching the failure threshold (default: 5 failures).
// After the reset timeout (default: 60s), it transitions to half-open to test
// recovery. If recovery succeeds, it closes; if it fails, it reopens.
//
// Example:
//
//    breaker := NewCircuitBreaker("api-calls", CircuitBreakerOptions{
//        FailureThreshold: 3,
//        ResetTimeoutMs:   30000,
//    })
//
//    err := breaker.Execute(func() error {
//        return callAPI()
//    })
//    if err != nil {
//        // Circuit may be open or operation failed
//    }
func (cb *CircuitBreaker) Execute(operation func() error) error {
    // Implementation
}

```

## Go Doc Best Practices

### 1. Start with Package Comment

```go
// Package config provides configuration management for proton-git-lfs.
//
// This package handles environment variables, preferences, and status
// reporting shared between the adapter and tray app.
package config

```

### 2. Document Exported Items

```go
// ✅ Good: Documented exported function
// ValidateOID checks if the OID is a valid SHA-256 hash (64 hex characters).
func ValidateOID(oid string) bool {
    return len(oid) == 64 && isHex(oid)
}

// ❌ Bad: Undocumented exported function
func ValidateOID(oid string) bool {
    return len(oid) == 64 && isHex(oid)
}

```

### 3. Use Complete Sentences

```go
// ✅ Good: Complete sentence
// WriteStatus atomically writes the status report to the status file.
func WriteStatus(status *StatusReport) error

// ❌ Bad: Incomplete
// Write status
func WriteStatus(status *StatusReport) error

```

### 4. Include Examples

```go
// ParseTimeout parses a timeout string and returns milliseconds.
//
// Example:
//
//    ms, err := ParseTimeout("30s")
//    // ms = 30000, err = nil
//
//    ms, err := ParseTimeout("invalid")
//    // ms = 0, err = ErrInvalidTimeout
func ParseTimeout(s string) (int64, error) {
    // Implementation
}

```

### 5. Document Errors

```go
// LoadConfig loads configuration from ~/.proton-git-lfs/config.json.
//
// Returns an error if:
//   - File doesn't exist (returns default config, no error)
//   - File is not valid JSON (returns ErrInvalidJSON)
//   - File has invalid permissions (returns ErrPermissionDenied)
func LoadConfig() (*Config, error) {
    // Implementation
}

```

### 6. Use Sections for Long Docs

```go
// UploadFile uploads a file to Proton Drive.
//
// # Parameters
//
//   - oid: SHA-256 hash (64 hex chars)
//   - localPath: Local file path
//   - remotePath: Proton Drive destination
//
// # Returns
//
// Returns nil on success, or an error describing the failure.
//
// # Errors
//
//   - ErrInvalidOID: OID format is invalid
//   - ErrFileNotFound: Local file doesn't exist
//   - ErrRateLimited: API rate limit exceeded
//
// # Example
//
//    err := UploadFile("abc...", "/tmp/f", "/LFS/ab/c1/abc...")
func UploadFile(oid, localPath, remotePath string) error {
    // Implementation
}

```

## Documentation Priority Targets

### 1. CLI Commands (Highest Priority)

`cmd/tray/cli.go`:

- `loginCommand`
- `logoutCommand`
- `statusCommand`
- `configCommand`
- `registerCommand`

### 2. Main Adapter Functions

`cmd/adapter/main.go`:

- `main` (package comment)
- `handleMessage`
- `handleInitMessage`
- `handleUploadBatch`
- `handleDownloadBatch`
- `handleTerminateMessage`

### 3. Backend Interface

`cmd/adapter/backend.go`:

- `Backend` interface
- `NewLocalBackend`
- `NewDriveCLIBackend`
- `PutOID`
- `GetOID`
- `VerifyOID`

### 4. Configuration

`internal/config/`:

- `config.go`: All exported functions
- `status.go`: `StatusReport`, `WriteStatus`, `ReadStatus`
- `prefs.go`: `Preferences`, `LoadPrefs`, `SavePrefs`

### 5. System Tray

`cmd/tray/`:

- `menu.go`: Menu structure functions
- `connect.go`: Connect flow
- `status.go`: Status polling
- `setup.go`: Binary discovery

## Documentation Quality Checklist

For each exported item:

- [ ] Package comment at top of package
- [ ] Doc comment starts with item name
- [ ] Complete sentences (not fragments)
- [ ] Parameters documented (in prose or list)
- [ ] Return values documented
- [ ] Errors documented (what conditions cause them)
- [ ] At least one example (for main functions)
- [ ] See also references (for related functions)

## Integration with Deployment

### GitHub Pages with pkgsite

Add to `.github/workflows/docs.yml`:

```yaml
name: Go Documentation

on:
  push:
    branches: [main]
  workflow_dispatch:

permissions:
  contents: read
  pages: write
  id-token: write

jobs:
  build:
    runs-on: ubuntu-latest
    steps:

      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5

        with:
          go-version: '1.25'
          cache: true

      # Generate documentation

      - name: Install pkgsite

        run: go install golang.org/x/pkgsite/cmd/pkgsite@latest

      - name: Generate docs

        run: |
          mkdir -p docs/go-api
          pkgsite -http=:6060 &
          sleep 5
          wget -r -np -N -E -p -k http://localhost:6060/
          mv localhost:6060/* docs/go-api/
          killall pkgsite

      - uses: actions/upload-pages-artifact@v3

        with:
          path: docs/go-api

  deploy:
    needs: build
    runs-on: ubuntu-latest
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    steps:

      - uses: actions/deploy-pages@v4

        id: deployment

```

### Alternative: godoc.org

For public Go modules, documentation is automatically available at:

- `https://pkg.go.dev/github.com/<username>/<repo>`

Just push to GitHub and it will be indexed automatically.

## Common Patterns

### Error Variable

```go
// ErrInvalidOID is returned when an OID format is invalid.
//
// Valid OIDs must be exactly 64 hexadecimal characters (SHA-256 hash).
var ErrInvalidOID = errors.New("invalid OID format")

```

### Constant

```go
// MaxConcurrentOperations is the maximum number of concurrent Git LFS
// operations allowed. Operations beyond this limit will wait for an
// available slot.
const MaxConcurrentOperations = 10

```

### Interface

```go
// Backend abstracts storage operations for Git LFS objects.
//
// Implementations:
//   - LocalBackend: Stores objects in local filesystem
//   - DriveCLIBackend: Stores objects in Proton Drive via subprocess
//
// All methods must be safe for concurrent use.
type Backend interface {
    // PutOID stores an object by its OID.
    //
    // Returns an error if the operation fails. The error is classified
    // as retryable or permanent via the BackendError type.
    PutOID(oid, localPath string) error

    // GetOID retrieves an object by its OID.
    //
    // Returns an error if the object doesn't exist or retrieval fails.
    GetOID(oid, localPath string) error

    // VerifyOID checks if an object exists.
    //
    // Returns nil if the object exists, or an error otherwise.
    VerifyOID(oid string) error
}

```

## Tools

### go doc

```bash

# View package docs

go doc ./cmd/adapter

# View function docs

go doc ./cmd/adapter.UploadFile

# View all package members

go doc -all ./cmd/adapter

# View source

go doc -src ./cmd/adapter.UploadFile

```

### pkgsite (Modern)

```bash

# Install

go install golang.org/x/pkgsite/cmd/pkgsite@latest

# Serve locally

pkgsite -open .

```

### godoc (Legacy)

```bash

# Install

go install golang.org/x/tools/cmd/godoc@latest

# Serve locally

godoc -http=:6060

# Open http://localhost:6060

```

## Validation

Check documentation quality:

```bash

# Lint with golangci-lint (includes doc checks)

golangci-lint run

# Check for missing docs

go vet ./...

# Test examples

go test -run Example

```

## References

- [Effective Go - Commentary](https://go.dev/doc/effective_go#commentary)
- [Go Doc Comments](https://go.dev/doc/comment)
- [pkgsite](https://pkg.go.dev/golang.org/x/pkgsite)
