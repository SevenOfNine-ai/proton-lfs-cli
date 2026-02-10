# Proton SDK Bridge

Bridge implementation path: `proton-lfs-bridge/`.

## Architecture

```
Go Adapter → Node.js LFS Bridge → proton-drive-cli (TypeScript subprocess) → Proton API
                ↓
            pass-cli (credentials)
```

The bridge spawns `proton-drive-cli bridge <command>` as a subprocess, passing JSON via stdin and reading JSON from stdout. Credentials are never passed via command-line arguments.

## Current Interface

- `POST /init` with `username` + `password`.
- `POST /upload` with `token`, `oid`, `path`.
- `POST /download` with `token`, `oid`, `outputPath`.
- `POST /refresh` with `token`.
- `GET /list` with `token` and optional `folder`.
- `GET /health` for readiness checks.

## Backend Modes

- `SDK_BACKEND_MODE=local`: deterministic local persistence prototype.
- `SDK_BACKEND_MODE=proton-drive-cli` (or `real` as legacy alias): TypeScript bridge using `proton-drive-cli` subprocess for auth/upload/download/list via Proton Drive API with E2E encryption.
- Integration tests can also target an external LFS bridge via `PROTON_LFS_BRIDGE_URL`.

## Subprocess Communication Protocol

The bridge (`proton-lfs-bridge/lib/protonDriveBridge.js`) communicates with `proton-drive-cli` using:

1. **Spawn**: `node <proton-drive-cli-path> bridge <command>`
2. **Stdin**: JSON payload with credentials and operation parameters
3. **Stdout**: JSON response envelope `{ ok: true/false, payload: {...}, error: "...", code: 400-500 }`
4. **Stderr**: Diagnostic logs (not parsed for responses)

## Security Considerations

- Credentials passed via stdin (not visible in `ps` output)
- OID validation: strict 64-character hex regex before subprocess spawn
- Path traversal prevention: reject paths containing `..`
- Subprocess pool: maximum 10 concurrent operations
- Timeout: 5 minutes per operation (configurable via `PROTON_DRIVE_CLI_TIMEOUT_MS`)
- Session tokens stored in `~/.proton-drive-cli/session.json` with 0600 permissions

## Requirements Propagated From Git LFS

- Upload/download must preserve exact bytes and object identity.
- Errors must be typed and per-object, not process-fatal.
- Session failure must produce explicit adapter errors, never silent success.
- API contracts must remain deterministic so adapter tests can assert behavior.

## Known Issues

1. proton-drive-cli session refresh not fully reliable (workaround: re-authenticate on 401).
2. CAPTCHA may require manual intervention for new accounts.
3. No streaming for large files (>2GB may timeout — increase `PROTON_DRIVE_CLI_TIMEOUT_MS`).

## Next Hardening Targets

1. Improve session reuse to avoid re-authentication on every operation.
2. Add strict response schema validation between adapter and service.
3. Add fault-injection tests (timeouts, partial writes, session expiry).
4. Address upstream session refresh issue in proton-drive-cli.
