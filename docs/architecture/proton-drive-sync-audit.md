# Proton Drive Sync Architecture Audit

**Date:** 2026-02-16
**Source Repository:** <https://github.com/SevenOfNine-ai/proton-drive-sync>
**License:** GPL-3.0 (incompatible - NO CODE COPYING)
**Purpose:** Extract architectural patterns and ideas for proton-git-lfs improvements

---

## Executive Summary

Proton Drive Sync is a production-grade TypeScript/Node.js daemon providing bidirectional file synchronization with Proton Drive. The project demonstrates several architectural patterns and reliability strategies that could significantly enhance proton-git-lfs, particularly in areas of:

1. **Authentication & Session Management** - Sophisticated token refresh with parent/child session hierarchy
2. **Error Recovery & Retry Logic** - Database-backed job queue with categorized exponential backoff
3. **Rate Limiting Prevention** - Careful handling of Proton API constraints to avoid IP blocks
4. **State Persistence** - SQLite database via Drizzle ORM for robust state tracking
5. **User Experience** - Interactive CLI setup wizards, clear error messaging, progressive disclosure

### Key Takeaway

Our current implementation is **functional but brittle**. We lack retry logic, have minimal error categorization, no persistent state beyond session files, and limited protection against rate-limiting cascades. Adopting patterns from proton-drive-sync could dramatically improve reliability and user experience.

---

## 1. Authentication & Session Management

### Their Approach

**Parent-Child Session Hierarchy:**

```typescript
// Parent session: Created at login, used for forking child sessions
// Child session: Operational session for API calls, can be regenerated
private async apiRequestWithRefresh<T>(method, endpoint, data) {
  try {
    return await apiRequest(method, endpoint, data, this.session);
  } catch (error) {
    if (apiError.status === 401 && this.session?.RefreshToken) {
      await this.refreshToken();
      return await apiRequest(method, endpoint, data, this.session);
    }
  }
}

```

**Session Persistence:**

- Stores: UID, AccessToken, RefreshToken, UserID, keyPassword, user keys
- `getReusableCredentials()` enables restoration of both parent and child sessions
- Session reuse check: Validates existing session before triggering full SRP auth

**Token Expiration Detection:**

```typescript
private isTokenExpiringSoon(token: string): boolean {
  const decoded = jwtDecode<{ exp: number }>(token);
  const now = Math.floor(Date.now() / 1000);
  return (decoded.exp - now) < 5 * 60; // Refresh 5 minutes before expiry
}

```

**Fork Recovery Mechanism:**

- When child session refresh fails (code 10013), attempt to fork new child from parent
- If parent also expired, trigger full re-authentication
- Prevents cascading session failures

### Current Implementation - Authentication

**Limitations:**

- No token expiration detection (we rely on error-triggered refresh)
- No parent/child session hierarchy (single session model)
- No proactive session validation before operations
- Session refresh in `proton-drive-cli` marked as "not working properly" in README

**What We Do Well:**

- Credential provider abstraction (pass-cli, git-credential)
- Session isolation from Go adapter (credentials never pass through Go)
- Clear separation: proton-drive-cli handles all auth internally

### Recommendations - Authentication

| Priority | Recommendation | Complexity | Impact |
| ---------- | --------------- | ------------ | -------- |
| **HIGH** | Implement proactive token expiration checking | Low | Prevents failed operations |
| **HIGH** | Add session reuse check to avoid unnecessary SRP flows | Medium | Reduces auth overhead & rate-limit risk |
| **MEDIUM** | Implement parent/child session pattern for resilience | High | Improves long-running reliability |
| **MEDIUM** | Add fork recovery when refresh fails | Medium | Better error recovery |
| **LOW** | Store session metadata (creation time, last refresh) | Low | Better observability |

**Implementation Notes:**

- Add `jwtDecode` or equivalent to check token expiration in `proton-drive-cli`
- Modify `AuthService.getSession()` to proactively refresh when exp < 5 minutes
- Add `SessionManager.isSessionForUser(username)` check in bridge `auth` command
- Consider storing "parent session" capability for future fork recovery

---

## 2. Rate Limiting & Anti-Abuse

### Their Approach - Authentication

**No Automatic Retries on Rate Limit:**

```typescript
// Proton API rate-limit (code 2028) — surface actual message, no auto-retry
if (protonCode === 2028) {
  const apiMessage = (data?.Error as string) || 'Too many requests';
  throw new AppError(apiMessage, ErrorCode.RATE_LIMITED, {...}, true);
}

```

**Key Strategy:** Fail fast and surface rate-limit errors to the user rather than retry automatically (which worsens the problem).

**Background Reconciliation Throttle:**

