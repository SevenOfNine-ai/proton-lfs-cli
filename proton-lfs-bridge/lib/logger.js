/**
 * Logger utility for SDK service
 * Provides consistent logging across the service
 */

const fs = require('fs');
const path = require('path');
const config = require('./config');

const LOG_LEVEL = config.LOG_LEVEL;
const LOG_FILE = config.LOG_FILE;

const LEVELS = {
  error: 0,
  warn: 1,
  info: 2,
  debug: 3
};

const LEVEL_NAMES = {
  0: 'ERROR',
  1: 'WARN ',
  2: 'INFO ',
  3: 'DEBUG'
};

const currentLevel = LEVELS[LOG_LEVEL] || LEVELS.info;

/**
 * Format log message
 */
function formatMessage(level, message) {
  const timestamp = new Date().toISOString();
  const levelName = LEVEL_NAMES[level];
  return `[${timestamp}] ${levelName} - ${message}`;
}

/**
 * Write log to file (if configured)
 */
function writeToFile(message) {
  if (!LOG_FILE) return;

  try {
    const logDir = path.dirname(LOG_FILE);
    if (!fs.existsSync(logDir)) {
      fs.mkdirSync(logDir, { recursive: true });
    }
    fs.appendFileSync(LOG_FILE, message + '\n');
  } catch (error) {
    console.error('Failed to write log file:', error);
  }
}

/**
 * Log at specified level
 */
function log(level, message) {
  if (level > currentLevel) return;

  const formatted = formatMessage(level, message);
  
  // Always log to stderr for async safety
  process.stderr.write(formatted + '\n');
  
  // Also write to file if configured
  writeToFile(formatted);
}

module.exports = {
  error: (message) => log(LEVELS.error, message),
  warn: (message) => log(LEVELS.warn, message),
  info: (message) => log(LEVELS.info, message),
  debug: (message) => log(LEVELS.debug, message)
};
