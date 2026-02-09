# Proton Git LFS Backend

Pre-alpha Git LFS custom transfer backend for Proton Drive.

## Current State (2026-02-09)

- Git LFS custom transfer adapter protocol is implemented and tested.
- `local` backend roundtrip path is implemented for deterministic integration testing.
- `sdk` backend path is wired through `proton-sdk-service` and integration-tested.
- `proton-sdk-service` now supports `SDK_BACKEND_MODE=real` through an in-repo .NET bridge that uses Proton's C# SDK.
- Real mode is experimental and requires .NET 9 SDK plus valid Proton credentials.
- Mock transfers are fail-closed by default and require explicit opt-in.
- SDK feasibility by environment (external vs internal) is documented in `docs/architecture/sdk-capability-matrix.md`.

## Quick Start

```bash
make setup
make build
make test
make test-integration
```

Root JS dependency install (Yarn 4 via Corepack):

```bash
corepack enable
corepack prepare yarn@4.1.1 --activate
yarn install
# fallback
npm install
```

Make-based install:

```bash
make setup
# fallback if you prefer npm
make setup JS_PM=npm
```

SDK-backed integration path:

```bash
pass-cli login
make check-sdk-prereqs
make test-integration-sdk
```

In-repo real SDK mode:

```bash
pass-cli login
export SDK_BACKEND_MODE=real
make test-integration-sdk
```

If you do not have internal Proton NuGet access, use one of:

```bash
export PROTON_SDK_SERVICE_URL='http://127.0.0.1:3000'
make test-integration-sdk-real

# or
export SDK_BACKEND_MODE=real
export PROTON_REAL_BRIDGE_BIN='/absolute/path/to/proton-real-bridge'
make test-integration-sdk
```

If your account requires dedicated data password or 2FA code, set:

```bash
export PROTON_DATA_PASSWORD='...'
export PROTON_SECOND_FACTOR_CODE='...'
```

External/real SDK service integration path:

```bash
pass-cli login
export PROTON_SDK_SERVICE_URL='http://127.0.0.1:3000'
make test-integration-sdk-real
```

If your Node binary is managed in shell startup (for example `nvm` in `~/.zshrc`), pass it explicitly:

```bash
make test-integration-sdk NODE="$(command -v node)"
```

`make test-integration-sdk` uses `yarn` by default. Override only if needed:

```bash
make test-integration-sdk JS_PM=npm
```

If you use non-default vault/item references, set `PROTON_PASS_*` first, then run:

```bash
eval "$(make -s pass-env)"
make test-integration-sdk
```

## Credentials

Use Proton Pass references, not plaintext credentials:

```bash
pass-cli login
eval "$(./scripts/export-pass-env.sh)"
```

Canonical reference root is `pass://Personal/Proton Git LFS`.

## Repository Layout

- `cmd/adapter/`: Go custom transfer adapter.
- `proton-sdk-service/`: Node bridge service prototype for SDK calls.
- `tests/integration/`: black-box Git LFS integration tests.
- `docs/`: project plan, architecture, testing, and operations docs.
- `submodules/`: upstream references (`git-lfs`, `sdk`, `pass-cli`).

## Documentation

Start at `docs/README.md`.

## Security

This repository is pre-production. See `SECURITY.md`.
