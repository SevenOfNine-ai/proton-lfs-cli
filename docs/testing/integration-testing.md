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
| `make test-integration-sdk-real` | SDK backend integration path against external `PROTON_SDK_SERVICE_URL` |

## Prerequisites

- `git-lfs` available on `PATH`.
- Adapter built with `make build`.
- For local SDK path: Node.js installed and service deps installed (`make setup`).
- For in-repo real SDK mode (`SDK_BACKEND_MODE=real`): .NET 9 SDK installed (`dotnet --version`).
- For in-repo real SDK mode, `dotnet restore proton-sdk-service/tools/proton-real-bridge/ProtonRealBridge.csproj` must succeed (including any required NuGet source credentials).
  - `submodules/sdk/cs/nuget.config` maps `Proton.*` packages to a NuGet source key named `Proton`; ensure that source is configured locally.
- JS dependencies should be installed from repository root using Yarn 4 via Corepack (`corepack enable && corepack prepare yarn@4.1.1 --activate && yarn install`) when running the local SDK path.

## Credentials For SDK Tests

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

To run against the in-repo real backend mode:

```bash
export SDK_BACKEND_MODE=real
make test-integration-sdk
```

If this fails during `/init` with `proton bridge command failed (exit 1)`, read the embedded SDK service logs in the test output and fix the reported `dotnet restore/build` issue first.
If you cannot access internal Proton NuGet feeds, do not use in-repo `SDK_BACKEND_MODE=real` build path.
Use one of:
- External real service: set `PROTON_SDK_SERVICE_URL` and run `make test-integration-sdk-real`.
- Prebuilt bridge binary from a trusted internal build: set `PROTON_REAL_BRIDGE_BIN`.
See `docs/architecture/sdk-capability-matrix.md` for the full environment matrix.

Optional (accounts requiring explicit data password or 2FA):

- `PROTON_DATA_PASSWORD`
- `PROTON_SECOND_FACTOR_CODE`

## Personal Account Practical Steps

If you are testing with a personal Proton account and do not have internal Proton NuGet access, use this flow:

1. Store credentials in Proton Pass.
2. Use the default references:
   - `pass://Personal/Proton Git LFS/username`
   - `pass://Personal/Proton Git LFS/password`
3. Or export custom references before tests:

```bash
eval "$(./scripts/export-pass-env.sh --ref-root 'pass://Personal/Your Entry')"
```

4. Authenticate and run prerequisite checks:

```bash
pass-cli login
make check-sdk-prereqs
```

5. Choose one runtime path:
   - Local prototype path (no real Proton backend): `make test-integration-sdk`
   - Real backend via external service: set `PROTON_SDK_SERVICE_URL` and run `make test-integration-sdk-real`
   - Real backend via trusted prebuilt bridge binary: set `SDK_BACKEND_MODE=real` and `PROTON_REAL_BRIDGE_BIN`, then run `make test-integration-sdk`

To run SDK integration tests against an externally running service:

```bash
export PROTON_SDK_SERVICE_URL='http://127.0.0.1:3000'
make test-integration-sdk-real
```

When `PROTON_SDK_SERVICE_URL` is set, `make test-integration-sdk` also uses the external service and skips local Node/JS dependency checks.

Optional override for temporary local troubleshooting:

- `PROTON_TEST_USERNAME`
- `PROTON_TEST_PASSWORD`

## Coverage Expectations

- Real `git-lfs` subprocess path for upload and download.
- SDK service API contract path covering `/init`, `/upload`, `/download`, `/refresh`, and `/list`.
- Standalone mode behavior (`action: null`) coverage.
- Object-level failure handling coverage (`complete.error`).
- Wrong-OID response rejection coverage (`progress` and `complete`).
- Adapter crash and no-response subprocess failure coverage.
- Stalled-adapter timeout semantics coverage (`lfs.activitytimeout`) across OS CI matrix.
- Concurrent multi-file roundtrip coverage (`lfs.customtransfer.proton.concurrent=true`).
- High-volume concurrent stress/soak coverage (`PROTON_LFS_STRESS_*`).

## High-Value Missing Tests

- Real Proton API integration tests are now runnable when an external real SDK service is provided via `PROTON_SDK_SERVICE_URL`.
- In-repo service defaults to local persistence unless `SDK_BACKEND_MODE=real` is set.

## Stress Tuning

`make test-integration-stress` supports optional scale controls:

- `PROTON_LFS_STRESS_FILE_COUNT` (default `24`)
- `PROTON_LFS_STRESS_ROUNDS` (default `3`)
- `PROTON_LFS_STRESS_CONCURRENCY` (default `8`)
