const fs = require('fs');
const path = require('path');

describe('subprocess rate limiting', () => {
  const originalEnv = { ...process.env };
  const mockBridgePath = path.join(__dirname, '..', 'testdata', 'mock-proton-drive-cli.js');

  beforeEach(() => {
    jest.resetModules();
    process.env = { ...originalEnv };
    fs.chmodSync(mockBridgePath, 0o755);
    process.env.PROTON_DRIVE_CLI_BIN = mockBridgePath;
    process.env.LFS_STORAGE_BASE = 'LFS';
  });

  afterEach(() => {
    process.env = { ...originalEnv };
  });

  test('handles concurrent operations within pool limit', async () => {
    const bridge = require('../../lib/protonDriveBridge');

    // Launch 5 concurrent auth operations (within pool limit of 10)
    const promises = Array.from({ length: 5 }, () =>
      bridge.authenticate({
        username: 'user@proton.me',
        password: 'secret'
      })
    );

    const results = await Promise.allSettled(promises);
    const fulfilled = results.filter(r => r.status === 'fulfilled');
    expect(fulfilled.length).toBe(5);
  });

  test('handles subprocess timeout', async () => {
    process.env.MOCK_BRIDGE_HANG = '1';
    process.env.PROTON_DRIVE_CLI_TIMEOUT_MS = '500'; // 500ms timeout
    const bridge = require('../../lib/protonDriveBridge');

    await expect(
      bridge.authenticate({
        username: 'user@proton.me',
        password: 'secret'
      })
    ).rejects.toMatchObject({
      name: 'BridgeError',
      code: 504,
      message: expect.stringContaining('timed out')
    });
  }, 10000);

  test('handles subprocess crash', async () => {
    process.env.MOCK_BRIDGE_CRASH = '1';
    const bridge = require('../../lib/protonDriveBridge');

    await expect(
      bridge.authenticate({
        username: 'user@proton.me',
        password: 'secret'
      })
    ).rejects.toMatchObject({
      name: 'BridgeError',
    });
  });
});
