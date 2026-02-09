# SDK Capability Matrix

Date: 2026-02-09

This matrix defines which Proton SDK paths are realistically runnable by external contributors versus internal Proton environments.

## Runtime Paths In This Repository

| Path | Configuration | Needs Internal Proton Access | Uses Your Own Proton Account | Who Can Run It |
|---|---|---|---|---|
| Local prototype backend | `SDK_BACKEND_MODE=local` | No | Not required (supports test credentials) | Anyone |
| In-repo real bridge (build from source) | `SDK_BACKEND_MODE=real` and no `PROTON_REAL_BRIDGE_BIN` | Yes (internal NuGet source for `Proton.*` packages) | Yes | Internal Proton environment |
| Real bridge via prebuilt binary | `SDK_BACKEND_MODE=real` and `PROTON_REAL_BRIDGE_BIN=/path/to/proton-real-bridge` | No local source access required | Yes | Anyone with trusted prebuilt binary |
| External real SDK service | `PROTON_SDK_SERVICE_URL=http://<host>:<port>` | Depends on service operator, not local machine | Yes | Anyone with reachable service endpoint |

## SDK Surface Expectations (Upstream Submodule)

| SDK Surface | Turnkey Auth/Login Included | Integrator Must Provide | Notes |
|---|---|---|---|
| JS SDK (`submodules/sdk/js/sdk`) | No | Authenticated HTTP client and account provider | Public API is integration-oriented, not standalone login flow |
| Swift bindings (`submodules/sdk/swift/ProtonDriveSDK`) | No | `HttpClientProtocol` and `AccountClientProtocol` implementations | Assumes host app already manages auth/session/account keys |
| Kotlin bindings (`submodules/sdk/kt/sdk`) | No | Session bootstrap inputs, API provider, address resolvers | Designed for embedding in Proton client stacks |
| C# SDK source (`submodules/sdk/cs`) | Low-level session primitives exist | Production-grade integrator flow still required | Build currently depends on internal Proton NuGet source mapping |

## Policy For This Repository

1. Default contributor path is `SDK_BACKEND_MODE=local`.
2. Real validation for external contributors uses either:
   - `PROTON_SDK_SERVICE_URL` (external service), or
   - `PROTON_REAL_BRIDGE_BIN` (prebuilt bridge).
3. Do not require in-repo C# source builds for external CI or onboarding.
