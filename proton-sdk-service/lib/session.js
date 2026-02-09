/**
 * Session management for Proton Drive authentication
 * 
 * Handles:
 * - Session token generation and validation
 * - Token expiration and refresh
 * - Basic in-memory session storage (Phase 4: replace with persistent storage)
 * 
 * Phase 4: Integrate with actual Proton SDK authentication
 * See docs/architecture/proton-sdk-bridge.md for authentication flow
 */

const crypto = require('crypto');
const logger = require('./logger');

// In-memory session storage
// Phase 4: Replace with database or Redis for production
const sessions = new Map();

// Session configuration
const SESSION_DURATION = 24 * 60 * 60 * 1000; // 24 hours
const TOKEN_LENGTH = 32;

function cloneMetadata(metadata) {
  if (!metadata || typeof metadata !== 'object') {
    return {};
  }
  return JSON.parse(JSON.stringify(metadata));
}

function newSession(username, metadata = {}) {
  const token = crypto.randomBytes(TOKEN_LENGTH).toString('hex');
  const refreshToken = crypto.randomBytes(TOKEN_LENGTH).toString('hex');
  const expiresAt = new Date(Date.now() + SESSION_DURATION);
  const userId = crypto.createHash('sha256').update(username).digest('hex').slice(0, 16);

  return {
    accessToken: token,
    refreshToken: refreshToken,
    userId: userId,
    username: username,
    metadata: cloneMetadata(metadata),
    expiresAt: expiresAt,
    createdAt: new Date()
  };
}

/**
 * Create a new session
 * Phase 4: Replace with actual Proton authentication
 */
async function createSession(username, password, metadata = {}) {
  try {
    // Phase 4: Call actual Proton SDK authentication
    // For now, validate credentials locally (insecure, demoonly)
    if (!username || !password) {
      throw new Error('Invalid credentials');
    }

    const session = newSession(username, metadata);

    sessions.set(session.accessToken, session);
    logger.info(`Session created for user: ${username}`);

    return session;
  } catch (error) {
    logger.error(`Failed to create session: ${error.message}`);
    throw error;
  }
}

/**
 * Validate a session token
 */
function validateSession(token) {
  if (!token) {
    return null;
  }

  const session = sessions.get(token);
  
  if (!session) {
    logger.warn('Invalid session token');
    return null;
  }

  // Check expiration
  if (new Date() > session.expiresAt) {
    logger.warn(`Session expired for user: ${session.username}`);
    sessions.delete(token);
    return null;
  }

  return session;
}

/**
 * Refresh a session token
 */
async function refreshSession(token) {
  try {
    const session = validateSession(token);
    
    if (!session) {
      throw new Error('Invalid or expired session');
    }

    // Remove old session
    sessions.delete(token);

    // Refresh with same user identity and new token pair.
    const refreshed = newSession(session.username, session.metadata);
    sessions.set(refreshed.accessToken, refreshed);
    
    logger.info(`Session refreshed for user: ${session.username}`);
    
    return refreshed;
  } catch (error) {
    logger.error(`Failed to refresh session: ${error.message}`);
    throw error;
  }
}

/**
 * Revoke a session
 */
function revokeSession(token) {
  const session = sessions.get(token);
  if (session) {
    sessions.delete(token);
    logger.info(`Session revoked for user: ${session.username}`);
    return true;
  }
  return false;
}

/**
 * Get current session count (for monitoring)
 */
function getSessionCount() {
  return sessions.size;
}

/**
 * Clean up expired sessions periodically
 */
function cleanupExpiredSessions() {
  let deleted = 0;
  const now = new Date();
  
  for (const [token, session] of sessions.entries()) {
    if (now > session.expiresAt) {
      sessions.delete(token);
      deleted++;
    }
  }
  
  if (deleted > 0) {
    logger.debug(`Cleaned up ${deleted} expired sessions`);
  }
}

// Run cleanup every 5 minutes
const cleanupInterval = setInterval(cleanupExpiredSessions, 5 * 60 * 1000);
if (cleanupInterval.unref) {
  cleanupInterval.unref();
}

module.exports = {
  createSession,
  validateSession,
  refreshSession,
  revokeSession,
  getSessionCount,
  cleanupExpiredSessions
};
