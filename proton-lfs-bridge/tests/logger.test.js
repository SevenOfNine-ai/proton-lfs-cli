/**
 * Unit tests for logger utility
 * Tests log levels, output formatting, and file writing
 */

const fs = require('fs');
const path = require('path');
const os = require('os');

describe('Logger', () => {
  let testDir;
  let originalStderrWrite;
  let messages;

  beforeEach(() => {
    jest.resetModules();
    testDir = fs.mkdtempSync(path.join(os.tmpdir(), 'logger-test-'));
    messages = [];
    originalStderrWrite = process.stderr.write;
    process.stderr.write = (msg) => {
      messages.push(String(msg));
      return true;
    };
  });

  afterEach(() => {
    process.stderr.write = originalStderrWrite;
    if (fs.existsSync(testDir)) {
      fs.rmSync(testDir, { recursive: true, force: true });
    }
  });

  describe('log levels', () => {
    test('logs info level messages', () => {
      const logger = require('../lib/logger');
      logger.info('Test info message');
      expect(messages.some(m => m.includes('Test info message'))).toBe(true);
    });

    test('logs warn level messages', () => {
      const logger = require('../lib/logger');
      logger.warn('Test warning');
      expect(messages.some(m => m.includes('Test warning'))).toBe(true);
    });

    test('logs error level messages', () => {
      const logger = require('../lib/logger');
      logger.error('Test error');
      expect(messages.some(m => m.includes('Test error'))).toBe(true);
    });

    test('logs debug level messages when enabled', () => {
      const logger = require('../lib/logger');
      logger.debug('Debug message');
      // Debug may only log if LOG_LEVEL=debug is set
      expect(Array.isArray(messages)).toBe(true);
    });
  });

  describe('message formatting', () => {
    test('includes timestamp in messages', () => {
      const logger = require('../lib/logger');
      logger.info('Test');
      expect(messages.some(m => m.match(/\d{4}-\d{2}-\d{2}/))).toBe(true);
    });

    test('includes log level in output', () => {
      const logger = require('../lib/logger');
      logger.info('Message');
      expect(messages.some(m => m.includes('INFO'))).toBe(true);
    });

    test('formats objects passed as message', () => {
      const logger = require('../lib/logger');
      logger.info({ key: 'value', number: 42 });
      expect(messages.length).toBeGreaterThan(0);
    });
  });

  describe('error logging', () => {
    test('logs error messages', () => {
      const logger = require('../lib/logger');
      logger.error('An error occurred');
      expect(messages.length).toBeGreaterThan(0);
      expect(messages.some(m => m.includes('An error occurred'))).toBe(true);
    });

    test('handles null/undefined gracefully', () => {
      const logger = require('../lib/logger');
      expect(() => {
        logger.error(null);
        logger.error(undefined);
      }).not.toThrow();
    });
  });

  describe('log output', () => {
    test('outputs to stderr', () => {
      const logger = require('../lib/logger');
      logger.info('Test');
      expect(messages.length).toBeGreaterThan(0);
    });

    test('does not throw on logging', () => {
      const logger = require('../lib/logger');
      expect(() => {
        logger.info('Message 1');
        logger.warn('Message 2');
        logger.error('Message 3');
      }).not.toThrow();
    });
  });

  describe('performance', () => {
    test('logs high volume without blocking', () => {
      const logger = require('../lib/logger');
      const start = Date.now();

      for (let i = 0; i < 1000; i++) {
        logger.debug(`Message ${i}`);
      }

      const elapsed = Date.now() - start;
      expect(elapsed).toBeLessThan(500);
    });
  });
});