```typescript
// Pause filesystem scanning when pending jobs exceed threshold
if (pendingJobs > BACKGROUND_RECONCILIATION_SKIP_THRESHOLD) {
  logger.info('Skipping background reconciliation: ${pendingJobs} jobs pending');
}

```

**Request Spacing Patterns:**

- Poll interval: `JOB_POLL_INTERVAL_MS` (prevents tight request loops)
- Debounce threshold: `WATCHER_DEBOUNCE_MS` for filesystem changes
- Database-backed job queue with retry scheduling (prevents thundering herd)

**CAPTCHA Handling:**

```typescript
if (error instanceof CaptchaError) {
  return {
    ok: false,
    error: 'CAPTCHA verification required — run: proton-drive login',
    code: 407,
    details: JSON.stringify({ captchaUrl, captchaToken, ... })
  };
}

```

### Current Implementation - Rate Limiting

**Limitations:**

- **CRITICAL:** No explicit rate-limit detection or handling
- No request throttling/spacing beyond subprocess pool (max 10 concurrent)
- No retry backoff for failed operations
- CAPTCHA errors not explicitly handled (fall through to generic error)
- Multiple concurrent Git LFS processes could overwhelm API independently

**What We Do Well:**

- Subprocess concurrency limit (max 10, 5-min timeout)
- Credential resolution happens once per session (not per operation)
- Mock mode prevents API calls during testing

### Recommendations - Rate Limiting

| Priority | Recommendation | Complexity | Impact |
| ---------- | --------------- | ------------ | -------- |
| **CRITICAL** | Detect Proton API code 2028 and surface clear rate-limit message | Low | Prevents IP blocks |
| **HIGH** | Add exponential backoff for transient errors (network, 5xx) | Medium | Reduces API load |
| **HIGH** | Implement CAPTCHA error detection with actionable user guidance | Low | Better UX during rate-limit events |
| **MEDIUM** | Add request spacing/throttling at bridge client level | Medium | Prevents request bursts |
| **MEDIUM** | Log rate-limit events to separate file for monitoring | Low | Better observability |
| **LOW** | Consider global rate limiter across multiple Git LFS processes | High | System-wide protection |

**Implementation Notes:**

- Parse Proton API `Code: 2028` in `bridge.go` error handling
- Return specific error code (e.g., 429) that Go adapter surfaces clearly
- Add `PROTON_REQUEST_DELAY_MS` env var for configurable request spacing
- Implement basic retry with backoff in `bridge.go` `runBridgeCommand`:

  ```go
  for attempt := 1; attempt <= maxRetries; attempt++ {
    resp, err := bc.runBridgeCommand(...)
    if isTransientError(err) && attempt < maxRetries {
      time.Sleep(exponentialBackoff(attempt))
      continue
    }
    return resp, err
  }

  ```

---

## 3. Error Recovery & Retry Logic

### Their Approach - Rate Limiting

**Database-Backed Job Queue with Retry State:**

Schema: `syncJobs` table

```sql

- status: PENDING, PROCESSING, SYNCED, BLOCKED
- retryAt: timestamp (next retry time)
- nRetries: counter
- errorLog: JSON array of error messages

```

**Categorized Error Handling:**

```typescript
function categorizeError(error: unknown): { category: string, maxRetries: number } {
  // Network errors: 5 retries
  // Authentication errors: 2 retries
  // File-not-found: 0 retries (permanent failure)
  // Unknown errors: 3 retries
}

```

**Exponential Backoff with Jitter:**

```typescript
const baseDelay = Math.pow(2, nRetries) * 1000; // 1s, 2s, 4s, 8s, 16s...
const jitter = Math.random() * baseDelay * 0.1;
const retryAt = Date.now() + baseDelay + jitter;

```

**Stale Job Cleanup:**

```typescript
// On startup: "Clean up stale/orphaned data from previous run"
// Detects jobs left in PROCESSING state after crash
// Resets them to PENDING with timestamp check

```

**Graceful Shutdown:**

```typescript
const result = await Promise.race([
  waitForActiveTasks().then(() => 'done'),
  timeoutPromise(SHUTDOWN_TIMEOUT_MS)
]);
// Logs abandoned tasks if timeout exceeded

```

### Current Implementation - Error Recovery

**Limitations:**

- **CRITICAL:** No retry logic whatsoever - single failure = operation fails
- No error categorization (network vs auth vs permanent)
- No persistent operation state (crash = lost state)
- No stale temp file cleanup on startup (we added basic cleanup but no operation recovery)
- No graceful shutdown handling for in-flight operations

**What We Do Well:**

- Subprocess timeout (5 minutes) prevents indefinite hangs
- Atomic status file writes prevent corruption
- SHA-256 verification catches silent corruption

### Recommendations - Error Recovery

