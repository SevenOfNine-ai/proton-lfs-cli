/**
 * Proton Drive bridge runner.
 *
 * Executes proton-drive-cli as a subprocess for Git LFS operations.
 * Replaces the previous .NET bridge with a TypeScript implementation
 * from submodules/proton-drive-cli.
 *
 * Credentials are passed via stdin (never via command-line arguments)
 * to prevent exposure in the process list.
 */

const path = require('path');
const { spawn } = require('child_process');
const logger = require('./logger');
const config = require('./config');

const MAX_BRIDGE_OUTPUT_DETAILS = 4 * 1024;

// OID validation (64-character hex string)
const OID_RE = /^[a-f0-9]{64}$/i;

// Subprocess pool tracking
let activeSubprocesses = 0;

class BridgeError extends Error {
  constructor(message, code = 502, details = '') {
    super(message);
    this.name = 'BridgeError';
    this.code = code;
    this.details = details;
  }
}

function validateOid(oid) {
  if (!oid || typeof oid !== 'string') {
    throw new BridgeError('OID is required', 400);
  }
  if (!OID_RE.test(oid)) {
    throw new BridgeError('Invalid OID format: expected 64-character hex string', 400);
  }
}

function validatePath(filePath) {
  if (!filePath || typeof filePath !== 'string') {
    throw new BridgeError('File path is required', 400);
  }
  const normalized = path.normalize(filePath);
  if (normalized.includes('..')) {
    throw new BridgeError('Path traversal not allowed', 400);
  }
}

function resolveCommand(command) {
  return {
    executable: process.execPath, // node
    args: [config.PROTON_DRIVE_CLI_BIN, 'bridge', command]
  };
}

function outputTail(raw) {
  const text = String(raw || '').trim();
  if (text.length <= MAX_BRIDGE_OUTPUT_DETAILS) {
    return text;
  }
  return text.slice(text.length - MAX_BRIDGE_OUTPUT_DETAILS);
}

function collectJsonCandidates(raw) {
  const normalized = String(raw || '').replace(/\uFEFF/g, '').trim();
  if (!normalized) {
    return [];
  }

  const candidates = [normalized];
  const lines = normalized.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
  for (let index = lines.length - 1; index >= 0; index -= 1) {
    const line = lines[index];
    if (line.startsWith('{') || line.startsWith('[')) {
      candidates.push(line);
    }
  }

  const lastObjectStart = normalized.lastIndexOf('{');
  if (lastObjectStart >= 0) {
    candidates.push(normalized.slice(lastObjectStart).trim());
  }

  const lastArrayStart = normalized.lastIndexOf('[');
  if (lastArrayStart >= 0) {
    candidates.push(normalized.slice(lastArrayStart).trim());
  }

  return [...new Set(candidates)];
}

function parseBridgeEnvelope(parsed, fallbackErrorCode) {
  if (parsed && parsed.ok === false) {
    throw new BridgeError(
      parsed.error || 'proton bridge operation failed',
      Number.isInteger(parsed.code) ? parsed.code : fallbackErrorCode,
      parsed.details || ''
    );
  }
  if (parsed && parsed.ok === true && Object.prototype.hasOwnProperty.call(parsed, 'payload')) {
    return parsed.payload;
  }
  return parsed;
}

function combineProcessOutput(stdout, stderr) {
  const out = String(stdout || '').trim();
  const err = String(stderr || '').trim();

  if (out && err) {
    return outputTail(`${out}\n${err}`);
  }
  return outputTail(out || err);
}

function parseBridgeOutput(raw, fallbackErrorCode) {
  const trimmed = String(raw || '').trim();
  if (trimmed === '') {
    return null;
  }

  const candidates = collectJsonCandidates(trimmed);
  for (const candidate of candidates) {
    try {
      const parsed = JSON.parse(candidate);
      return parseBridgeEnvelope(parsed, fallbackErrorCode);
    } catch (error) {
      if (error instanceof BridgeError) {
        throw error;
      }
    }
  }

  throw new BridgeError('failed to parse proton bridge response', fallbackErrorCode, outputTail(trimmed));
}

