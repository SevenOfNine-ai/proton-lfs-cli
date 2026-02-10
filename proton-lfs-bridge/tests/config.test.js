describe('config module', () => {
  const originalEnv = { ...process.env };

  beforeEach(() => {
    jest.resetModules();
  });

  afterEach(() => {
    process.env = { ...originalEnv };
  });

  test('defaults when env vars are unset', () => {
    delete process.env.LFS_BRIDGE_PORT;
    delete process.env.SDK_BACKEND_MODE;
    delete process.env.LFS_STORAGE_BASE;
    delete process.env.PROTON_APP_VERSION;
    delete process.env.PROTON_DRIVE_CLI_BIN;
    delete process.env.PROTON_DRIVE_CLI_TIMEOUT_MS;
    delete process.env.LOG_LEVEL;
    delete process.env.LOG_FILE;
    delete process.env.MAX_CONCURRENT_SUBPROCESSES;

    const config = require('../lib/config');

    expect(config.LFS_BRIDGE_PORT).toBe(3000);
    expect(config.effectiveBackendMode).toBe('local');
    expect(config.LFS_STORAGE_BASE).toBe('LFS');
    expect(config.PROTON_APP_VERSION).toBe('external-drive-protonlfs@dev');
    expect(config.PROTON_DRIVE_CLI_TIMEOUT_MS).toBe(300000);
    expect(config.MAX_CONCURRENT_SUBPROCESSES).toBe(10);
    expect(config.LOG_LEVEL).toBe('info');
    expect(config.LOG_FILE).toBeNull();
    expect(config.isRealBackendMode()).toBe(false);
  });

  test('effectiveBackendMode normalizes "real" to "proton-drive-cli"', () => {
    process.env.SDK_BACKEND_MODE = 'real';
    const config = require('../lib/config');

    expect(config.effectiveBackendMode).toBe('proton-drive-cli');
    expect(config.isRealBackendMode()).toBe(true);
  });

  test('effectiveBackendMode accepts "proton-drive-cli" directly', () => {
    process.env.SDK_BACKEND_MODE = 'proton-drive-cli';
    const config = require('../lib/config');

    expect(config.effectiveBackendMode).toBe('proton-drive-cli');
    expect(config.isRealBackendMode()).toBe(true);
  });

  test('custom values from env', () => {
    process.env.LFS_BRIDGE_PORT = '4000';
    process.env.LFS_STORAGE_BASE = 'CustomLFS';
    process.env.PROTON_APP_VERSION = 'test@1.0.0';
    process.env.PROTON_DRIVE_CLI_TIMEOUT_MS = '60000';
    process.env.LOG_LEVEL = 'debug';
    process.env.LOG_FILE = '/tmp/test.log';
    process.env.MAX_CONCURRENT_SUBPROCESSES = '5';

    const config = require('../lib/config');

    expect(config.LFS_BRIDGE_PORT).toBe(4000);
    expect(config.LFS_STORAGE_BASE).toBe('CustomLFS');
    expect(config.PROTON_APP_VERSION).toBe('test@1.0.0');
    expect(config.PROTON_DRIVE_CLI_TIMEOUT_MS).toBe(60000);
    expect(config.LOG_LEVEL).toBe('debug');
    expect(config.LOG_FILE).toBe('/tmp/test.log');
    expect(config.MAX_CONCURRENT_SUBPROCESSES).toBe(5);
  });

  test('invalid numeric env falls back to default', () => {
    process.env.LFS_BRIDGE_PORT = 'not-a-number';
    process.env.PROTON_DRIVE_CLI_TIMEOUT_MS = '-1';

    const config = require('../lib/config');

    expect(config.LFS_BRIDGE_PORT).toBe(3000);
    expect(config.PROTON_DRIVE_CLI_TIMEOUT_MS).toBe(300000);
  });
});