| Priority | Recommendation | Complexity | Impact |
| ---------- | --------------- | ------------ | -------- |
| **CRITICAL** | Implement basic retry logic for transient errors (3 attempts) | Medium | Dramatically improves reliability |
| **HIGH** | Categorize errors: transient (retry), auth (re-auth), permanent (fail) | Medium | Smarter error handling |
| **HIGH** | Add exponential backoff with jitter | Low | Prevents thundering herd |
| **MEDIUM** | Persist operation state to allow crash recovery | High | Enables resume after crash |
| **MEDIUM** | Implement graceful shutdown with active task drain | Medium | Cleaner shutdown |
| **LOW** | Add operation-level timeout (separate from subprocess timeout) | Low | Better control |

**Implementation Notes:**

Add to `bridge.go`:

```go
type RetryConfig struct {
  MaxAttempts   int
  BaseDelay     time.Duration
  MaxDelay      time.Duration
}

func (bc *BridgeClient) runWithRetry(command string, request map[string]any, cfg RetryConfig) (*BridgeResponse, error) {
  for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
    resp, err := bc.runBridgeCommand(command, request)

    if err == nil {
      return resp, nil
    }

    errorCategory := categorizeError(err)
    if errorCategory == ErrorPermanent || attempt >= cfg.MaxAttempts {
      return resp, err
    }

    delay := calculateBackoff(attempt, cfg.BaseDelay, cfg.MaxDelay)
    time.Sleep(delay)
  }
  return nil, fmt.Errorf("max retries exceeded")
}

```

Error categorization:

- **Transient:** Network timeouts, 5xx errors, subprocess spawn failures
- **Auth:** 401/403 errors, invalid tokens
- **Permanent:** 404 not found, OID validation failures, path traversal

---

## 4. API Client Architecture

### Their Approach - Error Recovery

**Request/Response Wrapper Pattern:**

```typescript
private async apiRequestWithRefresh<T>(method, endpoint, data) {
  try {
    return await apiRequest(method, endpoint, data, this.session);
  } catch (error) {
    if (apiError.status === 401 && this.session?.RefreshToken) {
      await this.refreshToken();
      return await apiRequest(method, endpoint, data, this.session);
    }
    throw error;
  }
}

```

**Token Injection:**

- Centralized in `httpClientAdapter.ts` via `injectAuthHeaders()`
- Always adds `x-pm-appversion: web-drive@5.2.0`
- Injects `Authorization: Bearer <token>` from session

**Error Response Normalization:**

```typescript
if (isHttpClientError(error)) {
  const data = error.response?.data as Record<string, unknown>;
  const protonCode = data?.Code;  // Proton-specific error code
  const apiMessage = data?.Error;  // User-facing message
  // Standardize into AppError with context
}

```

**SDK Client Abstraction:**

- Uses official `@protontech/drive-sdk` (v0.7.0)
- Custom auth layer wraps SDK for credential resolution
- All Drive operations go through SDK (no raw HTTP for file ops)

### Current Implementation - API Architecture

**Limitations:**

- No automatic token refresh on 401 (fails immediately)
- No centralized error response parsing
- No request/response interceptors
- Bridge protocol envelope is simple but lacks metadata (request ID, timing, etc.)

**What We Do Well:**

- Clean JSON envelope protocol (`{ ok, payload, error, code }`)
- Credential provider abstraction works well
- SDK integration via proton-drive-cli is isolated from Go
- Environment filtering prevents credential leakage

### Recommendations - API Architecture

| Priority | Recommendation | Complexity | Impact |
| ---------- | --------------- | ------------ | -------- |
| **HIGH** | Add automatic token refresh on 401 in bridge handlers | Medium | Better resilience |
| **MEDIUM** | Centralize Proton API error parsing in proton-drive-cli | Medium | Consistent error messages |
| **MEDIUM** | Add request IDs to bridge protocol for tracing | Low | Better debugging |
| **LOW** | Add timing metadata to bridge responses | Low | Performance monitoring |
| **LOW** | Log SDK version in status reports | Low | Better support |

**Implementation Notes:**

In `src/cli/bridge.ts`:

```typescript
async function handleWithAutoRefresh(handler: () => Promise<void>): Promise<void> {
  try {
    await handler();
  } catch (error: any) {
    if (error?.response?.status === 401 || error?.code === ErrorCode.AUTH_FAILED) {
      // Attempt token refresh
      const authService = new AuthService();
      await authService.refreshSession();
      // Retry handler once
      await handler();
      return;
    }
    throw error;
  }
}

```

Add request tracking:

```typescript
const requestId = crypto.randomUUID();
const startTime = Date.now();
try {
  // ... operation
  writeSuccess({ requestId, duration: Date.now() - startTime, ... });
} catch (error) {
  writeError(message, code, JSON.stringify({ requestId, duration: Date.now() - startTime }));
}

```

