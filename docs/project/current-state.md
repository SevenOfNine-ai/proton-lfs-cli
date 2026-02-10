# Current State

Date: 2026-02-10

## Implemented

- Adapter protocol loop (`init`, `upload`, `download`, `terminate`) is implemented and testable.
- Local backend is usable for deterministic end-to-end integration tests.
- SDK backend path is wired and covered with integration tests against `proton-lfs-bridge`.
- `proton-lfs-bridge` uses `proton-drive-cli` (TypeScript subprocess) as its bridge to the Proton API, replacing the former .NET C# bridge.
- SDK integration suite can run against an external service via `PROTON_LFS_BRIDGE_URL` and covers `/init`, `/upload`, `/download`, `/refresh`, and `/list`.
- Proton Pass reference-based credential flow is implemented (`PROTON_PASS_*`).
- Security hardening: OID validation, path traversal prevention, subprocess pool (max 10), per-operation timeout (5 min).
- Security tests: command injection, rate limiting, credential flow, session file permissions.

## Architecture

```
Go Adapter → Node.js LFS Bridge → proton-drive-cli (TypeScript subprocess) → Proton API
                ↓
            pass-cli (credentials)
```

- **No .NET SDK required.** The former C# bridge and `submodules/sdk` have been removed.
- `SDK_BACKEND_MODE=proton-drive-cli` is the canonical value; `real` is accepted as a legacy alias.

## Not Implemented Yet

- Production-grade session lifecycle (session refresh has known issues in proton-drive-cli).
- Production observability baseline (metrics, SLOs, alerts, runbooks).
- Streaming support for very large files (>2GB may timeout).

## Local Baseline

```bash
make setup
make build-all        # Builds Go adapter, Git LFS, and proton-drive-cli
make test
make test-integration
```

SDK integration path:

```bash
eval "$(make -s pass-env)"
make test-integration-sdk
```

SDK backend with proton-drive-cli:

```bash
eval "$(make -s pass-env)"
export SDK_BACKEND_MODE=proton-drive-cli
make check-sdk-prereqs
make test-integration-sdk
```

External real LFS bridge:

```bash
eval "$(make -s pass-env)"
export PROTON_LFS_BRIDGE_URL='http://127.0.0.1:3000'
make test-integration-sdk-real
```
