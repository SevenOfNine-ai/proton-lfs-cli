# Security Threat Model

## Trust Boundaries

```
[User Machine]
  ├── Git Client (trusted)
  ├── Git LFS (trusted)
  ├── Go Adapter (trusted, our code)
  ├── Node.js LFS Bridge (trusted, our code)
  ├── proton-drive-cli subprocess (trusted, our submodule)
  └── pass-cli (trusted, Proton official)

[Network Boundary]
  └── Proton API (external, TLS-protected)
```

## Attack Surfaces

### 1. Subprocess Command Injection

**Risk**: Malicious OID or file path with shell metacharacters could execute arbitrary commands.

**Mitigations**:

- `spawn()` with array arguments (not shell string) — no shell interpretation
- OID validated against `/^[a-f0-9]{64}$/i` before subprocess spawn
- File paths validated against `..` traversal before use
- Credentials passed via stdin, not command-line arguments

**Tests**: `proton-lfs-bridge/tests/security/command-injection.test.js`

### 2. Credential Exposure

**Risk**: Credentials visible in process list, logs, or on disk.

**Mitigations**:

- Credentials passed via stdin to subprocess (not visible in `ps aux`)
- Credential flow: pass-cli -> Go adapter -> stdin -> proton-drive-cli (memory only)
- **Passwords are never persisted to disk** — `saveSession()` strips `mailboxPassword` before writing
- Session file (`~/.proton-drive-cli/session.json`) contains only revocable tokens (sessionId, accessToken, refreshToken)
- Error messages sanitized — no credential values in responses
- Session tokens stored with 0600 permissions in `~/.proton-drive-cli/`
- Pass-cli references used instead of plaintext env vars

**Tests**: `tests/integration/credential_security_test.go`

### 3. Path Traversal

**Risk**: Malicious paths like `../../etc/passwd` could read/write arbitrary files.

**Mitigations**:

- `path.normalize()` check for `..` sequences in `protonDriveBridge.js`
- OID-to-path conversion uses only validated hex characters
- Download output paths validated before use

### 4. Resource Exhaustion (DoS)

**Risk**: Unlimited subprocess spawns consuming all system resources.

**Mitigations**:

- Subprocess pool limit: maximum 10 concurrent operations
- Per-operation timeout: 5 minutes (configurable)
- Process killed on timeout (SIGKILL)

**Tests**: `proton-lfs-bridge/tests/security/rate-limiting.test.js`

### 5. Session Token Theft

**Risk**: Session file readable by other users on shared systems.

**Mitigations**:

- Session file at `~/.proton-drive-cli/session.json` contains only revocable tokens (no passwords)
- File permissions should be 0600 (owner read-write only)
- Session stored in user home directory, not shared locations
- Tokens can be revoked server-side via `proton-drive logout`

**Validation**: `TestCredentialSessionFilePermissions` in integration tests

### 6. Network Interception

**Risk**: Man-in-the-middle attack on Proton API communication.

**Mitigations**:

- All Proton API calls use HTTPS (TLS)
- SRP authentication — password never sent to server
- E2E encryption — file contents encrypted client-side before upload

## Known Gaps

1. Session file permission enforcement is verified in tests but not actively set by the bridge code (relies on proton-drive-cli).
2. No rate limiting on the HTTP endpoints of the LFS bridge itself (only on subprocess spawns).
3. Debug logging could potentially expose sensitive data — warning added when debug mode is active with SDK backend.