---

## 5. Reliability & Robustness

### Their Approach - API Architecture

**Database for State Persistence:**

- SQLite via Drizzle ORM
- Tables: `syncJobs`, `fileState`, `nodeMapping`, `processingQueue`
- Atomic transactions prevent race conditions
- Enables crash recovery and audit trails

**Stale State Cleanup:**

```typescript
// On startup: Clean up stale/orphaned data from previous run
// Remove processingQueue entries older than threshold
// Reset stuck PROCESSING jobs to PENDING

```

**File Change Detection:**

```typescript
// Change token comparison: "mtime:size"
// Skip re-upload if change token matches
// Faster than hashing file content

```

**Concurrency Control:**

```typescript
// Transaction-based job claiming
return db.transaction((tx) => {
  const job = tx.select().from(syncJobs).where(...).limit(1);
  if (!job) return null;
  tx.update(syncJobs).set({ status: 'PROCESSING' });
  tx.insert(processingQueue).values({ jobId: job.id });
  return job;
});

```

**Graceful Degradation:**

- Logs errors but continues processing other jobs
- Orphaned state is cleaned rather than causing crashes
- Dashboard continues working even if sync daemon crashes

### Current Implementation - Reliability

**Limitations:**

- No persistent operation state (everything in-memory)
- No crash recovery mechanism
- No transaction safety for multi-step operations
- Status file is fire-and-forget (tray may read stale data)
- No audit trail of past operations

**What We Do Well:**

- Atomic status file writes (tmp + rename)
- SHA-256 verification ensures data integrity
- Subprocess isolation limits blast radius
- Concurrency limit prevents resource exhaustion

### Recommendations - Reliability

| Priority | Recommendation | Complexity | Impact |
| ---------- | --------------- | ------------ | -------- |
| **HIGH** | Add operation history log (last 100 operations) | Medium | Better debugging & audit |
| **MEDIUM** | Implement change token caching to skip re-uploads | Medium | Performance improvement |
| **MEDIUM** | Add stale temp file cleanup on adapter startup | Low | Better resource management |
| **MEDIUM** | Status file versioning to detect corruption | Low | Better error detection |
| **LOW** | Consider SQLite for long-running daemon mode | High | Enables full crash recovery |

**Implementation Notes:**

Operation history in `internal/config/status.go`:

```go
type OperationHistory struct {
  Operations []OperationRecord `json:"operations"`
  MaxEntries int               `json:"-"`
}

type OperationRecord struct {
  Timestamp time.Time `json:"timestamp"`
  Operation string    `json:"operation"` // "upload", "download"
  OID       string    `json:"oid"`
  Status    string    `json:"status"` // "success", "error"
  Error     string    `json:"error,omitempty"`
  Duration  int64     `json:"duration_ms"`
}

func AppendOperation(record OperationRecord) error {
  history := loadHistory()
  history.Operations = append(history.Operations, record)
  if len(history.Operations) > history.MaxEntries {
    history.Operations = history.Operations[1:]
  }
  return saveHistory(history)
}

```

Change token for deduplication in `backend.go`:

```go
type UploadCache struct {
  OID         string    `json:"oid"`
  ChangeToken string    `json:"change_token"` // "mtime:size"
  UploadedAt  time.Time `json:"uploaded_at"`
}

func shouldSkipUpload(oid, path string) (bool, error) {
  cache := loadUploadCache(oid)
  if cache == nil {
    return false, nil
  }

  stat, err := os.Stat(path)
  if err != nil {
    return false, err
  }

  currentToken := fmt.Sprintf("%d:%d", stat.ModTime().Unix(), stat.Size())
  return cache.ChangeToken == currentToken, nil
}

```

---

## 6. Performance Optimizations

### Their Approach - Reliability

**Connection Pooling:**

- SDK client maintains persistent HTTP connections
- Session tokens cached in memory
- No per-operation authentication overhead

**Parallel Operations:**

```typescript
// Configurable concurrency (default: 4)
setSyncConcurrency(config.sync_concurrency);

// Task pool pattern
const activeTasks = new Map<string, Promise<void>>();
for (let i = 0; i < available; i++) {
  const job = await getNextJob();
  activeTasks.set(job.id, processJob(job));
}

```

**Change Token Optimization:**

```typescript
// Faster than re-hashing file content
const changeToken = `${mtime_ms}:${size}`;
if (cached.changeToken === changeToken) {
  return; // Skip upload
}

```

**Batch Operations:**

```typescript
// Transaction batching: Multi-operation updates in single DB transaction
db.transaction((tx) => {
  tx.update(nodeMapping).set({ ... });
  tx.insert(fileState).values({ ... });
  tx.update(syncJobs).set({ status: 'SYNCED' });
});

```

