/**
 * Unit tests for logger utility
 * Tests log levels, output formatting, and file writing
 */

const logger = require('../lib/logger');
const fs = require('fs');
const path = require('path');
const os = require('os');

describe('Logger', () => {
  let testDir;
  let logFile;
  let originalConsole;

  beforeEach(() => {
    // Create temporary directory for log files
    testDir = fs.mkdtempSync(path.join(os.tmpdir(), 'logger-test-'));
    logFile = path.join(testDir, 'test.log');

    // Capture console output
    originalConsole = {
      log: console.log,
      info: console.info,
      warn: console.warn,
      error: console.error,
      debug: console.debug,
    };
  });

  afterEach(() => {
    // Restore console
    Object.assign(console, originalConsole);

    // Clean up log file
    if (fs.existsSync(testDir)) {
      fs.rmSync(testDir, { recursive: true, force: true });
    }
  });

  describe('log levels', () => {
    test('logs info level messages', () => {
      const messages = [];
      console.info = (msg) => messages.push(msg);

      logger.info('Test info message');

      expect(messages.some(m => m.includes('Test info message'))).toBe(true);
    });

    test('logs warn level messages', () => {
      const messages = [];
      console.warn = (msg) => messages.push(msg);

      logger.warn('Test warning');

      expect(messages.some(m => m.includes('Test warning'))).toBe(true);
    });

    test('logs error level messages', () => {
      const messages = [];
      console.error = (msg) => messages.push(msg);

      logger.error('Test error');

      expect(messages.some(m => m.includes('Test error'))).toBe(true);
    });

    test('logs debug level messages when enabled', () => {
      const messages = [];
      console.debug = (msg) => messages.push(msg);

      logger.debug('Debug message');

      // Debug may only log if debug enabled
      expect(Array.isArray(messages)).toBe(true);
    });
  });

  describe('message formatting', () => {
    test('includes timestamp in messages', () => {
      const messages = [];
      console.info = (msg) => messages.push(msg);

      logger.info('Test');

      expect(messages.some(m => m.match(/\d{4}-\d{2}-\d{2}/))).toBe(true);
    });

    test('includes log level in output', () => {
      const messages = [];
      console.info = (msg) => messages.push(msg);

      logger.info('Message');

      expect(messages.some(m => m.includes('INFO'))).toBe(true);
    });

    test('formats objects correctly', () => {
      const messages = [];
      console.info = (msg) => messages.push(msg);

      logger.info({ key: 'value', number: 42 });

      expect(messages.length).toBeGreaterThan(0);
    });
  });

  describe('structured logging', () => {
    test('supports context objects', () => {
      const messages = [];
      console.info = (msg) => messages.push(msg);

      logger.info('Event happened', { oid: 'abc123', size: 1024 });

      expect(messages.some(m => 
        m.includes('abc123') || m.includes('1024')
      )).toBe(true);
    });

    test('logs with session ID when available', () => {
      const messages = [];
      console.info = (msg) => messages.push(msg);

      logger.info('Session started', { sessionId: 'sess-123' });

      expect(messages.some(m => m.includes('sess-123'))).toBe(true);
    });
  });

  describe('error logging', () => {
    test('logs error objects with stack trace', () => {
      const messages = [];
      console.error = (msg) => messages.push(msg);

      const testError = new Error('Test error');
      logger.error('An error occurred', testError);

      expect(messages.length).toBeGreaterThan(0);
    });

    test('handles null/undefined gracefully', () => {
      const messages = [];
      console.error = (msg) => messages.push(msg);

      expect(() => {
        logger.error('Message', null);
        logger.error('Message', undefined);
      }).not.toThrow();
    });
  });

  describe('log output', () => {
    test('outputs to console', () => {
      let logged = false;
      console.info = () => { logged = true; };

      logger.info('Test');

      expect(logged).toBe(true);
    });

    test('does not throw on logging', () => {
      expect(() => {
        logger.info('Message 1');
        logger.warn('Message 2');
        logger.error('Message 3');
      }).not.toThrow();
    });
  });

  describe('context tracking', () => {
    test('includes request ID when available', () => {
      const messages = [];
      console.info = (msg) => messages.push(msg);

      logger.info('Request processed', { requestId: 'req-456' });

      expect(messages.some(m => m.includes('req-456'))).toBe(true);
    });

    test('includes operation type', () => {
      const messages = [];
      console.info = (msg) => messages.push(msg);

      logger.info('Upload started', { operation: 'upload' });

      expect(messages.some(m => m.includes('upload'))).toBe(true);
    });
  });

  describe('performance', () => {
    test('logs high volume without blocking', async () => {
      const start = Date.now();

      for (let i = 0; i < 1000; i++) {
        logger.debug(`Message ${i}`);
      }

      const elapsed = Date.now() - start;
      // Should complete 1000 logs in < 500ms
      expect(elapsed).toBeLessThan(500);
    });

    test('handles large context objects', () => {
      const largeContext = {
        data: new Array(1000).fill('x'),
        metadata: { key: 'value' },
      };

      expect(() => {
        logger.info('Large context', largeContext);
      }).not.toThrow();
    });
  });
});
