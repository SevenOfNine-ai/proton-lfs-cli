/**
 * Real Proton Drive bridge runner.
 *
 * Executes the local .NET bridge tool that uses Proton's C# SDK.
 * This mode is intentionally fail-closed: if the tool is missing or fails,
 * callers receive structured errors.
 */

const path = require('path');
const { spawn } = require('child_process');
const logger = require('./logger');

const DEFAULT_STORAGE_BASE = process.env.LFS_STORAGE_BASE || 'LFS';
const DEFAULT_APP_VERSION = process.env.PROTON_APP_VERSION || 'external-drive-protonlfs@dev';
const DEFAULT_BRIDGE_TIMEOUT_MS = 5 * 60 * 1000;
const MAX_BRIDGE_OUTPUT_DETAILS = 4 * 1024;
const DEFAULT_BRIDGE_PROJECT = path.resolve(
  __dirname,
  '..',
  'tools',
  'proton-real-bridge',
  'ProtonRealBridge.csproj'
);

function parseTimeoutMs(value, fallback) {
  const parsed = Number.parseInt(String(value || ''), 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return fallback;
  }
  return parsed;
}

function trimmedEnv(name) {
  return String(process.env[name] || '').trim();
}

class BridgeError extends Error {
  constructor(message, code = 502, details = '') {
    super(message);
    this.name = 'BridgeError';
    this.code = code;
    this.details = details;
  }
}

function resolveCommand(command) {
  const bridgeBinary = trimmedEnv('PROTON_REAL_BRIDGE_BIN');
  if (bridgeBinary) {
    return {
      executable: bridgeBinary,
      args: [command]
    };
  }

  const projectPath = trimmedEnv('PROTON_REAL_BRIDGE_PROJECT') || DEFAULT_BRIDGE_PROJECT;
  const configuration = trimmedEnv('PROTON_REAL_BRIDGE_CONFIGURATION') || 'Release';

  return {
    executable: 'dotnet',
    args: ['run', '--project', projectPath, '--configuration', configuration, '--', command]
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
    const timeoutMs = parseTimeoutMs(process.env.PROTON_REAL_BRIDGE_TIMEOUT_MS, DEFAULT_BRIDGE_TIMEOUT_MS);
    const { executable, args } = resolveCommand(command);

    logger.info(`Executing real proton bridge command: ${command}`);

    const childEnv = { ...process.env };
    if (executable === 'dotnet') {
      childEnv.DOTNET_NOLOGO = childEnv.DOTNET_NOLOGO || '1';
      childEnv.DOTNET_SKIP_FIRST_TIME_EXPERIENCE = childEnv.DOTNET_SKIP_FIRST_TIME_EXPERIENCE || '1';
      childEnv.DOTNET_CLI_TELEMETRY_OPTOUT = childEnv.DOTNET_CLI_TELEMETRY_OPTOUT || '1';
    }

    const child = spawn(executable, args, {
      env: childEnv,
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
      child.kill('SIGKILL');
      reject(new BridgeError('proton bridge command timed out', 504));
    }, timeoutMs);

    const finalize = (handler) => {
      if (finished) {
        return;
      }
      finished = true;
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
            // Preserve structured bridge errors when the bridge emitted JSON.
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
    throw new BridgeError('username and password are required for real proton mode', 400);
  }

  await runBridge('auth', {
    ...normalized,
    appVersion: DEFAULT_APP_VERSION
  });
}

async function uploadFile(credentials = {}, oid, filePath) {
  const normalized = normalizeCredentials(credentials);
  const response = await runBridge('upload', {
    ...normalized,
    oid: String(oid || '').trim(),
    path: String(filePath || '').trim(),
    storageBase: DEFAULT_STORAGE_BASE,
    appVersion: DEFAULT_APP_VERSION
  });
  return response || {};
}

async function downloadFile(credentials = {}, oid, outputPath) {
  const normalized = normalizeCredentials(credentials);
  const response = await runBridge('download', {
    ...normalized,
    oid: String(oid || '').trim(),
    outputPath: String(outputPath || '').trim(),
    storageBase: DEFAULT_STORAGE_BASE,
    appVersion: DEFAULT_APP_VERSION
  });
  return response || {};
}

async function listFiles(credentials = {}, folder) {
  const normalized = normalizeCredentials(credentials);
  const response = await runBridge('list', {
    ...normalized,
    folder: String(folder || DEFAULT_STORAGE_BASE).trim(),
    storageBase: DEFAULT_STORAGE_BASE,
    appVersion: DEFAULT_APP_VERSION
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
