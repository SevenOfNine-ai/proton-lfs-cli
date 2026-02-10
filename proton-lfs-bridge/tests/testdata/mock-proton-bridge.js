#!/usr/bin/env node

const fs = require('fs');

const command = process.argv[2] || '';
const input = fs.readFileSync(0, 'utf8');
const request = input.trim() ? JSON.parse(input) : {};

function write(payload, exitCode = 0) {
  if (process.env.MOCK_BRIDGE_STDOUT_NOISE === '1') {
    process.stdout.write('Build succeeded.\n');
  }
  if (process.env.MOCK_BRIDGE_STDERR_NOISE === '1') {
    process.stderr.write('mock bridge warning\n');
  }
  process.stdout.write(JSON.stringify(payload));
  process.exit(exitCode);
}

if (command === 'auth') {
  if (request.username === 'bad-user') {
    write({ ok: false, code: 401, error: 'invalid credentials' }, 1);
  }
  write({
    ok: true,
    payload: {
      authenticated: true,
      username: request.username
    }
  });
}

if (command === 'upload') {
  if (request.oid === 'missing') {
    write({ ok: false, code: 404, error: 'source object missing' }, 1);
  }
  write({
    ok: true,
    payload: {
      oid: request.oid,
      size: 123,
      location: `${request.storageBase}/${String(request.oid).slice(0, 2)}/${String(request.oid).slice(2)}`
    }
  });
}

if (command === 'download') {
  write({
    ok: true,
    payload: {
      oid: request.oid,
      size: 321,
      path: request.outputPath
    }
  });
}

if (command === 'list') {
  write({
    ok: true,
    payload: {
      files: [
        {
          oid: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
          name: 'fixture',
          size: 1
        }
      ]
    }
  });
}

write({ ok: false, code: 400, error: `unsupported command: ${command}` }, 1);
