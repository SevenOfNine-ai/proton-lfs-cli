# Adapter Configuration Reference

Source of truth in code: `cmd/adapter/config_constants.go`.

## Proton Pass Naming

No fixed entry name is required. Any reference that resolves via `pass-cli` works.

Canonical convention in this repository:

- Root: `pass://Personal/Proton Git LFS`
- Username: `${ROOT}/username`
- Password: `${ROOT}/password`

## Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `SDK_SERVICE_URL` | `http://localhost:3000` | SDK service base URL |
| `PROTON_LFS_BACKEND` | `local` | Adapter backend (`local`, `sdk`) |
| `ADAPTER_ALLOW_MOCK_TRANSFERS` | `false` | Enables mock transfer mode |
| `PROTON_LFS_LOCAL_STORE_DIR` | empty | Local backend object root |
| `PROTON_PASS_CLI_BIN` | `pass-cli` | Proton Pass CLI binary path |
| `PROTON_PASS_REF_ROOT` | `pass://Personal/Proton Git LFS` | Pass ref root |
| `PROTON_PASS_USERNAME_REF` | `${PROTON_PASS_REF_ROOT}/username` | Pass username ref |
| `PROTON_PASS_PASSWORD_REF` | `${PROTON_PASS_REF_ROOT}/password` | Pass password ref |
| `PROTON_USERNAME` | empty | Legacy fallback username |
| `PROTON_PASSWORD` | empty | Legacy fallback password |

Credential resolution order:

1. CLI flags.
2. `PROTON_USERNAME` / `PROTON_PASSWORD`.
3. `PROTON_PASS_USERNAME_REF` / `PROTON_PASS_PASSWORD_REF`.
4. If only password ref is set, fallback to `pass-cli user info --output json` for username.

## Helper Script

```bash
pass-cli login
eval "$(./scripts/export-pass-env.sh)"
```

The script verifies that `pass-cli` is authenticated, validates both references, sets `PROTON_PASS_*`, and unsets plaintext credential vars.

## SDK Service Real Mode Constants

When running `proton-sdk-service` with `SDK_BACKEND_MODE=real`:

| Variable | Default | Purpose |
|---|---|---|
| `SDK_BACKEND_MODE` | `local` | `real` enables the in-repo C# Proton SDK bridge |
| `PROTON_APP_VERSION` | `external-drive-protonlfs@dev` | Proton client app version header |
| `PROTON_DATA_PASSWORD` | empty | Optional dedicated data password fallback |
| `PROTON_SECOND_FACTOR_CODE` | empty | Optional 2FA code fallback |
| `PROTON_REAL_BRIDGE_BIN` | empty | Optional prebuilt bridge executable path |
| `PROTON_REAL_BRIDGE_PROJECT` | `proton-sdk-service/tools/proton-real-bridge/ProtonRealBridge.csproj` | Bridge project path used by `dotnet run` |
| `PROTON_REAL_BRIDGE_CONFIGURATION` | `Release` | Build configuration for bridge execution |
| `PROTON_REAL_BRIDGE_TIMEOUT_MS` | `300000` | Node-side command timeout |
| `PROTON_REAL_BRIDGE_TIMEOUT_SECONDS` | `300` | Bridge-side operation timeout |