**Directory Recursion Optimization:**

```typescript
// When creating directories, queue children as separate jobs
// Prevents blocking on large directory trees
for (const child of children) {
  queueJob({ type: 'CREATE', path: child.path });
}

```

**SDK Cache Awareness:**

```typescript
// Iterate through ALL children to ensure SDK cache marked complete
// SDK only sets isFolderChildrenLoaded after full iteration
for (const child of client.iterateFolderChildren(parentUid)) {
  if (child.name === target) { found = child; }
}
return found; // Continue iterating even after match

```

### Current Implementation - Performance

**Limitations:**

- No operation batching (each upload/download is independent)
- No change token caching (always re-hash on upload)
- No parallel operation support in adapter (Git LFS spawns multiple processes instead)
- Subprocess overhead for every bridge operation
- No persistent connection pooling (each operation creates new session)

**What We Do Well:**

- Git LFS handles concurrency via multiple adapter instances
- Subprocess pool prevents resource exhaustion (max 10)
- SHA-256 verification ensures correctness over speed
- Temp file + rename for atomic downloads

### Recommendations - Performance

| Priority | Recommendation | Complexity | Impact |
| ---------- | --------------- | ------------ | -------- |
| **HIGH** | Implement change token caching to skip redundant uploads | Medium | Major performance win |
| **MEDIUM** | Add batch exists check before upload operations | Medium | Reduces API calls |
| **MEDIUM** | Session reuse across operations (daemon mode) | High | Eliminates auth overhead |
| **LOW** | Add connection pooling in proton-drive-cli | Medium | Faster operations |
| **LOW** | Parallel batch operations (batch-exists, batch-delete) | Low | Better throughput |

**Implementation Notes:**

Change token cache in `cmd/adapter/backend.go`:

```go
type DriveCLIBackend struct {
  bridge      *BridgeClient
  credProvider string
  uploadCache map[string]UploadCacheEntry // OID -> cache entry
  cacheMutex  sync.RWMutex
}

func (b *DriveCLIBackend) Upload(session *Session, oid, path string, size int64) (int64, error) {
  // Check cache
  if b.shouldSkipUpload(oid, path) {
    return size, nil
  }

  // Perform upload
  err := b.bridge.Upload(creds, oid, path)
  if err != nil {
    return 0, err
  }

  // Update cache
  b.updateUploadCache(oid, path)
  return size, nil
}

```

Batch optimization in `bridge.go`:

```go
// Before upload, check if object already exists
exists, err := bc.Exists(creds, oid)
if err != nil {
  return 0, err
}
if exists {
  return size, nil // Skip upload (deduplication)
}

```

---

## 7. User Experience

### Their Approach - Performance

**Interactive Setup Wizard:**

```typescript
// Sequential steps with clear section headers
showBanner();
showSection('1. Dashboard Access');
showSection('2. Service Installation');
showSection('3. Authentication');
showSection('4. Delete Behavior');
showSection('5. Advanced Settings');

```

**Progressive Disclosure:**

```typescript
const username = process.env.PROTON_USERNAME ||
  await input({ message: 'Proton username:' });
// Environment variable fallback allows automation

```

**Existing State Detection:**

```typescript
const existingCredentials = await getStoredCredentials();
if (existingCredentials) {
  logger.info(`Existing authentication found for '${existingCredentials.username}'.`);
  const reauth = await confirm({
    message: 'Re-authenticate?',
    default: false
  });
  if (!reauth) return;
}

```

**Actionable Error Messages:**

```

❌ CAPTCHA verification required — run: proton-drive login
❌ Rate limited by Proton API — wait and retry
✓ Session is valid.
✓ Credentials saved securely.

```

**Multi-Channel Status:**

- CLI commands: `status`, `logs`, `dashboard`
- Web dashboard: `http://localhost:4242`
- IPC communication between daemon and UI
- Desktop notifications (implied by architecture)

**Validation with Helpful Feedback:**

```typescript
// Validates existing service installation
const installed = await isServiceInstalled();
if (installed) {
  logger.warn('Service already installed. Use --force to reinstall.');
}

```

### Current Implementation - User Experience

**Limitations:**

- No interactive setup wizard (manual git config + credential setup)
- Error messages are technical (exposing internal details)
- No detection of existing configuration
- No guidance for CAPTCHA or rate-limit scenarios
- Tray app status is minimal (just icon colors)
- No web dashboard or rich status interface

**What We Do Well:**

- Credential provider abstraction (users choose pass-cli or git-credential)
- Clear --help text with examples
- Status file protocol for tray communication
- Separation of concerns (adapter, tray, CLI are independent)

