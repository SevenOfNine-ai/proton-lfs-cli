/**
 * Central configuration module for the LFS bridge service.
 *
 * All environment variable reads are consolidated here so that
 * consumers import config values instead of reading process.env
 * directly. This module must NOT require logger.js (avoids
 * circular dependency — logger.js imports config.js).
 */

const path = require('path');

function trimmedEnv(name) {
  return String(process.env[name] || '').trim();
}

function parseIntEnv(name, fallback) {
  const raw = trimmedEnv(name);
  if (!raw) return fallback;
  const parsed = Number.parseInt(raw, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

// --- Server ---
const LFS_BRIDGE_PORT = parseIntEnv('LFS_BRIDGE_PORT', 3000);

// --- Backend mode ---
const BACKEND_MODE_LOCAL = 'local';
const BACKEND_MODE_REAL = 'real'; // legacy alias
const BACKEND_MODE_PROTON_DRIVE_CLI = 'proton-drive-cli';

const rawBackendMode = String(process.env.SDK_BACKEND_MODE || 'local').trim().toLowerCase();
const effectiveBackendMode = (rawBackendMode === BACKEND_MODE_REAL || rawBackendMode === BACKEND_MODE_PROTON_DRIVE_CLI)
  ? BACKEND_MODE_PROTON_DRIVE_CLI
  : BACKEND_MODE_LOCAL;

function isRealBackendMode() {
  return effectiveBackendMode === BACKEND_MODE_PROTON_DRIVE_CLI;
}

// --- Storage ---
const LFS_STORAGE_BASE = trimmedEnv('LFS_STORAGE_BASE') || 'LFS';
const TEMP_DIR = trimmedEnv('TEMP_DIR') || '/tmp';
const SDK_STORAGE_DIR = trimmedEnv('SDK_STORAGE_DIR') || path.join(TEMP_DIR, 'proton-git-lfs-sdk-storage');

// --- Proton Drive CLI bridge ---
const PROTON_APP_VERSION = trimmedEnv('PROTON_APP_VERSION') || 'external-drive-protonlfs@dev';

const DEFAULT_DRIVE_CLI_BIN = path.resolve(
  __dirname,
  '..',
  '..',
  'submodules',
  'proton-drive-cli',
  'dist',
  'index.js'
);
const PROTON_DRIVE_CLI_BIN = trimmedEnv('PROTON_DRIVE_CLI_BIN') || DEFAULT_DRIVE_CLI_BIN;
const PROTON_DRIVE_CLI_TIMEOUT_MS = parseIntEnv('PROTON_DRIVE_CLI_TIMEOUT_MS', 5 * 60 * 1000);
const MAX_CONCURRENT_SUBPROCESSES = parseIntEnv('MAX_CONCURRENT_SUBPROCESSES', 10);

// --- Additional auth factors (stay as env-sourced) ---
const PROTON_DATA_PASSWORD = trimmedEnv('PROTON_DATA_PASSWORD');
const PROTON_SECOND_FACTOR_CODE = trimmedEnv('PROTON_SECOND_FACTOR_CODE');

// --- Logging ---
const LOG_LEVEL = trimmedEnv('LOG_LEVEL') || 'info';
const LOG_FILE = trimmedEnv('LOG_FILE') || null;

module.exports = {
  LFS_BRIDGE_PORT,
  BACKEND_MODE_LOCAL,
  BACKEND_MODE_REAL,
  BACKEND_MODE_PROTON_DRIVE_CLI,
  rawBackendMode,
  effectiveBackendMode,
  isRealBackendMode,
  LFS_STORAGE_BASE,
  TEMP_DIR,
  SDK_STORAGE_DIR,
  PROTON_APP_VERSION,
  PROTON_DRIVE_CLI_BIN,
  PROTON_DRIVE_CLI_TIMEOUT_MS,
  MAX_CONCURRENT_SUBPROCESSES,
  PROTON_DATA_PASSWORD,
  PROTON_SECOND_FACTOR_CODE,
  LOG_LEVEL,
  LOG_FILE,
};
