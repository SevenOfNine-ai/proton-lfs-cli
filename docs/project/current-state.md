# Current State

Date: 2026-02-09

## Implemented

- Adapter protocol loop (`init`, `upload`, `download`, `terminate`) is implemented and testable.
- Local backend is usable for deterministic end-to-end integration tests.
- SDK backend path is wired and covered with integration tests against `proton-sdk-service`.
- `proton-sdk-service` now has an experimental `SDK_BACKEND_MODE=real` path backed by the in-repo C# Proton SDK bridge (source builds require internal Proton NuGet access).
- SDK integration suite can run against an external service via `PROTON_SDK_SERVICE_URL` and now covers `/init`, `/upload`, `/download`, `/refresh`, and `/list`.
- Proton Pass reference-based credential flow is implemented (`PROTON_PASS_*`).

## Not Implemented Yet

- Production-grade real mode lifecycle (session reuse, stronger credential isolation, 2FA/data-password ergonomics).
- Production-grade auth/session lifecycle in the SDK bridge.
- Production observability baseline (metrics, SLOs, alerts, runbooks).

## Local Baseline

```bash
make setup
make build
make test
make test-integration
```

SDK integration path:

```bash
eval "$(make -s pass-env)"
make test-integration-sdk
```

SDK real mode path:

```bash
eval "$(make -s pass-env)"
export SDK_BACKEND_MODE=real
make test-integration-sdk
```

External personal-account real path (no in-repo bridge source build):

```bash
eval "$(make -s pass-env)"
export PROTON_SDK_SERVICE_URL='http://127.0.0.1:3000'
make test-integration-sdk-real
```
