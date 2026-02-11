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
| --- | --- | --- |
| `PROTON_LFS_BACKEND` | `local` | Adapter backend (`local`, `sdk`) |
| `ADAPTER_ALLOW_MOCK_TRANSFERS` | `false` | Enables mock transfer mode |
| `PROTON_LFS_LOCAL_STORE_DIR` | empty | Local backend object root |
| `PROTON_PASS_CLI_BIN` | `pass-cli` | Proton Pass CLI binary path |
| `PROTON_PASS_REF_ROOT` | `pass://Personal/Proton Git LFS` | Pass ref root |
| `PROTON_PASS_USERNAME_REF` | `${PROTON_PASS_REF_ROOT}/username` | Pass username ref |
| `PROTON_PASS_PASSWORD_REF` | `${PROTON_PASS_REF_ROOT}/password` | Pass password ref |
| `PROTON_DRIVE_CLI_BIN` | `submodules/proton-drive-cli/dist/index.js` | Path to proton-drive-cli entry point |

Credentials are resolved exclusively via pass-cli. Direct environment variable fallback (`PROTON_USERNAME`/`PROTON_PASSWORD`) has been removed.

Credential resolution order:

1. `PROTON_PASS_USERNAME_REF` / `PROTON_PASS_PASSWORD_REF` via pass-cli.
2. If only password ref is set, fallback to `pass-cli user info --output json` for username.
3. If credentials cannot be resolved, the adapter exits with an error.

## Helper Script

```bash
pass-cli login
eval "$(./scripts/export-pass-env.sh)"
```

The script verifies that `pass-cli` is authenticated, validates both references, sets `PROTON_PASS_*`, and unsets plaintext credential vars.

## proton-drive-cli Constants

The Go adapter spawns `proton-drive-cli bridge <command>` directly as a subprocess:

| Variable | Default | Purpose |
| --- | --- | --- |
| `PROTON_APP_VERSION` | `external-drive-protonlfs@dev` | Proton client app version header |
| `PROTON_DATA_PASSWORD` | empty | Optional dedicated data password fallback |
| `PROTON_SECOND_FACTOR_CODE` | empty | Optional 2FA code fallback |
| `PROTON_DRIVE_CLI_BIN` | `submodules/proton-drive-cli/dist/index.js` | Path to proton-drive-cli entry point |
| `PROTON_DRIVE_CLI_TIMEOUT_MS` | `300000` | Subprocess command timeout |
| `PROTON_DRIVE_CLI_SESSION_DIR` | `~/.proton-drive-cli` | Session file storage directory |