### Recommendations - User Experience

| Priority | Recommendation | Complexity | Impact |
| ---------- | --------------- | ------------ | -------- |
| **HIGH** | Add setup wizard: `proton-drive setup` command | High | Much better onboarding |
| **HIGH** | Improve error messages with actionable guidance | Medium | Better UX during issues |
| **MEDIUM** | Add `proton-drive status` command (richer than tray icon) | Low | Better visibility |
| **MEDIUM** | Detect existing config and offer validation | Medium | Reduces setup errors |
| **LOW** | Add desktop notifications for errors (via tray) | Medium | Better error awareness |
| **LOW** | Web dashboard for status/logs (long-term) | Very High | Professional UX |

**Implementation Notes:**

Setup wizard in `submodules/proton-drive-cli/src/cli/setup.ts`:

```typescript
export async function setupCommand() {
  console.log('\n━━━ Proton Git LFS Setup ━━━\n');

  // Step 1: Credential provider
  const provider = await select({
    message: 'Choose credential provider:',
    choices: [
      { value: 'pass-cli', name: 'Proton Pass CLI (recommended)' },
      { value: 'git-credential', name: 'Git Credential Manager' },
    ],
  });

  // Step 2: Test authentication
  console.log('\nTesting authentication...');
  const authService = new AuthService();
  // ... validate credentials

  // Step 3: Git config
  console.log('\nConfiguring Git LFS...');
  execSync('git config lfs.customtransfer.proton.path ...', { stdio: 'inherit' });

  console.log('\n✓ Setup complete! Test with: git lfs push origin main\n');
}

```

Improved error messages in `bridge.ts`:

```typescript
function formatUserFacingError(error: any): string {
  if (error instanceof CaptchaError) {
    return 'CAPTCHA required. Run: proton-drive login';
  }
  if (error?.response?.data?.Code === 2028) {
    return 'Rate limited by Proton. Wait 10 minutes and retry.';
  }
  if (error?.code === 'ECONNREFUSED') {
    return 'Cannot connect to Proton API. Check your internet connection.';
  }
  return error.message || 'Operation failed';
}

```

---

## 8. Code Pattern Comparisons

### Credential Resolution

**Proton Drive Sync Pattern:**

```typescript
// Unified provider abstraction
const provider = createProvider(request.credentialProvider);
const cred = await provider.resolve({ username: request.username });

```

**Our Pattern:**

```go
// Provider name sent to proton-drive-cli, which resolves internally
req["credentialProvider"] = creds.CredentialProvider
resp, err := bc.runBridgeCommand("auth", req)

```

**Analysis:** Both use provider abstraction, but we delegate resolution to the TypeScript layer while they have a unified credential interface. Our approach is simpler but less testable.

**Recommendation:** Keep our pattern (simpler), but add `proton-drive credential verify` command for testing.

---

### Error Handling

**Proton Drive Sync Pattern:**

```typescript
try {
  await operation();
} catch (error) {
  const category = categorizeError(error);
  if (category === ErrorCategory.Transient && attempt < maxRetries) {
    await sleep(exponentialBackoff(attempt));
    return retry();
  }
  throw error;
}

```

**Our Pattern:**

```go
resp, err := bc.runBridgeCommand(command, request)
if err != nil {
  return nil, err // Immediate failure, no retry
}

```

**Analysis:** We fail immediately on any error. They categorize and retry transient failures.

**Recommendation:** HIGH priority - add retry logic to `bridge.go`.

---

### State Persistence

**Proton Drive Sync Pattern:**

```typescript
// SQLite database with migration support
const db = drizzle(connection);
await db.insert(syncJobs).values({ ... });

```

**Our Pattern:**

```go
// JSON file (atomic write)
config.WriteStatus(config.StatusReport{ State: "ok", ... })

```

**Analysis:** They use database for queryable state. We use simple file for status reporting.

**Recommendation:** MEDIUM priority - add operation history log (100 entries) to status system.

---

### Session Management

**Proton Drive Sync Pattern:**

```typescript
// Proactive token refresh
if (this.isTokenExpiringSoon(session.accessToken)) {
  return await this.refreshSession();
}

```

**Our Pattern:**

```typescript
// No proactive refresh - relies on 401 error handling
const session = await SessionManager.loadSession();
return session;

```

**Analysis:** They prevent 401 errors by refreshing early. We wait for failure.

**Recommendation:** HIGH priority - add proactive token refresh to `AuthService.getSession()`.

---

### Concurrency Control

**Proton Drive Sync Pattern:**

```typescript
// Database transaction for atomic job claiming
const job = await db.transaction((tx) => {
  const job = tx.select().from(syncJobs).where(...);
  tx.update(syncJobs).set({ status: 'PROCESSING' });
  return job;
});

```