function runBridge(command, payload) {
  return new Promise((resolve, reject) => {
    // Enforce subprocess pool limit
    if (activeSubprocesses >= config.MAX_CONCURRENT_SUBPROCESSES) {
      reject(new BridgeError('Too many concurrent bridge operations', 503));
      return;
    }

    const timeoutMs = config.PROTON_DRIVE_CLI_TIMEOUT_MS;
    const { executable, args } = resolveCommand(command);

    logger.info(`Executing proton-drive-cli bridge command: ${command}`);

    activeSubprocesses += 1;

    const child = spawn(executable, args, {
      env: { ...process.env },
      stdio: ['pipe', 'pipe', 'pipe']
    });

    let stdout = '';
    let stderr = '';
    let finished = false;

    const timeout = setTimeout(() => {
      if (finished) {
        return;
      }
      finished = true;
      activeSubprocesses = Math.max(0, activeSubprocesses - 1);
      child.kill('SIGKILL');
      reject(new BridgeError('proton bridge command timed out', 504));
    }, timeoutMs);

    const finalize = (handler) => {
      if (finished) {
        return;
      }
      finished = true;
      activeSubprocesses = Math.max(0, activeSubprocesses - 1);
      clearTimeout(timeout);
      handler();
    };

    child.stdout.on('data', (chunk) => {
      stdout += chunk.toString();
    });
    child.stderr.on('data', (chunk) => {
      stderr += chunk.toString();
    });

    child.on('error', (error) => {
      finalize(() => {
        const details = String(error && error.message ? error.message : '');
        reject(new BridgeError('failed to execute proton bridge command', 502, details));
      });
    });

    child.on('close', (code) => {
      finalize(() => {
        if (code !== 0) {
          try {
            // Preserve structured bridge errors when the bridge emitted JSON
            parseBridgeOutput(stdout, 500);
          } catch (error) {
            if (error instanceof BridgeError && error.message !== 'failed to parse proton bridge response') {
              if (stderr.trim()) {
                error.details = outputTail([error.details, stderr.trim()].filter(Boolean).join('\n'));
              }
              reject(error);
              return;
            }
          }

          reject(new BridgeError(`proton bridge command failed (exit ${code})`, 500, combineProcessOutput(stdout, stderr)));
          return;
        }

        try {
          const parsed = parseBridgeOutput(stdout, 502);
          resolve(parsed || {});
        } catch (error) {
          if (error instanceof BridgeError && stderr.trim()) {
            error.details = outputTail([error.details, stderr.trim()].filter(Boolean).join('\n'));
          }
          reject(error);
        }
      });
    });

    // Pass credentials via stdin (NOT command-line args)
    const serialized = JSON.stringify(payload || {});
    child.stdin.write(serialized);
    child.stdin.end();
  });
}

function normalizeCredentials(credentials = {}) {
  return {
    username: String(credentials.username || '').trim(),
    password: String(credentials.password || ''),
    dataPassword: String(credentials.dataPassword || ''),
    secondFactorCode: String(credentials.secondFactorCode || '')
  };
}

async function authenticate(credentials = {}) {
  const normalized = normalizeCredentials(credentials);
  if (!normalized.username || !normalized.password) {
    throw new BridgeError('username and password are required for proton bridge mode', 400);
  }

  await runBridge('auth', {
    ...normalized,
    appVersion: config.PROTON_APP_VERSION
  });
}

async function uploadFile(credentials = {}, oid, filePath) {
  const normalized = normalizeCredentials(credentials);
  validateOid(oid);
  validatePath(filePath);

  const response = await runBridge('upload', {
    ...normalized,
    oid: String(oid || '').trim(),
    path: String(filePath || '').trim(),
    storageBase: config.LFS_STORAGE_BASE,
    appVersion: config.PROTON_APP_VERSION
  });
  return response || {};
}

async function downloadFile(credentials = {}, oid, outputPath) {
  const normalized = normalizeCredentials(credentials);
  validateOid(oid);
  validatePath(outputPath);

  const response = await runBridge('download', {
    ...normalized,
    oid: String(oid || '').trim(),
    outputPath: String(outputPath || '').trim(),
    storageBase: config.LFS_STORAGE_BASE,
    appVersion: config.PROTON_APP_VERSION
  });
  return response || {};
}

async function listFiles(credentials = {}, folder) {
  const normalized = normalizeCredentials(credentials);
  const response = await runBridge('list', {
    ...normalized,
    folder: String(folder || config.LFS_STORAGE_BASE).trim(),
    storageBase: config.LFS_STORAGE_BASE,
    appVersion: config.PROTON_APP_VERSION
  });
  return Array.isArray(response && response.files) ? response.files : [];
}

module.exports = {
  BridgeError,
  authenticate,
  uploadFile,
  downloadFile,
  listFiles
};
