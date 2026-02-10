const js = require('@eslint/js');
const globals = require('globals');

module.exports = [
  {
    ignores: [
      'node_modules/**',
      'coverage/**',
    ],
  },
  js.configs.recommended,
  {
    files: ['**/*.js'],
    languageOptions: {
      ecmaVersion: 2021,
      sourceType: 'commonjs',
      globals: {
        ...globals.node,
        ...globals.es2021,
      },
    },
    rules: {
      indent: [
        'error',
        2,
        {
          SwitchCase: 1,
          VariableDeclarator: 'first',
          FunctionDeclaration: {
            parameters: 'first',
          },
          FunctionExpression: {
            parameters: 'first',
          },
          CallExpression: {
            arguments: 'first',
          },
          ArrayExpression: 'first',
          ObjectExpression: 'first',
        },
      ],
      'linebreak-style': ['error', 'unix'],
      quotes: ['error', 'single', { avoidEscape: true }],
      semi: ['error', 'always'],
      'no-unused-vars': ['warn', { argsIgnorePattern: '^_', caughtErrorsIgnorePattern: '^_' }],
      'no-console': ['warn', { allow: ['warn', 'error'] }],
      'no-var': 'error',
      'prefer-const': ['error', { destructuring: 'any', ignoreReadBeforeAssign: false }],
      'prefer-arrow-callback': 'error',
      'no-implicit-coercion': 'error',
      'no-multi-spaces': 'error',
      'no-multiple-empty-lines': ['error', { max: 2, maxEOF: 1 }],
      'space-before-function-paren': [
        'error',
        {
          anonymous: 'always',
          named: 'never',
          asyncArrow: 'always',
        },
      ],
      'object-curly-spacing': ['error', 'always'],
      'array-bracket-spacing': ['error', 'never'],
      'computed-property-spacing': ['error', 'never'],
      'key-spacing': ['error', { beforeColon: false, afterColon: true }],
      'keyword-spacing': ['error', { before: true, after: true }],
      'space-infix-ops': 'error',
      'space-before-blocks': 'error',
      'no-unexpected-multiline': 'error',
      eqeqeq: ['error', 'always'],
      'no-eq-null': 'error',
      curly: ['error', 'all'],
      'no-empty-function': ['warn', { allow: ['arrowFunctions', 'functions', 'methods'] }],
      'no-else-return': 'error',
      'no-fallthrough': 'error',
      'require-await': 'warn',
    },
  },
  {
    files: ['tests/**/*.js', '**/*.test.js'],
    languageOptions: {
      globals: {
        ...globals.jest,
      },
    },
  },
];
