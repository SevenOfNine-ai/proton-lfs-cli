module.exports = {
  testEnvironment: 'node',
  coverageDirectory: 'coverage',
  collectCoverageFrom: [
    'lib/**/*.js',
    '!**/node_modules/**',
    '!**/tests/**',
  ],
  testMatch: [
    '**/tests/**/*.test.js',
  ],
  coverageThreshold: {
    global: {
      branches: 70,
      functions: 80,
      lines: 80,
      statements: 80,
    },
  },
  setupFilesAfterEnv: [
    '<rootDir>/tests/setup.js',
  ],
  testTimeout: 10000,
  verbose: true,
};