**Our Pattern:**

```go
// Channel-based semaphore
select {
case bc.semaphore <- struct{}{}:
  defer func() { <-bc.semaphore }()
default:
  return nil, fmt.Errorf("concurrency limit reached")
}

```

**Analysis:** They use database for multi-process coordination. We use in-process semaphore (simpler, sufficient for our model).

**Recommendation:** Keep our pattern - Git LFS spawns separate processes, each with its own limit.

---

## 9. Priority Matrix

### Critical (Do First)

| Item | Effort | Impact | Notes |
| ------ | -------- | -------- | ------- |
| Detect & surface Proton API code 2028 (rate-limit) | Low | Critical | Prevents IP blocks |
| Implement basic retry logic (3 attempts, exponential backoff) | Medium | Critical | Dramatically improves reliability |
| Add CAPTCHA error detection with user guidance | Low | Critical | Better UX during auth issues |

### High Priority (Next Quarter)

| Item | Effort | Impact | Notes |
| ------ | -------- | -------- | ------- |
| Proactive token expiration checking | Low | High | Prevents failed operations |
| Session reuse check (skip SRP if valid session exists) | Medium | High | Reduces rate-limit risk |
| Categorize errors (transient, auth, permanent) | Medium | High | Enables smart retry logic |
| Change token caching (skip redundant uploads) | Medium | High | Major performance win |
| Setup wizard command | High | High | Much better onboarding |
| Improve error messages (actionable guidance) | Medium | High | Better UX |
| Operation history log (last 100 ops) | Medium | High | Better debugging |

### Medium Priority (6-12 Months)

| Item | Effort | Impact | Notes |
| ------ | -------- | -------- | ------- |
| Parent/child session pattern | High | Medium | Improves long-running reliability |
| Fork recovery when refresh fails | Medium | Medium | Better error recovery |
| Batch exists check before uploads | Medium | Medium | Reduces API calls |
| Request spacing/throttling | Medium | Medium | Prevents request bursts |
| Centralize Proton API error parsing | Medium | Medium | Consistent error messages |
| Persistent operation state (crash recovery) | High | Medium | Enables resume after crash |
| Graceful shutdown with active task drain | Medium | Medium | Cleaner shutdown |

### Low Priority (Nice to Have)

| Item | Effort | Impact | Notes |
| ------ | -------- | -------- | ------- |
| Store session metadata (creation time, last refresh) | Low | Low | Better observability |
| Add request IDs to bridge protocol | Low | Low | Better debugging |
| Add timing metadata to bridge responses | Low | Low | Performance monitoring |
| Log SDK version in status reports | Low | Low | Better support |
| Desktop notifications for errors | Medium | Low | Better error awareness |
| Web dashboard for status/logs | Very High | Low | Professional UX (long-term) |
| SQLite for daemon mode | High | Low | Only if building long-running daemon |
| Connection pooling in proton-drive-cli | Medium | Low | Marginal performance gain |
| Global rate limiter across processes | High | Low | Complex, low ROI |

---

## 10. Implementation Roadmap

### Phase 1: Stability & Reliability (Critical - 2 weeks)

**Goal:** Prevent IP blocks and handle transient failures gracefully.

**Tasks:**

1. Add Proton API code 2028 detection in `bridge.go` error handling
2. Implement basic retry logic with exponential backoff (3 attempts)
3. Add CAPTCHA error detection and user guidance
4. Test retry behavior with simulated network failures

**Success Criteria:**

- Rate-limit errors surface clear messages: "Rate limited by Proton. Wait 10 minutes."
- Transient network errors retry automatically (3x with backoff)
- CAPTCHA errors provide actionable guidance

---

### Phase 2: Authentication Improvements (High - 2 weeks)

**Goal:** Reduce unnecessary SRP authentication and prevent token expiration failures.

**Tasks:**

1. Add JWT expiration checking in `AuthService.getSession()`
2. Implement proactive token refresh (5 minutes before expiry)
3. Add session reuse check in bridge `auth` command
4. Add `SessionManager.isSessionForUser(username)` helper

**Success Criteria:**

- Token expiration detected and refreshed proactively
- Existing sessions reused (skip SRP when valid)
- Reduced auth API calls by ~80%

---

### Phase 3: Error Handling & Categorization (High - 2 weeks)

**Goal:** Smart error handling with appropriate retry strategies.

**Tasks:**

1. Create error categorization function (transient, auth, permanent)
2. Implement different retry strategies per category
3. Surface auth errors with re-authentication guidance
4. Add permanent error detection (404, validation failures)

**Success Criteria:**

- Network timeouts retry automatically
- 401 errors trigger re-authentication flow
- 404 errors fail immediately (no retry)

---

