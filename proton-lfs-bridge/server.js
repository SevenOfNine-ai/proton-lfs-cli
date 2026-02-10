/**
 * Proton LFS Bridge
 * HTTP bridge between Git LFS adapter and Proton Drive
 *
 * Provides RESTful API for:
 * - Session initialization with Proton credentials
 * - File upload with client-side encryption
 * - File download with client-side decryption
 * - Session management and token refresh
 *
 * See docs/architecture/proton-sdk-bridge.md for architecture
 */

const express = require('express');
const dotenv = require('dotenv');
const fs = require('fs');
const config = require('./lib/config');
const logger = require('./lib/logger');
const sessionManager = require('./lib/session');
const fileManager = require('./lib/fileManager');
const protonDriveBridge = require('./lib/protonDriveBridge');

// Load environment configuration
dotenv.config();

function createServer() {
  const app = express();
  const effectiveBackendMode = config.effectiveBackendMode;

  if (config.rawBackendMode !== effectiveBackendMode && config.rawBackendMode !== config.BACKEND_MODE_REAL) {
    logger.warn(`Unknown SDK_BACKEND_MODE=${config.rawBackendMode}; falling back to ${effectiveBackendMode}`);
  }

  function isRealBackendMode() {
    return config.isRealBackendMode();
  }

  function mapBridgeError(err, fallbackStatus, fallbackMessage) {
    if (err instanceof protonDriveBridge.BridgeError) {
      if (err.details) {
        logger.error(`Bridge error details: ${err.details}`);
      }
      const status = Number.isInteger(err.code) ? err.code : fallbackStatus;
      const publicMessage = String(err.message || fallbackMessage).trim() || fallbackMessage;
      return { status, message: publicMessage };
    }
    return { status: fallbackStatus, message: fallbackMessage };
  }

  function getRealSessionCredentials(session) {
    if (!session || !session.metadata || typeof session.metadata !== 'object') {
      return null;
    }
    const credentials = session.metadata.credentials;
    if (!credentials || typeof credentials !== 'object') {
      return null;
    }
    if (!credentials.username || !credentials.password) {
      return null;
    }
    return credentials;
  }

  // Middleware
  app.use(express.json());
  app.use(express.urlencoded({ extended: true }));

  // Request logging middleware
  app.use((req, res, next) => {
    logger.info(`${req.method} ${req.path}`);
    next();
  });

  /**
   * Health check endpoint
   * GET /health
   *
   * Returns service status and version
   */
  app.get('/health', (req, res) => {
    res.json({
      status: 'ok',
      version: '1.0.0',
      backendMode: effectiveBackendMode,
      timestamp: new Date().toISOString(),
      uptime: process.uptime()
    });
  });

  /**
   * Initialize SDK session with Proton credentials
   * POST /init
   *
   * Request body:
   * {
   *   "username": "user@proton.me",
   *   "password": "password"
   * }
   *
   * Response:
   * {
   *   "token": "session-token",
   *   "expiresAt": "2026-02-08T23:33:49Z",
   *   "userId": "user-id"
   * }
   *
   * Phase 4: Integrate with actual Proton SDK authentication
   */
  app.post('/init', async (req, res) => {
    try {
      const { username, password, dataPassword, secondFactorCode } = req.body;

      if (!username || !password) {
        return res.status(400).json({
          error: 'Missing username or password'
        });
      }

      logger.info(`Authenticating user: ${username}`);

      const sessionMetadata = {
        mode: effectiveBackendMode
      };

      if (isRealBackendMode()) {
        const resolvedDataPassword = dataPassword || config.PROTON_DATA_PASSWORD || password;
        const resolvedSecondFactorCode = secondFactorCode || config.PROTON_SECOND_FACTOR_CODE || '';

        await protonDriveBridge.authenticate({
          username,
          password,
          dataPassword: resolvedDataPassword,
          secondFactorCode: resolvedSecondFactorCode
        });
        sessionMetadata.credentials = {
          username,
          password,
          dataPassword: resolvedDataPassword,
          secondFactorCode: resolvedSecondFactorCode
        };
      }

      const token = await sessionManager.createSession(username, password, sessionMetadata);

      res.json({
        token: token.accessToken,
        expiresAt: token.expiresAt,
        userId: token.userId,
        refreshToken: token.refreshToken
      });
    } catch (error) {
      logger.error(`Auth error: ${error.message}`);
      const mapped = mapBridgeError(error, 401, 'authentication failed');
      res.status(mapped.status).json({ error: mapped.message });
    }
  });

  /**
   * Upload file to Proton Drive with encryption
   * POST /upload
   *
   * Request body:
   * {
   *   "token": "session-token",
   *   "oid": "sha256-hash",
   *   "path": "/path/to/local/file"
   * }
   *
   * Response:
   * {
   *   "oid": "sha256-hash",
   *   "size": 1024,
   *   "encrypted": true,
   *   "timestamp": "2026-02-08T23:33:49Z"
   * }
   *
   * Phase 4: Integrate with Proton SDK file encryption and upload
   */
  app.post('/upload', async (req, res) => {
    try {
      const { token, oid, path: filePath } = req.body;

      if (!token || !oid || !filePath) {
        return res.status(400).json({
          error: 'Missing required fields: token, oid, path'
        });
      }

      logger.info(`Upload request: OID=${oid} Path=${filePath}`);

      // Verify session is valid
      const session = sessionManager.validateSession(token);
      if (!session) {
        return res.status(401).json({ error: 'Invalid or expired session' });
      }

      // Verify file exists
      if (!fs.existsSync(filePath)) {
        return res.status(404).json({ error: 'File not found' });
      }

      let result;
      if (isRealBackendMode()) {
        const credentials = getRealSessionCredentials(session);
        if (!credentials) {
          return res.status(401).json({ error: 'Real backend session is missing credentials' });
        }
        result = await protonDriveBridge.uploadFile(credentials, oid, filePath);
      } else {
        result = await fileManager.uploadFile(token, oid, filePath);
      }

      res.json({
        oid: result.oid,
        size: result.size,
        encrypted: true,
        timestamp: new Date().toISOString(),
        location: result.location
      });
    } catch (error) {
      logger.error(`Upload error: ${error.message}`);
      const mapped = mapBridgeError(error, 500, 'upload failed');
      res.status(mapped.status).json({ error: mapped.message });
    }
  });

  /**
   * Download file from Proton Drive with decryption
   * POST /download
   *
   * Request body:
   * {
   *   "token": "session-token",
   *   "oid": "sha256-hash",
   *   "outputPath": "/path/to/output/file"
   * }
   *
   * Response:
   * {
   *   "oid": "sha256-hash",
   *   "size": 1024,
   *   "path": "/path/to/output/file",
   *   "timestamp": "2026-02-08T23:33:49Z"
   * }
   *
   * Phase 4: Integrate with Proton SDK file download and decryption
   */
  app.post('/download', async (req, res) => {
    try {
      const { token, oid, outputPath } = req.body;

      if (!token || !oid || !outputPath) {
        return res.status(400).json({
          error: 'Missing required fields: token, oid, outputPath'
        });
      }

      logger.info(`Download request: OID=${oid} OutputPath=${outputPath}`);

      // Verify session is valid
      const session = sessionManager.validateSession(token);
      if (!session) {
        return res.status(401).json({ error: 'Invalid or expired session' });
      }

      let result;
      if (isRealBackendMode()) {
        const credentials = getRealSessionCredentials(session);
        if (!credentials) {
          return res.status(401).json({ error: 'Real backend session is missing credentials' });
        }
        result = await protonDriveBridge.downloadFile(credentials, oid, outputPath);
      } else {
        result = await fileManager.downloadFile(token, oid, outputPath);
      }

      res.json({
        oid: result.oid,
        size: result.size,
        path: result.path,
        timestamp: new Date().toISOString()
      });
    } catch (error) {
      logger.error(`Download error: ${error.message}`);
      const mapped = mapBridgeError(error, 500, 'download failed');
      res.status(mapped.status).json({ error: mapped.message });
    }
  });

  /**
   * Refresh authentication token
   * POST /refresh
   *
   * Request body:
   * {
   *   "token": "session-token"
   * }
   *
   * Response:
   * {
   *   "token": "new-session-token",
   *   "expiresAt": "2026-02-09T23:33:49Z"
   * }
   */
  app.post('/refresh', async (req, res) => {
    try {
      const { token } = req.body;

      if (!token) {
        return res.status(400).json({ error: 'Missing token' });
      }

      logger.info('Token refresh requested');

      if (isRealBackendMode()) {
        const session = sessionManager.validateSession(token);
        if (!session) {
          return res.status(401).json({ error: 'Invalid or expired session' });
        }
        const credentials = getRealSessionCredentials(session);
        if (!credentials) {
          return res.status(401).json({ error: 'Real backend session is missing credentials' });
        }
        await protonDriveBridge.authenticate(credentials);
      }

      const newToken = await sessionManager.refreshSession(token);

      res.json({
        token: newToken.accessToken,
        expiresAt: newToken.expiresAt
      });
    } catch (error) {
      logger.error(`Refresh error: ${error.message}`);
      const mapped = mapBridgeError(error, 401, 'refresh failed');
      res.status(mapped.status).json({ error: mapped.message });
    }
  });

  /**
   * List files in LFS folder on Proton Drive
   * GET /list
   *
   * Query parameters:
   * - token: session token
   * - folder: folder path (default: LFS)
   *
   * Response:
   * {
   *   "files": [
   *     {"oid": "hash", "name": "file", "size": 1024, "modified": "..."}
   *   ]
   * }
   */
  app.get('/list', async (req, res) => {
    try {
      const { token, folder } = req.query;

      if (!token) {
        return res.status(400).json({ error: 'Missing token' });
      }

      const session = sessionManager.validateSession(token);
      if (!session) {
        return res.status(401).json({ error: 'Invalid or expired session' });
      }

      logger.info(`List files in folder: ${folder || 'LFS'}`);

      let files;
      if (isRealBackendMode()) {
        const credentials = getRealSessionCredentials(session);
        if (!credentials) {
          return res.status(401).json({ error: 'Real backend session is missing credentials' });
        }
        files = await protonDriveBridge.listFiles(credentials, folder || 'LFS');
      } else {
        files = await fileManager.listFiles(token, folder || 'LFS');
      }

      res.json({ files });
    } catch (error) {
      logger.error(`List error: ${error.message}`);
      const mapped = mapBridgeError(error, 500, 'list failed');
      res.status(mapped.status).json({ error: mapped.message });
    }
  });

  /**
   * Error handling middleware
   */
  app.use((req, res) => {
    res.status(404).json({ error: 'Not found' });
  });

  app.use((err, req, res, next) => {
    logger.error(`Unhandled error: ${err.message}`);
    res.status(500).json({ error: 'Internal server error' });
  });

  return { app, start };
}

function start(app) {
  const PORT = config.LFS_BRIDGE_PORT;
  const server = app.listen(PORT, () => {
    logger.info(`Proton LFS Bridge v1.0.0 listening on port ${PORT}`);
    logger.info(`Backend mode: ${config.effectiveBackendMode}`);
    logger.info('Available endpoints:');
    logger.info('  GET  /health          - Health check');
    logger.info('  POST /init            - Initialize session');
    logger.info('  POST /upload          - Upload file');
    logger.info('  POST /download        - Download file');
    logger.info('  POST /refresh         - Refresh token');
    logger.info('  GET  /list            - List files');
  });

  // Graceful shutdown
  process.on('SIGTERM', () => {
    logger.info('SIGTERM received, shutting down gracefully');
    server.close(() => {
      logger.info('Server closed');
      process.exit(0);
    });
  });

  return server;
}

if (require.main === module) {
  const { app, start } = createServer();
  start(app);
}

module.exports = { createServer };
