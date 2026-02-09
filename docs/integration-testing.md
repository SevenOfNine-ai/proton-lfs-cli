# Integration Testing Guide

> [!NOTE]
> Integration coverage now includes local-store and SDK-wrapper roundtrip tests. Real Proton Drive API integration is still pending.

This document describes how to test the Proton Git-LFS system end-to-end, from Git LFS → Custom Adapter → Proton SDK Service → File Storage.

## Table of Contents

1. [Unit Tests](#unit-tests)
2. [Integration Tests](#integration-tests)
3. [Manual Testing](#manual-testing)
4. [Performance Testing](#performance-testing)
5. [Troubleshooting](#troubleshooting)

---

## Unit Tests

Unit tests verify individual components in isolation.

### Running Unit Tests

```bash
# Test all components
make test

# Test only the adapter
make test-adapter

# Test only Git LFS
make test-lfs

# Test SDK service (requires Node.js)
cd proton-sdk-service && npm test

# Watch mode for adapter tests
make test-watch
```

### Test Coverage

Current coverage:
- **Adapter**: 21.6% of statements
  - JSON message parsing
  - Protocol event handling
  - Error responses
  - Session initialization

To improve coverage:
1. Add tests for SDK client integration
2. Add tests for file I/O operations
3. Add mock SDK service responses

---

## Integration Tests

Integration tests verify components working together.

### Prerequisites

1. **Go** installed and working
2. **Git LFS** submodule initialized
3. **Node.js** and npm (for SDK service)
4. **Proton Account** (for full end-to-end testing)

### Automated SDK Roundtrip Test

```bash
cd proton-sdk-service && npm install
cd ..

# Optional: override credentials passed to adapter for sdk integration tests.
export PROTON_TEST_USERNAME="your-username"
export PROTON_TEST_PASSWORD="your-password"

make test-integration
```

Pass-reference only variant:
```bash
eval "$(./scripts/export-pass-env.sh)"
make test-integration-sdk
```

### Phase 1: Adapter ↔ Protocol

**Test Goal**: Verify adapter correctly implements JSON-RPC protocol

```bash
# Test adapter with init message
echo '{"event":"init","operation":"download","remote":"origin","concurrent":false,"concurrenttransfers":1}' | \
  ./bin/git-lfs-proton-adapter --debug

# Expected output: JSON response with empty object
```

**Test Messages**:
```bash
# Init
{"event":"init","operation":"download","remote":"origin","concurrent":false,"concurrenttransfers":1}

# Upload
{"event":"upload","oid":"abc123","size":1024,"path":"/tmp/file"}

# Download
{"event":"download","oid":"abc123","size":1024}

# Terminate
{"event":"terminate"}
```

### Phase 2: Adapter ↔ SDK Service

**Test Goal**: Verify adapter communicates with SDK service correctly

1. Start SDK service:
```bash
cd proton-sdk-service
npm install
npm start  # Runs on port 3000
```

2. Configure adapter to use SDK:
```bash
export SDK_SERVICE_URL=http://localhost:3000
eval "$(./scripts/export-pass-env.sh)"
./bin/git-lfs-proton-adapter --backend sdk --sdk-service="$SDK_SERVICE_URL"
```

Alternative: resolve credentials via Proton Pass CLI references:
```bash
export PROTON_PASS_REF_ROOT="pass://Personal/Proton Git LFS"
eval "$(./scripts/export-pass-env.sh)"
./bin/git-lfs-proton-adapter --backend sdk --sdk-service="$SDK_SERVICE_URL"
```

3. Send authentication request:
```bash
echo '{"event":"init","operation":"download"}' | \
  ./bin/git-lfs-proton-adapter --backend sdk --sdk-service="$SDK_SERVICE_URL"
```

**Verify**:
- ✓ Adapter connects to SDK service
- ✓ SDK service responds with session token
- ✓ Adapter stores session for subsequent operations

### Phase 3: Git LFS ↔ Adapter

**Test Goal**: Verify Git LFS can discover and use the custom adapter

1. Build and install components:
```bash
make build

# Install Git LFS
cd submodules/git-lfs && sudo make install

# Install adapter to PATH
sudo cp bin/git-lfs-proton-adapter /usr/local/bin/
```

2. Configure Git to use custom adapter:
```bash
git config --global lfs.customtransfer.proton.path "$(which git-lfs-proton-adapter)"
git config --global lfs.customtransfer.proton.args "--backend=sdk --sdk-service=http://localhost:3000"
git config --global lfs.standalonetransferagent proton
```

3. Create test repository:
```bash
mkdir /tmp/test-proton-lfs
cd /tmp/test-proton-lfs
git init
git lfs install --local
git config lfs.customtransfer.proton.path "$(which git-lfs-proton-adapter)"
```

4. Create and track large file:
```bash
# Create 1MB test file
dd if=/dev/zero of=largefile.bin bs=1M count=1

# Track with Git LFS
git lfs track "*.bin"
git add largefile.bin .gitattributes
git commit -m "Add large file with LFS"
```

5. Enable debug logging:
```bash
export GIT_TRACE=1
export GIT_TRANSFER_TRACE=1
export DEBUG=true
```

6. Run Git operations:
```bash
# Push (triggers upload)
git push origin main

# Verify adapter logs
# Should see: "Upload request: OID=... Size=..."

# Clone (triggers download)
git clone /tmp/test-proton-lfs /tmp/test-proton-lfs-clone
cd /tmp/test-proton-lfs-clone/

# Verify file was downloaded
ls -lh largefile.bin
```

---

## Manual Testing

### Scenario 1: Single File Upload

```bash
# Start SDK service
cd proton-sdk-service && npm start &

# In another terminal
cd proton-git-lfs

# Create test file
dd if=/dev/zero of=/tmp/test-100mb.bin bs=1M count=100

# Initialize Git repository
mkdir /tmp/test-upload
cd /tmp/test-upload
git init
git lfs install --local
git config lfs.customtransfer.proton.path "$(pwd)/../bin/git-lfs-proton-adapter"
git config lfs.standalonetransferagent proton

# Add large file
cp /tmp/test-100mb.bin ./data.bin
git lfs track "*.bin"
git add data.bin .gitattributes
git commit -m "Add data"

# Push (should upload via custom adapter)
git push origin main
```

### Scenario 2: Clone with Download

```bash
# In directory with existing LFS repository

# Configure to use Proton adapter
git config lfs.customtransfer.proton.path "$(which git-lfs-proton-adapter)"
git config lfs.standalonetransferagent proton

# Clone
GIT_TRACE=1 git clone <repo-url> /tmp/test-clone

# Verify files were downloaded and decrypted
ls -lh /tmp/test-clone/
```

### Scenario 3: Concurrent Transfers

```bash
# Create repository with multiple large files
git init
git lfs install

# Add 5 files (each 50MB)
for i in {1..5}; do
  dd if=/dev/zero of=file-$i.bin bs=1M count=50
  git lfs track "*.bin"
  git add file-$i.bin
done

git commit -m "Add 5 files (250MB total)"

# Configure concurrent transfers
git config lfs.concurrenttransfers 4
git config lfs.customtransfer.proton.concurrent true

# Push with debug (should upload in parallel)
GIT_TRANSFER_TRACE=1 git push origin main
```

---

## Performance Testing

### Benchmark Adapter Message Processing

```bash
cd proton-git-lfs

# Build adapter
make build-adapter

# Run benchmark
go test -bench=. ./cmd/adapter/... -benchmem

# Example output:
# BenchmarkMessageProcessing-8    10000    100000 ns/op    X KB/op    X allocs/op
```

### Measure Transfer Throughput

```bash
# Create 1GB test file
dd if=/dev/zero of=/tmp/1gb-test.bin bs=1M count=1024

# Measure upload time
time ./bin/git-lfs-proton-adapter << EOF
{"event":"init","operation":"upload","concurrenttransfers":4}
{"event":"upload","oid":"test123","size":1073741824,"path":"/tmp/1gb-test.bin"}
{"event":"terminate"}
EOF

# Calculate throughput: size / time
```

### Monitor Memory Usage

```bash
# Start adapter in background with memory monitoring
./bin/git-lfs-proton-adapter --debug &
ADAPTER_PID=$!

# Monitor memory
watch -n 1 "ps -p $ADAPTER_PID -o %mem,%cpu,rss,vsz"

# Send test transfers...
```

---

## Troubleshooting

### Adapter Not Found by Git LFS

**Symptom**: `transfer failed: not a valid transfer type: proton`

**Solution**:
```bash
# Verify adapter is in PATH
which git-lfs-proton-adapter

# Verify Git config
git config lfs.customtransfer.proton.path

# Rebuild and reinstall
make clean build
sudo cp bin/git-lfs-proton-adapter /usr/local/bin/

# Reconfigure Git
git config --global lfs.customtransfer.proton.path /usr/local/bin/git-lfs-proton-adapter
```

### SDK Service Connection Failures

**Symptom**: `connection refused` errors

**Solution**:
```bash
# Verify SDK service is running
curl http://localhost:3000/health

# If not running, start it
cd proton-sdk-service
npm install
npm start

# Verify adapter can reach it
./bin/git-lfs-proton-adapter --sdk-service http://localhost:3000 << EOF
{"event":"init","operation":"upload"}
EOF
```

### File Encoding Issues

**Symptom**: Downloaded files are corrupted

**Potential Causes**:
- Binary mode not enabled
- Encryption/decryption failure
- SDK service file handling issue

**Debug**:
```bash
# Compare original and downloaded file
sha256sum original.bin downloaded.bin

# Check adapter logs
export DEBUG=true
export LOG_LEVEL=debug
```

### Session Token Expiration

**Symptom**: `invalid or expired session` errors during long transfers

**Solution**:
- Increase `SESSION_CACHE_DIR` timeout in `.env`
- Implement token refresh before expiry
- Run SDK service with longer session duration

### Performance Issues

**Symptom**: Transfers taking longer than expected

**Investigation**:
```bash
# Check network connectivity
curl -w "@curl-format.txt" -o /dev/null -s https://api.protonmail.com

# Monitor CPU usage
top -p $(pgrep -f git-lfs-proton-adapter)

# Check concurrency setting
git config lfs.concurrenttransfers

# Verify SDK service isn't bottlenecked
ab -n 100 -c 10 http://localhost:3000/health
```

---

## Continuous Integration

### GitHub Actions Test Flow

```yaml
name: Integration Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
      - uses: actions/setup-node@v2
      
      - name: Build
        run: make build
      
      - name: Test
        run: make test
      
      - name: Integration Test
        run: |
          ./ci/integration-tests.sh
```

### Local CI Test

```bash
# Simulate CI locally
make clean
make setup
make build
make test
./ci/integration-tests.sh
```

---

## Test Checklist

Before deployment, verify:

- [ ] Unit tests pass: `make test`
- [ ] Adapter builds without warnings: `make build-adapter`
- [ ] Git LFS tests pass: `make test-lfs`
- [ ] SDK service starts: `npm start` (in proton-sdk-service/)
- [ ] Adapter accepts initialization: Echo init message
- [ ] Git recognizes custom adapter: `git config lfs.customtransfer.proton.path`
- [ ] Git LFS repository can be initialized: `git lfs install`
- [ ] Large files can be tracked: `git lfs track "*.iso"`
- [ ] Files upload successfully: `git push origin main`
- [ ] Files download successfully: `git clone <url>`
- [ ] Concurrent transfers work: `lfs.concurrenttransfers=4`
- [ ] Error handling works: Send malformed JSON to adapter
- [ ] Logging is enabled: `export DEBUG=true`
- [ ] Performance is acceptable: `make benchmark`

---

## References

- [Architecture Documentation](architecture.md)
- [Custom Transfer Protocol](custom-transfer-integration.md)
- [SDK Integration Guide](proton-sdk-integration.md)
- [Deployment Guide](deployment.md)
- [Git LFS Documentation](https://git-lfs.github.io/)
