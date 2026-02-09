/**
 * Unit tests for file manager
 * Tests file upload, download, listing, and organization
 */

const fileManager = require('../lib/fileManager');
const fs = require('fs');
const path = require('path');
const os = require('os');

describe('File Manager', () => {
  let testDir;

  beforeEach(() => {
    // Create temporary test directory
    testDir = fs.mkdtempSync(path.join(os.tmpdir(), 'proton-test-'));
  });

  afterEach(() => {
    // Clean up test directory
    if (fs.existsSync(testDir)) {
      fs.rmSync(testDir, { recursive: true, force: true });
    }
  });

  describe('getProtonDrivePath', () => {
    test('generates correct hierarchical path for OID', () => {
      const oid = '00abc123def456';
      const result = fileManager.getProtonDrivePath(oid);

      expect(result).toBe('LFS/00/abc123def456');
    });

    test('handles various OIDs correctly', () => {
      const tests = [
        { oid: 'aabbcc', expected: 'LFS/aa/bbcc' },
        { oid: 'ff00ff', expected: 'LFS/ff/00ff' },
        { oid: '00000000', expected: 'LFS/00/000000' },
      ];

      tests.forEach(test => {
        const result = fileManager.getProtonDrivePath(test.oid);
        expect(result).toBe(test.expected);
      });
    });

    test('rejects invalid OID formats', () => {
      expect(() => fileManager.getProtonDrivePath('')).toThrow();
      expect(() => fileManager.getProtonDrivePath('a')).toThrow();
      expect(() => fileManager.getProtonDrivePath(null)).toThrow();
    });

    test('resolves local object path for OID', () => {
      const oid = '00abc123def456';
      const result = fileManager.getLocalObjectPath(oid);

      expect(result).toContain(path.join('00', 'abc123def456'));
    });
  });

  describe('uploadFile', () => {
    test('returns file metadata after upload', async () => {
      // Create test file
      const testFile = path.join(testDir, 'test.bin');
      const content = Buffer.from('test content');
      fs.writeFileSync(testFile, content);

      const result = await fileManager.uploadFile('token', 'abc123', testFile);

      expect(result).toBeDefined();
      expect(result.oid).toBe('abc123');
      expect(result.size).toBe(content.length);
      expect(result.location).toContain('LFS/');
    });

    test('rejects non-existent files', async () => {
      try {
        await fileManager.uploadFile('token', 'oid', '/non/existent/file');
        fail('Should have thrown error');
      } catch (error) {
        expect(error.message).toContain('File not found');
      }
    });

    test('calculates SHA-256 hash', async () => {
      const testFile = path.join(testDir, 'hashtest.bin');
      fs.writeFileSync(testFile, 'content');

      const result = await fileManager.uploadFile('token', 'oid', testFile);

      expect(result.hash).toBeDefined();
      expect(result.hash).toMatch(/^[a-f0-9]{64}$/); // SHA-256 in hex
    });

    test('includes file size in result', async () => {
      const testFile = path.join(testDir, 'sizetest.bin');
      const data = Buffer.alloc(2048); // 2KB file
      fs.writeFileSync(testFile, data);

      const result = await fileManager.uploadFile('token', 'oid', testFile);

      expect(result.size).toBe(2048);
    });
  });

  describe('downloadFile', () => {
    test('downloads previously uploaded object bytes', async () => {
      const sourcePath = path.join(testDir, 'source.bin');
      const payload = Buffer.from('persisted-roundtrip');
      fs.writeFileSync(sourcePath, payload);

      const oid = 'aa00bb11cc22';
      await fileManager.uploadFile('token', oid, sourcePath);

      const outputPath = path.join(testDir, 'downloaded-roundtrip.bin');
      await fileManager.downloadFile('token', oid, outputPath);

      expect(fs.existsSync(outputPath)).toBe(true);
      expect(fs.readFileSync(outputPath)).toEqual(payload);
    });

    test('creates file at output path', async () => {
      const outputPath = path.join(testDir, 'downloaded.bin');

      await fileManager.downloadFile('token', 'oid123', outputPath);

      expect(fs.existsSync(outputPath)).toBe(true);
    });

    test('returns file metadata after download', async () => {
      const outputPath = path.join(testDir, 'test-download.bin');

      const result = await fileManager.downloadFile('token', 'oid456', outputPath);

      expect(result).toBeDefined();
      expect(result.oid).toBe('oid456');
      expect(result.path).toBe(outputPath);
      expect(result.size).toBeGreaterThan(0);
    });

    test('creates directories if needed', async () => {
      const outputPath = path.join(testDir, 'nested/deep/dir/file.bin');

      await fileManager.downloadFile('token', 'oid', outputPath);

      expect(fs.existsSync(outputPath)).toBe(true);
    });

    test('includes SHA-256 hash in result', async () => {
      const outputPath = path.join(testDir, 'hashtest.bin');

      const result = await fileManager.downloadFile('token', 'oid', outputPath);

      expect(result.hash).toBeDefined();
      expect(result.hash).toMatch(/^[a-f0-9]{64}$/);
    });
  });

  describe('listFiles', () => {
    test('returns array of files', async () => {
      const result = await fileManager.listFiles('token', 'LFS');

      expect(Array.isArray(result)).toBe(true);
    });

    test('file objects have required properties', async () => {
      const result = await fileManager.listFiles('token', 'LFS');

      if (result.length > 0) {
        const file = result[0];
        expect(file.oid).toBeDefined();
        expect(file.name).toBeDefined();
        expect(file.size).toBeDefined();
        expect(file.modified).toBeDefined();
      }
    });

    test('uses default folder if not specified', async () => {
      const result = await fileManager.listFiles('token');

      expect(Array.isArray(result)).toBe(true);
    });
  });

  describe('fileExists', () => {
    test('checks file existence', async () => {
      const result = await fileManager.fileExists('token', 'oid123');

      // Should not throw
      expect(typeof result).toBe('boolean');
    });
  });

  describe('deleteFile', () => {
    test('returns deletion result', async () => {
      const result = await fileManager.deleteFile('token', 'oid123');

      expect(result).toBeDefined();
      expect(result.deleted).toBe(true);
    });

    test('includes OID in response', async () => {
      const result = await fileManager.deleteFile('token', 'testoid');

      expect(result.oid).toBe('testoid');
    });
  });

  describe('Error handling', () => {
    test('rejects upload without file path', async () => {
      try {
        await fileManager.uploadFile('token', 'oid', null);
        fail('Should have thrown');
      } catch (error) {
        expect(error).toBeDefined();
      }
    });

    test('rejects download without output path', async () => {
      try {
        await fileManager.downloadFile('token', 'oid', null);
        fail('Should have thrown');
      } catch (error) {
        expect(error).toBeDefined();
      }
    });
  });
});
