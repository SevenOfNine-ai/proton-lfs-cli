# Proton SDK Bridge

Bridge implementation path: `proton-sdk-service/`.

## Current Interface

- `POST /init` with `username` + `password`.
- `POST /upload` with `token`, `oid`, `path`.
- `POST /download` with `token`, `oid`, `outputPath`.
- `POST /refresh` with `token`.
- `GET /list` with `token` and optional `folder`.
- `GET /health` for readiness checks.

## Current Reality

- Service supports two backend modes:
  - `SDK_BACKEND_MODE=local`: deterministic local persistence prototype.
  - `SDK_BACKEND_MODE=real`: in-repo `.NET` bridge (`proton-sdk-service/tools/proton-real-bridge`) that uses Proton C# SDK for auth/upload/download/list.
- Real mode currently authenticates per operation and keeps credentials in process memory-bound session metadata.
- Real mode can use `PROTON_DATA_PASSWORD` and `PROTON_SECOND_FACTOR_CODE` when request payload does not include those fields.
- Integration tests can also target an external SDK service via `PROTON_SDK_SERVICE_URL`.

Environment feasibility matrix: `docs/architecture/sdk-capability-matrix.md`.

## Requirements Propagated From Git LFS

- Upload/download must preserve exact bytes and object identity.
- Errors must be typed and per-object, not process-fatal.
- Session failure must produce explicit adapter errors, never silent success.
- API contracts must remain deterministic so adapter tests can assert behavior.

## Next Hardening Targets

1. Replace mock auth/session logic with real Proton SDK flow.
2. Reuse persisted SDK sessions in real mode instead of re-authenticating each operation.
3. Add strict response schema validation between adapter and service.
4. Add fault-injection tests (timeouts, partial writes, session expiry).
