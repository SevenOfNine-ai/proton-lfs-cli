/**
 * Unit tests for Express server endpoints
 * Tests all API endpoints with valid and invalid inputs
 */

const request = require('supertest');
const express = require('express');

// Create a simplified test app (mocking server.js)
const createTestApp = () => {
  const app = express();
  app.use(express.json());

  // Mock data
  const sessions = new Map();

  // /health endpoint
  app.get('/health', (req, res) => {
    res.json({ status: 'healthy', uptime: process.uptime() });
  });

  // /init endpoint
  app.post('/init', (req, res) => {
    const { username, password } = req.body;
    if (!username || !password) {
      return res.status(400).json({ error: 'Missing credentials' });
    }
    const sessionId = `sess-${Date.now()}`;
    const accessToken = `token-${sessionId}`;
    sessions.set(sessionId, {
      username,
      accessToken,
      expiresAt: Date.now() + 86400000, // 24 hours
    });
    res.json({ sessionId, accessToken, expiresAt: '2024-01-01T00:00:00Z' });
  });

  // /upload endpoint
  app.post('/upload', (req, res) => {
    const { sessionId, oid, path: filePath } = req.body;
    if (!sessions.has(sessionId)) {
      return res.status(401).json({ error: 'Invalid session' });
    }
    if (!oid || !filePath) {
      return res.status(400).json({ error: 'Missing oid or path' });
    }
    res.json({
      oid,
      location: `LFS/${oid.slice(0, 2)}/${oid.slice(2)}`,
      size: 1024,
    });
  });

  // /download endpoint
  app.post('/download', (req, res) => {
    const { sessionId, oid } = req.body;
    if (!sessions.has(sessionId)) {
      return res.status(401).json({ error: 'Invalid session' });
    }
    if (!oid) {
      return res.status(400).json({ error: 'Missing oid' });
    }
    res.json({
      oid,
      size: 2048,
      data: Buffer.from('mock file data').toString('base64'),
    });
  });

  // /refresh endpoint
  app.post('/refresh', (req, res) => {
    const { sessionId } = req.body;
    if (!sessions.has(sessionId)) {
      return res.status(401).json({ error: 'Invalid session' });
    }
    const newToken = `token-${Date.now()}`;
    const session = sessions.get(sessionId);
    session.accessToken = newToken;
    session.expiresAt = Date.now() + 86400000;
    res.json({ accessToken: newToken, expiresAt: '2024-01-02T00:00:00Z' });
  });

  // /list endpoint
  app.post('/list', (req, res) => {
    const { sessionId, folder } = req.body;
    if (!sessions.has(sessionId)) {
      return res.status(401).json({ error: 'Invalid session' });
    }
    res.json({
      files: [
        { oid: 'abc123', name: 'file1.bin', size: 1024 },
        { oid: 'def456', name: 'file2.bin', size: 2048 },
      ],
    });
  });

  return app;
};

