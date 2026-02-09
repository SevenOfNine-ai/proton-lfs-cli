# Deployment Guide

This guide is for development and CI environments. Production rollout is blocked by project plan gates.

## Prerequisites

- Go toolchain available.
- `git-lfs` installed and on `PATH`.
- Node.js with Corepack enabled and Yarn 4 for `proton-sdk-service` and SDK tests.
- .NET 9 SDK for in-repo real Proton bridge mode (`SDK_BACKEND_MODE=real`).
- Building the in-repo real bridge from source currently requires internal Proton NuGet access (`Proton.*` package source mapping in `submodules/sdk/cs/nuget.config`).

## Local Bring-Up

```bash
make setup
make build
make test
make test-integration
```

Install JS dependencies from root:

```bash
corepack enable
corepack prepare yarn@4.1.1 --activate
yarn install
# fallback
npm install
```

SDK-backed path:

```bash
pass-cli login
make test-integration-sdk
```

SDK-backed path with in-repo real Proton mode:

```bash
pass-cli login
export SDK_BACKEND_MODE=real
make check-sdk-prereqs
make test-integration-sdk
```

If you do not have internal Proton NuGet access, use one of:

```bash
# External real SDK service
export PROTON_SDK_SERVICE_URL='http://127.0.0.1:3000'
make test-integration-sdk-real

# Or trusted prebuilt bridge binary
export SDK_BACKEND_MODE=real
export PROTON_REAL_BRIDGE_BIN='/absolute/path/to/proton-real-bridge'
make test-integration-sdk
```

Optional environment variables for accounts that need explicit data password or 2FA:

- `PROTON_DATA_PASSWORD`
- `PROTON_SECOND_FACTOR_CODE`

If `node` is not visible to non-interactive shells, pass an explicit binary path:

```bash
make test-integration-sdk NODE="$(command -v node)"
```

Default package manager is `yarn` (Yarn 4 via Corepack). To use npm explicitly:

```bash
make test-integration-sdk JS_PM=npm
```

Preflight only:

```bash
make check-sdk-prereqs
```

## Git LFS Agent Wiring

Repository-level configuration example:

```bash
git lfs install --local
git config lfs.customtransfer.proton.path "$(pwd)/bin/git-lfs-proton-adapter"
git config lfs.customtransfer.proton.args "--backend=local"
git config lfs.standalonetransferagent proton
```

Switch to SDK backend:

```bash
git config lfs.customtransfer.proton.args "--backend=sdk --sdk-service=http://localhost:3000"
```

## CI Notes

- Keep credentials in CI secret stores only.
- Prefer `PROTON_PASS_*` references and pass-cli in CI.
- Run `make test` and `make test-integration` on every PR.
