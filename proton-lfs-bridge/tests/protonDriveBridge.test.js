const fs = require('fs');
const path = require('path');

describe('protonDriveBridge', () => {
  const originalEnv = { ...process.env };
  const mockBridgePath = path.join(__dirname, 'testdata', 'mock-proton-drive-cli.js');
  const mockNonJSONFailureBridgePath = path.join(__dirname, 'testdata', 'mock-proton-bridge-nonjson-fail.sh');

  beforeEach(() => {
    jest.resetModules();
    process.env = { ...originalEnv };
    fs.chmodSync(mockBridgePath, 0o755);
    fs.chmodSync(mockNonJSONFailureBridgePath, 0o755);
    process.env.PROTON_DRIVE_CLI_BIN = mockBridgePath;
    process.env.LFS_STORAGE_BASE = 'LFS';
    process.env.PROTON_APP_VERSION = 'external-drive-protonlfs@test';
  });

  afterEach(() => {
    process.env = { ...originalEnv };
  });

  test('authenticates successfully via bridge command', async () => {
    const bridge = require('../lib/protonDriveBridge');

    await expect(
      bridge.authenticate({
        username: 'user@proton.me',
        password: 'secret'
      })
    ).resolves.toBeUndefined();
  });

  test('maps bridge auth failures to BridgeError', async () => {
    const bridge = require('../lib/protonDriveBridge');

    await expect(
      bridge.authenticate({
        username: 'bad-user',
        password: 'wrong'
      })
    ).rejects.toMatchObject({
      name: 'BridgeError',
      code: 401
    });
  });

  test('parses bridge payload when stdout includes non-JSON preamble', async () => {
    process.env.MOCK_BRIDGE_STDOUT_NOISE = '1';
    const bridge = require('../lib/protonDriveBridge');

    await expect(
      bridge.authenticate({
        username: 'user@proton.me',
        password: 'secret'
      })
    ).resolves.toBeUndefined();
  });

  test('preserves structured bridge errors when stdout includes non-JSON preamble', async () => {
    process.env.MOCK_BRIDGE_STDOUT_NOISE = '1';
    const bridge = require('../lib/protonDriveBridge');

    await expect(
      bridge.authenticate({
        username: 'bad-user',
        password: 'wrong'
      })
    ).rejects.toMatchObject({
      name: 'BridgeError',
      code: 401
    });
  });

  test('reports non-JSON bridge failures with stderr details', async () => {
    // Use old-style mock that writes non-JSON and exits 1
    process.env.PROTON_DRIVE_CLI_BIN = mockNonJSONFailureBridgePath;
    const bridge = require('../lib/protonDriveBridge');

    await expect(
      bridge.authenticate({
        username: 'user@proton.me',
        password: 'secret'
      })
    ).rejects.toMatchObject({
      name: 'BridgeError',
      code: 500,
      message: expect.stringContaining('proton bridge command failed'),
      details: expect.stringContaining('mock bridge failed before JSON output')
    });
  });

  test('returns upload payload fields', async () => {
    const bridge = require('../lib/protonDriveBridge');
    const response = await bridge.uploadFile(
      {
        username: 'user@proton.me',
        password: 'secret'
      },
      'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
      '/tmp/input.bin'
    );

    expect(response.oid).toBe('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa');
    expect(response.size).toBe(123);
    expect(response.location).toContain('LFS/aa/');
  });

  test('returns list array payload', async () => {
    const bridge = require('../lib/protonDriveBridge');
    const files = await bridge.listFiles({
      username: 'user@proton.me',
      password: 'secret'
    }, 'LFS');

    expect(Array.isArray(files)).toBe(true);
    expect(files[0].oid).toBe('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa');
  });

  test('validates OID format before spawning subprocess', async () => {
    const bridge = require('../lib/protonDriveBridge');

    await expect(
      bridge.uploadFile(
        { username: 'user@proton.me', password: 'secret' },
        'short',
        '/tmp/input.bin'
      )
    ).rejects.toMatchObject({
      name: 'BridgeError',
      code: 400,
      message: expect.stringContaining('Invalid OID')
    });
  });

  test('rejects path traversal in file paths', async () => {
    const bridge = require('../lib/protonDriveBridge');

    await expect(
      bridge.uploadFile(
        { username: 'user@proton.me', password: 'secret' },
        'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
        '../../etc/passwd'
      )
    ).rejects.toMatchObject({
      name: 'BridgeError',
      code: 400,
      message: expect.stringContaining('Path traversal')
    });
  });

  test('rejects OID with shell metacharacters', async () => {
    const bridge = require('../lib/protonDriveBridge');

    await expect(
      bridge.uploadFile(
        { username: 'user@proton.me', password: 'secret' },
        '; rm -rf / ; aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
        '/tmp/input.bin'
      )
    ).rejects.toMatchObject({
      name: 'BridgeError',
      code: 400,
      message: expect.stringContaining('Invalid OID')
    });
  });

  test('rejects empty credentials', async () => {
    const bridge = require('../lib/protonDriveBridge');

    await expect(
      bridge.authenticate({})
    ).rejects.toMatchObject({
      name: 'BridgeError',
      code: 400
    });
  });

  test('downloads file via bridge command', async () => {
    const bridge = require('../lib/protonDriveBridge');
    const response = await bridge.downloadFile(
      { username: 'user@proton.me', password: 'secret' },
      'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
      '/tmp/output.bin'
    );

    expect(response.oid).toBe('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa');
    expect(response.downloaded).toBe(true);
  });
});
