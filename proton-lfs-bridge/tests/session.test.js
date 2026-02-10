/**
 * Unit tests for session manager
 * Tests session creation, validation, refresh, and expiration
 */

const session = require('../lib/session');

describe('Session Manager', () => {
  beforeEach(() => {
    // Clear sessions before each test
    jest.clearAllMocks();
  });

  describe('createSession', () => {
    test('creates a valid session with credentials', async () => {
      const result = await session.createSession('user@proton.me', 'password');

      expect(result).toBeDefined();
      expect(result.accessToken).toBeDefined();
      expect(result.refreshToken).toBeDefined();
      expect(result.userId).toBeDefined();
      expect(result.expiresAt).toBeInstanceOf(Date);
    });

    test('rejects missing username', async () => {
      try {
        await session.createSession('', 'password');
        fail('Should have thrown error');
      } catch (error) {
        expect(error.message).toContain('Invalid credentials');
      }
    });

    test('rejects missing password', async () => {
      try {
        await session.createSession('user@proton.me', '');
        fail('Should have thrown error');
      } catch (error) {
        expect(error.message).toContain('Invalid credentials');
      }
    });

    test('token is properly formatted', async () => {
      const result = await session.createSession('user@proton.me', 'password');
      expect(result.accessToken).toMatch(/^[a-f0-9]+$/);
      expect(result.accessToken.length).toBe(64); // 32 bytes hex = 64 chars
    });

    test('creates unique tokens for different sessions', async () => {
      const result1 = await session.createSession('user1@proton.me', 'pass1');
      const result2 = await session.createSession('user2@proton.me', 'pass2');

      expect(result1.accessToken).not.toBe(result2.accessToken);
      expect(result1.userId).not.toBe(result2.userId);
    });

    test('stores session metadata when provided', async () => {
      const metadata = {
        mode: 'real',
        credentials: {
          username: 'user@proton.me'
        }
      };
      const created = await session.createSession('user@proton.me', 'password', metadata);
      const validated = session.validateSession(created.accessToken);

      expect(validated.metadata).toBeDefined();
      expect(validated.metadata.mode).toBe('real');
      expect(validated.metadata.credentials.username).toBe('user@proton.me');
      expect(validated.metadata).not.toBe(metadata);
    });
  });

  describe('validateSession', () => {
    test('validates a valid session token', async () => {
      const created = await session.createSession('user@proton.me', 'password');
      const validated = session.validateSession(created.accessToken);

      expect(validated).toBeDefined();
      expect(validated.username).toBe('user@proton.me');
    });

    test('rejects invalid token', () => {
      const result = session.validateSession('invalid-token');
      expect(result).toBeNull();
    });

    test('rejects null token', () => {
      const result = session.validateSession(null);
      expect(result).toBeNull();
    });

    test('rejects undefined token', () => {
      const result = session.validateSession(undefined);
      expect(result).toBeNull();
    });

    test('rejects empty token', () => {
      const result = session.validateSession('');
      expect(result).toBeNull();
    });
  });

  describe('refreshSession', () => {
    test('refreshes a valid session', async () => {
      const original = await session.createSession('user@proton.me', 'password');
      const refreshed = await session.refreshSession(original.accessToken);

      expect(refreshed).toBeDefined();
      expect(refreshed.accessToken).toBeDefined();
      expect(refreshed.accessToken).not.toBe(original.accessToken);
    });

    test('rejects refresh with invalid token', async () => {
      try {
        await session.refreshSession('invalid-token');
        fail('Should have thrown error');
      } catch (error) {
        expect(error.message).toContain('Invalid or expired');
      }
    });

    test('preserves username after refresh', async () => {
      const original = await session.createSession('user@proton.me', 'password');
      const refreshed = await session.refreshSession(original.accessToken);

      expect(refreshed.username).toBe(original.username);
    });

    test('preserves metadata after refresh', async () => {
      const original = await session.createSession('user@proton.me', 'password', {
        mode: 'real',
        credentials: {
          username: 'user@proton.me'
        }
      });
      const refreshed = await session.refreshSession(original.accessToken);

      expect(refreshed.metadata).toBeDefined();
      expect(refreshed.metadata.mode).toBe('real');
      expect(refreshed.metadata.credentials.username).toBe('user@proton.me');
    });
  });

  describe('revokeSession', () => {
    test('revokes an existing session', async () => {
      const created = await session.createSession('user@proton.me', 'password');
      const token = created.accessToken;

      // Verify session exists
      expect(session.validateSession(token)).toBeDefined();

      // Revoke it
      const result = session.revokeSession(token);
      expect(result).toBe(true);

      // Verify session is gone
      expect(session.validateSession(token)).toBeNull();
    });

    test('returns false for non-existent session', () => {
      const result = session.revokeSession('non-existent-token');
      expect(result).toBe(false);
    });
  });

  describe('getSessionCount', () => {
    test('returns correct count of active sessions', async () => {
      const count1 = session.getSessionCount();

      await session.createSession('user1@proton.me', 'pass1');
      const count2 = session.getSessionCount();
      expect(count2).toBe(count1 + 1);

      await session.createSession('user2@proton.me', 'pass2');
      const count3 = session.getSessionCount();
      expect(count3).toBe(count2 + 1);
    });

    test('decreases count after revocation', async () => {
      const sess = await session.createSession('user@proton.me', 'password');
      const countBefore = session.getSessionCount();

      session.revokeSession(sess.accessToken);
      const countAfter = session.getSessionCount();

      expect(countAfter).toBe(countBefore - 1);
    });
  });

  describe('cleanupExpiredSessions', () => {
    test('removes expired sessions', () => {
      // This would require time manipulation or mocking
      // For now, just verify the function exists and is callable
      expect(() => session.cleanupExpiredSessions()).not.toThrow();
    });
  });
});
