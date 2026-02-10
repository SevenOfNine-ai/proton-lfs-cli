# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Proton Git LFS Backend - A pre-alpha Git LFS custom transfer adapter for Proton Drive that provides encrypted storage for Git LFS objects.

**Current State:** The .NET bridge has been replaced with a TypeScript bridge using `proton-drive-cli`. The project uses a Go+TypeScript+Node.js stack with `pass-cli` for credential management. No .NET SDK required.

## Architecture

```
Go Adapter → Node.js LFS Bridge → proton-drive-cli (TypeScript subprocess) → Proton API
                ↓
            pass-cli (credentials)
```

This is a multi-language polyglot project with three main components:

1. **Go Adapter** (`cmd/adapter/`): Custom transfer adapter that implements Git LFS protocol
   - `main.go`: Core adapter logic, message handling, protocol implementation
   - `backend.go`: Storage backend abstraction (local and SDK service backends)
   - `client.go`: HTTP client for LFS bridge communication
   - `passcli.go`: Credential resolution via pass-cli integration
   - `config_constants.go`: Environment variable configuration

2. **Node.js LFS Bridge** (`proton-lfs-bridge/`): HTTP bridge between Go adapter and Proton Drive
   - `server.js`: Express server with REST endpoints (/init, /upload, /download, /list, /refresh)
   - `lib/protonDriveBridge.js`: TypeScript bridge runner (spawns proton-drive-cli subprocess)
   - `lib/session.js`: Session token management
   - `lib/fileManager.js`: Local mock file operations

3. **Submodules** (`submodules/`):
   - `git-lfs`: Upstream Git LFS reference
   - `pass-cli`: Proton Pass CLI for secure credential storage (v1.4.3)
   - `proton-drive-cli`: TypeScript-based Proton Drive client with full auth flow, E2E encryption

## Development Commands

### Setup and Build
```bash
make setup              # Install deps (Go + JS), create .env
make build              # Build Go adapter to bin/git-lfs-proton-adapter
make build-all          # Build adapter + Git LFS submodule + proton-drive-cli
make build-drive-cli    # Build proton-drive-cli TypeScript bridge only
```

### Testing
```bash
make test               # Run Go adapter unit tests
make test-sdk           # Run Node.js LFS bridge tests (Jest)
make test-integration   # Run Git LFS integration tests
make test-integration-sdk  # SDK backend integration (uses pass-cli)
make test-integration-proton-drive-cli  # proton-drive-cli bridge tests
make test-integration-credentials       # Credential security tests
make test-e2e-mock      # Mocked E2E pipeline (no real credentials)
make test-e2e-real      # Real Proton Drive E2E (requires pass-cli login + build-drive-cli)

# SDK integration with external service
export PROTON_LFS_BRIDGE_URL='http://127.0.0.1:3000'
make test-integration-sdk-real

# proton-drive-cli bridge mode
export SDK_BACKEND_MODE=proton-drive-cli
make test-integration-sdk
```

### Linting and Formatting
```bash
make fmt                # Format Go code
make lint               # Run Go vet + golangci-lint
make lint-sdk           # Run ESLint on LFS bridge
```

### Credential Management
```bash
pass-cli login          # Authenticate with Proton Pass
eval "$(make -s pass-env)"  # Export credentials from Pass
./scripts/export-pass-env.sh  # Direct credential export
```

## Backend Modes

The adapter supports two backends controlled by `PROTON_LFS_BACKEND`:

1. **local** (default): Local filesystem storage for testing
   - Stores objects in `PROTON_LFS_LOCAL_STORE_DIR`
   - No authentication required
   - Used for protocol integration tests

2. **sdk**: Proton Drive SDK integration
   - Routes through `proton-lfs-bridge` Node.js bridge
   - Requires Proton credentials (resolved exclusively via pass-cli)
   - Two sub-modes via `SDK_BACKEND_MODE`:
     - `local`: Mock/deterministic persistence (no real Proton API)
     - `proton-drive-cli` (or `real` as legacy alias): Uses TypeScript bridge subprocess

## Credential Resolution (pass-cli Integration)

The Go adapter (`cmd/adapter/passcli.go`) resolves credentials via pass-cli:

