/**
 * Jest setup and configuration
 * Runs before all tests
 */

// Suppress console output during tests unless LOG_LEVEL is set
if (!process.env.LOG_LEVEL) {
  process.env.LOG_LEVEL = 'error';
}

// Set test environment
process.env.NODE_ENV = 'test';
process.env.SDK_SERVICE_PORT = '3000';
