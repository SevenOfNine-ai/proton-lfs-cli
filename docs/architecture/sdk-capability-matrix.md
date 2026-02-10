# SDK Capability Matrix

Date: 2026-02-10

This matrix defines which Proton SDK paths are realistically runnable by external contributors versus internal Proton environments.

## Runtime Paths In This Repository

| Path | Configuration | Needs Internal Proton Access | Uses Your Own Proton Account | Who Can Run It |
| --- | --- | --- | --- | --- |
| Local prototype backend | `SDK_BACKEND_MODE=local` | No | Not required (supports test credentials) | Anyone |
| proton-drive-cli bridge | `SDK_BACKEND_MODE=proton-drive-cli` | No | Yes | Anyone with Node.js 18+ |
| External real LFS bridge | `PROTON_LFS_BRIDGE_URL=http://<host>:<port>` | Depends on service operator, not local machine | Yes | Anyone with reachable service endpoint |

## proton-drive-cli Bridge

The `proton-drive-cli` submodule (`submodules/proton-drive-cli/`) provides:

- Complete SRP authentication (no external SDK dependencies)
- E2E encryption via OpenPGP
- File upload/download with block-level encryption
- Session management and persistence
- Pure TypeScript — no .NET, no internal NuGet access required

Build: `make build-drive-cli`

## Policy For This Repository

1. Default contributor path is `SDK_BACKEND_MODE=local`.
2. Real validation for external contributors uses either:
   - `SDK_BACKEND_MODE=proton-drive-cli` (local bridge), or
   - `PROTON_LFS_BRIDGE_URL` (external service).
3. No .NET SDK or internal NuGet access required for any path.