```bash
# Environment variables
PROTON_PASS_CLI_BIN=pass-cli           # Binary path
PROTON_PASS_REF_ROOT=pass://Personal/Proton Git LFS
PROTON_PASS_USERNAME_REF=pass://Personal/Proton Git LFS/username
PROTON_PASS_PASSWORD_REF=pass://Personal/Proton Git LFS/password
```

Credentials are resolved exclusively via pass-cli. Direct `PROTON_USERNAME`/`PROTON_PASSWORD` env var fallback has been removed. The adapter calls `pass-cli item view <reference>` and parses JSON or plaintext output.

## proton-drive-cli Bridge

The `proton-drive-cli` submodule (`submodules/proton-drive-cli/`) provides:
- Complete SRP authentication flow (`src/auth/`)
- Session management with token refresh
- File upload/download with E2E encryption (`src/drive/`)
- OpenPGP crypto operations (`src/crypto/`)
- Bridge command for Git LFS integration (`src/cli/bridge.ts`)

**Bridge protocol**: `proton-drive-cli bridge <command>` reads JSON from stdin, writes JSON to stdout using `{ ok, payload, error, code }` envelope format.

**Bridge helpers** (`src/cli/bridge-helpers.ts`): OID-to-path mapping using 2-character prefix directories (e.g., OID `abc123...` → `/LFS/ab/c123...`).

## Important Configuration Files

- `.env` / `.env.example`: Environment configuration (credentials, URLs, backend modes)
- `Makefile`: Build orchestration, test runners, prerequisite checks
- `package.json`: Root Yarn 4 workspace with `proton-lfs-bridge`
- `go.mod`: Go 1.25 module (minimal, no external deps)
- `docs/architecture/sdk-capability-matrix.md`: SDK access requirements by mode

## JavaScript Package Management

Uses **Yarn 4** (Berry) via Corepack:
```bash
corepack enable
corepack prepare yarn@4.1.1 --activate
yarn install

# Fallback to npm if Yarn unavailable
make setup JS_PM=npm
```

Workspace structure:
```json
{
  "workspaces": ["proton-lfs-bridge"]
}
```

## Testing Strategy

1. **Unit tests**: Go (`*_test.go`), Node.js (`*.test.js`)
2. **Integration tests**: `tests/integration/` with `-tags integration`
   - Black-box Git LFS protocol validation
   - Timeout and concurrency stress tests
   - SDK backend roundtrip tests
   - proton-drive-cli bridge tests
   - Credential security tests
3. **Security tests**: `proton-lfs-bridge/tests/security/`
   - Command injection prevention
   - Subprocess rate limiting
4. **Mock mode**: `ADAPTER_ALLOW_MOCK_TRANSFERS=true` for protocol-only testing

## Security Notes

- Never commit credentials to `.env` (use `.env.example` patterns)
- Credentials resolve exclusively via pass-cli (`pass://...` references)
- Credentials flow: pass-cli → Go adapter → LFS bridge → proton-drive-cli (via stdin)
- Credentials are passed via stdin to subprocesses (never command-line args)
- OID validation: `/^[a-f0-9]{64}$/i` before subprocess spawn
- Path traversal prevention: reject paths containing `..`
- Subprocess pool: max 10 concurrent operations with 5-minute timeout
- Session tokens stored in `~/.proton-drive-cli/session.json` (should be 0600)
- See `docs/security/threat-model.md` for full threat model

## Known Issues

1. `proton-drive-cli session refresh not working properly` (noted in its README)
2. `Mock transfers are fail-closed by default` (require explicit opt-in)
3. CAPTCHA may require manual intervention for new Proton accounts

## Development Workflow

1. Make changes to Go adapter or Node.js service
2. Run unit tests: `make test && make test-sdk`
3. Run integration tests: `make test-integration`
4. For SDK path: Ensure pass-cli login, then `make test-integration-sdk`
5. Run linting: `make lint`
6. Format code: `make fmt`
7. Check docs match runtime behavior (canonical rule: tests > docs)

## References

- Git LFS custom transfer spec: `submodules/git-lfs/docs/custom-transfers.md`
- Pass CLI docs: `submodules/pass-cli/docs/`
- proton-drive-cli: `submodules/proton-drive-cli/`
- Project docs: Start at `docs/README.md`
