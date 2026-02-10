const fs = require('fs');
const path = require('path');

describe('command injection prevention', () => {
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

  const maliciousOids = [
    '; rm -rf / ;',
    '$(whoami)',
    '`id`',
    '| cat /etc/passwd',
    '&& curl evil.com',
    '../../../etc/passwd',
    'aaaa\x00bbbb',
    'a'.repeat(63), // Too short
    'a'.repeat(65), // Too long
    'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAGG', // Non-hex chars
    '',
    null,
    undefined,
  ];

  test.each(maliciousOids)('rejects malicious OID: %s', async (oid) => {
    const bridge = require('../../lib/protonDriveBridge');

    await expect(
      bridge.uploadFile(
        { username: 'user@proton.me', password: 'secret' },
        oid,
        '/tmp/input.bin'
      )
    ).rejects.toMatchObject({
      name: 'BridgeError',
      code: 400,
    });
  });

  const maliciousPaths = [
    '../../etc/passwd',
    '../../../etc/shadow',
    'foo/../../etc/passwd',
  ];

  test.each(maliciousPaths)('rejects path traversal: %s', async (filePath) => {
    const bridge = require('../../lib/protonDriveBridge');

    await expect(
      bridge.uploadFile(
        { username: 'user@proton.me', password: 'secret' },
        'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
        filePath
      )
    ).rejects.toMatchObject({
      name: 'BridgeError',
      code: 400,
    });
  });

  test('spawns subprocess with array args (no shell)', async () => {
    // This test verifies that spawn() is used with array args
    // rather than a shell string (which would be vulnerable to injection).
    // The mock bridge validates it receives the correct arguments.
    const bridge = require('../../lib/protonDriveBridge');

    const response = await bridge.uploadFile(
      { username: 'user@proton.me', password: 'secret' },
      'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
      '/tmp/safe-file.bin'
    );

    expect(response.oid).toBe('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa');
  });

  test('credentials are passed via stdin not command-line args', async () => {
    // The mock bridge reads from stdin, proving credentials
    // are not passed via argv (which would be visible in ps output)
    const bridge = require('../../lib/protonDriveBridge');

    await expect(
      bridge.authenticate({
        username: 'user@proton.me',
        password: 'super-secret-password'
      })
    ).resolves.toBeUndefined();
  });

  test('rejects download with path traversal in output path', async () => {
    const bridge = require('../../lib/protonDriveBridge');

    await expect(
      bridge.downloadFile(
        { username: 'user@proton.me', password: 'secret' },
        'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
        '../../etc/shadow'
      )
    ).rejects.toMatchObject({
      name: 'BridgeError',
      code: 400,
    });
  });
});