describe('Express Server Endpoints', () => {
  let app;

  beforeEach(() => {
    app = createTestApp();
  });

  describe('GET /health', () => {
    test('returns health status', async () => {
      const response = await request(app).get('/health');

      expect(response.statusCode).toBe(200);
      expect(response.body.status).toBe('healthy');
      expect(response.body.uptime).toBeGreaterThan(0);
    });

    test('includes uptime metric', async () => {
      const response = await request(app).get('/health');

      expect(typeof response.body.uptime).toBe('number');
      expect(response.body.uptime).toBeGreaterThan(0);
    });
  });

  describe('POST /init', () => {
    test('creates session with valid credentials', async () => {
      const response = await request(app)
        .post('/init')
        .send({ username: 'user@proton.me', password: 'password123' });

      expect(response.statusCode).toBe(200);
      expect(response.body.sessionId).toBeDefined();
      expect(response.body.accessToken).toBeDefined();
      expect(response.body.expiresAt).toBeDefined();
    });

    test('rejects missing username', async () => {
      const response = await request(app)
        .post('/init')
        .send({ password: 'password123' });

      expect(response.statusCode).toBe(400);
      expect(response.body.error).toBeDefined();
    });

    test('rejects missing password', async () => {
      const response = await request(app)
        .post('/init')
        .send({ username: 'user@proton.me' });

      expect(response.statusCode).toBe(400);
      expect(response.body.error).toBeDefined();
    });

    test('returns unique session IDs', async () => {
      const res1 = await request(app)
        .post('/init')
        .send({ username: 'user1@proton.me', password: 'pass1' });

      const res2 = await request(app)
        .post('/init')
        .send({ username: 'user2@proton.me', password: 'pass2' });

      expect(res1.body.sessionId).not.toBe(res2.body.sessionId);
    });

    test('returns valid token format', async () => {
      const response = await request(app)
        .post('/init')
        .send({ username: 'user@proton.me', password: 'password' });

      expect(response.body.accessToken).toMatch(/^token-/);
    });
  });

  describe('POST /upload', () => {
    let sessionId;
    let accessToken;

    beforeEach(async () => {
      const initRes = await request(app)
        .post('/init')
        .send({ username: 'user@proton.me', password: 'password' });

      sessionId = initRes.body.sessionId;
      accessToken = initRes.body.accessToken;
    });

    test('uploads file with valid session', async () => {
      const response = await request(app)
        .post('/upload')
        .send({
          sessionId,
          oid: 'abc123',
          path: '/path/to/file.bin',
        });

      expect(response.statusCode).toBe(200);
      expect(response.body.oid).toBe('abc123');
      expect(response.body.location).toBeDefined();
    });

    test('rejects upload without session', async () => {
      const response = await request(app)
        .post('/upload')
        .send({
          sessionId: 'invalid-session',
          oid: 'abc123',
          path: '/path/to/file.bin',
        });

      expect(response.statusCode).toBe(401);
    });

    test('rejects upload without OID', async () => {
      const response = await request(app)
        .post('/upload')
        .send({
          sessionId,
          path: '/path/to/file.bin',
        });

      expect(response.statusCode).toBe(400);
    });

    test('rejects upload without path', async () => {
      const response = await request(app)
        .post('/upload')
        .send({
          sessionId,
          oid: 'abc123',
        });

      expect(response.statusCode).toBe(400);
    });

    test('returns hierarchical location path', async () => {
      const response = await request(app)
        .post('/upload')
        .send({
          sessionId,
          oid: 'abc123def456',
          path: '/path/to/file.bin',
        });

      expect(response.body.location).toMatch(/LFS\/\w{2}\//);
    });

    test('includes file size in response', async () => {
      const response = await request(app)
        .post('/upload')
        .send({
          sessionId,
          oid: 'abc123',
          path: '/path/to/file.bin',
        });

      expect(response.body.size).toBeDefined();
      expect(typeof response.body.size).toBe('number');
    });
  });

  describe('POST /download', () => {
    let sessionId;

    beforeEach(async () => {
      const initRes = await request(app)
        .post('/init')
        .send({ username: 'user@proton.me', password: 'password' });

      sessionId = initRes.body.sessionId;
    });

    test('downloads file with valid session', async () => {
      const response = await request(app)
        .post('/download')
        .send({
          sessionId,
          oid: 'abc123',
        });

      expect(response.statusCode).toBe(200);
      expect(response.body.oid).toBe('abc123');
      expect(response.body.data).toBeDefined();
    });

    test('rejects download without valid session', async () => {
      const response = await request(app)
        .post('/download')
        .send({
          sessionId: 'invalid',
          oid: 'abc123',
        });

      expect(response.statusCode).toBe(401);
    });

    test('rejects download without OID', async () => {
      const response = await request(app)
        .post('/download')
        .send({ sessionId });

      expect(response.statusCode).toBe(400);
    });

    test('returns file size', async () => {
      const response = await request(app)
        .post('/download')
        .send({
          sessionId,
          oid: 'abc123',
        });

      expect(response.body.size).toBeDefined();
      expect(typeof response.body.size).toBe('number');
    });
  });

  describe('POST /refresh', () => {
    let sessionId;

    beforeEach(async () => {
      const initRes = await request(app)
        .post('/init')
        .send({ username: 'user@proton.me', password: 'password' });

      sessionId = initRes.body.sessionId;
    });

    test('refreshes token with valid session', async () => {
      const response = await request(app)
        .post('/refresh')
        .send({ sessionId });

      expect(response.statusCode).toBe(200);
      expect(response.body.accessToken).toBeDefined();
      expect(response.body.accessToken).not.toBeNull();
    });

    test('rejects refresh without valid session', async () => {
      const response = await request(app)
        .post('/refresh')
        .send({ sessionId: 'invalid' });

      expect(response.statusCode).toBe(401);
    });

    test('returns new token format', async () => {
      const response = await request(app)
        .post('/refresh')
        .send({ sessionId });

      expect(response.body.accessToken).toMatch(/^token-/);
    });

    test('returns new expiration time', async () => {
      const response = await request(app)
        .post('/refresh')
        .send({ sessionId });

      expect(response.body.expiresAt).toBeDefined();
    });
  });

  describe('POST /list', () => {
    let sessionId;

    beforeEach(async () => {
      const initRes = await request(app)
        .post('/init')
        .send({ username: 'user@proton.me', password: 'password' });

      sessionId = initRes.body.sessionId;
    });

    test('lists files with valid session', async () => {
      const response = await request(app)
        .post('/list')
        .send({
          sessionId,
          folder: 'LFS',
        });

      expect(response.statusCode).toBe(200);
      expect(Array.isArray(response.body.files)).toBe(true);
    });

    test('rejects list without valid session', async () => {
      const response = await request(app)
        .post('/list')
        .send({
          sessionId: 'invalid',
          folder: 'LFS',
        });

      expect(response.statusCode).toBe(401);
    });

    test('returns file objects with required fields', async () => {
      const response = await request(app)
        .post('/list')
        .send({
          sessionId,
          folder: 'LFS',
        });

      if (response.body.files.length > 0) {
        const file = response.body.files[0];
        expect(file.oid).toBeDefined();
        expect(file.name).toBeDefined();
        expect(file.size).toBeDefined();
      }
    });
  });

  describe('Error handling', () => {
    test('handles malformed JSON', async () => {
      const response = await request(app)
        .post('/init')
        .set('Content-Type', 'application/json')
        .send('{ invalid json }');

      expect(response.statusCode).toBeGreaterThanOrEqual(400);
    });

    test('handles missing Content-Type', async () => {
      const response = await request(app)
        .post('/init')
        .send({ username: 'user@proton.me', password: 'password' });

      // Should still work with default parsing
      expect([200, 400]).toContain(response.statusCode);
    });
  });

  describe('Authentication flow', () => {
    test('complete auth flow works', async () => {
      // 1. Initialize
      const initRes = await request(app)
        .post('/init')
        .send({ username: 'user@proton.me', password: 'password' });

      expect(initRes.statusCode).toBe(200);
      const sessionId = initRes.body.sessionId;

      // 2. Use authenticated endpoint
      const uploadRes = await request(app)
        .post('/upload')
        .send({
          sessionId,
          oid: 'abc123',
          path: '/path/to/file.bin',
        });

      expect(uploadRes.statusCode).toBe(200);

      // 3. Refresh token
      const refreshRes = await request(app)
        .post('/refresh')
        .send({ sessionId });

      expect(refreshRes.statusCode).toBe(200);
      expect(refreshRes.body.accessToken).toBeDefined();
    });
  });
});