### Phase 4: Performance & Caching (High - 3 weeks)

**Goal:** Major performance improvements through change token caching.

**Tasks:**

1. Implement upload cache (OID -> mtime:size)
2. Add cache persistence (JSON file in ~/.proton-git-lfs/upload-cache.json)
3. Implement batch exists check before uploads
4. Add cache invalidation on manual delete

**Success Criteria:**

- Redundant uploads skipped (same file reuploaded = instant success)
- Batch exists reduces API calls for multi-file operations
- Cache persists across adapter runs

---

### Phase 5: Observability & UX (Medium - 3 weeks)

**Goal:** Better visibility into operations and improved onboarding.

**Tasks:**

1. Add operation history log (last 100 operations)
2. Implement `proton-drive status` command (richer than tray icon)
3. Create interactive setup wizard
4. Improve error messages with actionable guidance
5. Add operation timing to status reports

**Success Criteria:**

- Users can see recent operation history
- Setup wizard guides new users through configuration
- Error messages provide clear next steps

---

### Phase 6: Advanced Reliability (Medium - 4 weeks)

**Goal:** Production-grade error recovery and state management.

**Tasks:**

1. Implement parent/child session pattern
2. Add fork recovery when refresh fails
3. Implement graceful shutdown with task drain
4. Add stale temp file cleanup on startup
5. Status file versioning for corruption detection

**Success Criteria:**

- Long-running sessions stay alive via fork recovery
- Clean shutdown drains in-flight operations
- Stale state detected and cleaned automatically

---

## 11. Testing Strategy

### Unit Tests

- Error categorization logic
- Exponential backoff calculation
- Token expiration detection
- Change token generation

### Integration Tests

- Retry behavior with mock API failures
- Rate-limit error handling (code 2028)
- CAPTCHA error flow
- Session refresh on expiration
- Change token cache hit/miss

### E2E Tests

- Full setup wizard flow
- Multi-file upload with deduplication
- Network interruption recovery
- Concurrent operations across multiple Git LFS processes

### Manual Testing

- Rate-limit scenario (intentionally trigger 2028)
- CAPTCHA flow (new account)
- Long-running session (24+ hours)
- Crash recovery

---

## 12. Risks & Mitigation

| Risk | Likelihood | Impact | Mitigation |
| ------ | ----------- | -------- | ------------ |
| Retry logic causes cascading rate-limits | Medium | High | Implement per-error-type retry limits, exponential backoff with jitter, fail-fast on 2028 |
| Change token cache causes stale uploads | Low | High | Include file size in token, add cache invalidation, add TTL |
| Token refresh race condition (multiple processes) | Medium | Medium | Use file locking or accept occasional redundant refresh |
| Database migration complexity (if adding SQLite) | Low | High | Start with JSON files, migrate to SQLite only for daemon mode |
| Setup wizard masks underlying config complexity | Medium | Low | Provide both wizard and manual config paths |

---

## 13. Conclusion

Proton Drive Sync demonstrates production-grade patterns that would significantly improve proton-git-lfs reliability and user experience. The most impactful improvements are:

1. **Rate-limit detection and handling** - Critical to prevent IP blocks
2. **Retry logic with exponential backoff** - Dramatically improves reliability
3. **Proactive token refresh** - Prevents operation failures
4. **Change token caching** - Major performance improvement
5. **Setup wizard** - Much better onboarding experience

The phased roadmap prioritizes stability and reliability first (Phase 1-2), then performance (Phase 3-4), and finally UX improvements (Phase 5-6). This approach ensures we fix critical issues before adding enhancements.

**Estimated Total Effort:** 16 weeks (4 months) for Phases 1-6
**Recommended Start:** Phase 1 (Stability & Reliability) - 2 weeks

---

## Appendix: Key Differences

| Aspect | Proton Drive Sync | Proton Git LFS |
| -------- | ------------------- | ---------------- |
| **Architecture** | Long-running daemon with SQLite | Ephemeral subprocess per Git LFS operation |
| **Concurrency** | Single process, configurable parallelism | Multiple processes (Git LFS spawns many) |
| **State Persistence** | SQLite database | JSON status file |
| **Error Recovery** | Database-backed retry queue | No retry (fails immediately) |
| **Session Management** | Parent/child hierarchy | Single session |
| **UI** | CLI + web dashboard + daemon | CLI + system tray |
| **Language** | TypeScript/Node.js | Go + TypeScript bridge |
| **Use Case** | Bidirectional sync | Git LFS transfer only |

The architectural differences mean we can't copy all patterns directly, but the **error handling, retry logic, and token management patterns are highly applicable** to our use case.

---

**Document Version:** 1.0
**Last Updated:** 2026-02-16
**Next Review:** After Phase 1 completion
