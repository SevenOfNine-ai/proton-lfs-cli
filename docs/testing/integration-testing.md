# Integration Testing

## Scope

Integration tests validate Git LFS client behavior against the adapter runtime and backend implementations.

## Test Commands

| Command | Scope |
|---|---|
| `make test` | Adapter unit tests |
| `make test-sdk` | Node bridge unit tests |
| `make test-integration` | Git LFS + adapter integration suite |
| `make test-integration-timeout` | Stalled-adapter timeout semantics |
| `make test-integration-stress` | High-volume concurrent stress/soak |
| `make test-integration-sdk` | SDK backend integration path (local service by default) |
| `make test-integration-sdk-real` | SDK backend integration path against external `PROTON_LFS_BRIDGE_URL` |
| `make test-integration-proton-drive-cli` | proton-drive-cli bridge integration tests |
| `make test-integration-credentials` | Credential flow security tests |
| `make test-e2e-mock` | Mocked E2E pipeline (no real credentials) |
| `make test-e2e-real` | Real Proton Drive E2E (requires pass-cli login + build-drive-cli) |

## Prerequisites

- `git-lfs` available on `PATH`.
- Adapter built with `make build`.
- For local SDK path: Node.js installed and service deps installed (`make setup`).
- For proton-drive-cli bridge mode (`SDK_BACKEND_MODE=proton-drive-cli`): `make build-drive-cli` must succeed.
- JS dependencies should be installed from repository root using Yarn 4 via Corepack (`corepack enable && corepack prepare yarn@4.1.1 --activate && yarn install`) when running the local SDK path.

## Credentials For SDK Tests

Credentials are resolved exclusively via pass-cli. Direct environment variable fallback is not supported.

Preferred path:

```bash
pass-cli login
make test-integration-sdk
```

`make test-integration-sdk` now performs a prerequisite check and resolves `PROTON_PASS_*` via `scripts/export-pass-env.sh`.
For non-default vault/item references, export your custom `PROTON_PASS_*` values first.
If Node is only configured via shell startup files (`~/.zshrc`, `nvm`, `fnm`), run with:

```bash
make test-integration-sdk NODE="$(command -v node)"
```

Default package manager is `yarn` (Yarn 4 via Corepack). To use npm explicitly:

```bash
make test-integration-sdk JS_PM=npm
```

To run against the proton-drive-cli bridge mode:

```bash
export SDK_BACKEND_MODE=proton-drive-cli
make test-integration-sdk
```

If you cannot build proton-drive-cli, use one of:

- External real service: set `PROTON_LFS_BRIDGE_URL` and run `make test-integration-sdk-real`.
- Local prototype path (no real Proton backend): default `SDK_BACKEND_MODE=local`.

See `docs/architecture/sdk-capability-matrix.md` for the full environment matrix.

Optional (accounts requiring explicit data password or 2FA):

- `PROTON_DATA_PASSWORD`
- `PROTON_SECOND_FACTOR_CODE`

## Personal Account Practical Steps

If you are testing with a personal Proton account:

1. Store credentials in Proton Pass.
1. Use the default references:
   - `pass://Personal/Proton Git LFS/username`
   - `pass://Personal/Proton Git LFS/password`
1. Or export custom references before tests:

```bash
eval "$(./scripts/export-pass-env.sh --ref-root 'pass://Personal/Your Entry')"
```

1. Authenticate and run prerequisite checks:

```bash
pass-cli login
make check-sdk-prereqs
```

1. Choose one runtime path:

   - Local prototype path (no real Proton backend): `make test-integration-sdk`
   - proton-drive-cli bridge: `SDK_BACKEND_MODE=proton-drive-cli make test-integration-sdk`
   - Real backend via external service: set `PROTON_LFS_BRIDGE_URL` and run `make test-integration-sdk-real`

To run SDK integration tests against an externally running service:

```bash
export PROTON_LFS_BRIDGE_URL='http://127.0.0.1:3000'
make test-integration-sdk-real
```

When `PROTON_LFS_BRIDGE_URL` is set, `make test-integration-sdk` also uses the external service and skips local Node/JS dependency checks.

## Mocked E2E Testing

For CI and local testing without real Proton credentials:

```bash
make test-e2e-mock
```

This uses `mock-pass-cli.sh` and `mock-proton-drive-cli.js` to exercise the full pipeline: `git lfs push` -> adapter -> LFS bridge -> mock bridge -> mock storage, then clone and pull back.

## Coverage Expectations

- Real `git-lfs` subprocess path for upload and download.
- LFS bridge API contract path covering `/init`, `/upload`, `/download`, `/refresh`, and `/list`.
- Standalone mode behavior (`action: null`) coverage.
- Object-level failure handling coverage (`complete.error`).
- Wrong-OID response rejection coverage (`progress` and `complete`).
- Adapter crash and no-response subprocess failure coverage.
- Stalled-adapter timeout semantics coverage (`lfs.activitytimeout`) across OS CI matrix.
- Concurrent multi-file roundtrip coverage (`lfs.customtransfer.proton.concurrent=true`).
- High-volume concurrent stress/soak coverage (`PROTON_LFS_STRESS_*`).
- Mocked E2E pipeline coverage (full Git LFS push/pull through mock bridge).

## High-Value Missing Tests

- Real Proton API integration tests are now runnable when an external real LFS bridge is provided via `PROTON_LFS_BRIDGE_URL`.
- In-repo service defaults to local persistence unless `SDK_BACKEND_MODE=proton-drive-cli` is set.

## Stress Tuning

`make test-integration-stress` supports optional scale controls:

- `PROTON_LFS_STRESS_FILE_COUNT` (default `24`)
- `PROTON_LFS_STRESS_ROUNDS` (default `3`)
- `PROTON_LFS_STRESS_CONCURRENCY` (default `8`)
