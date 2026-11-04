'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const path = require('node:path');

test('shipped contracts match actual Go Specs, not only another cached schema', { timeout: 60000 }, () => {
  const result = spawnSync(process.execPath, ['cindy-plugins/scripts/sync-contracts.mjs', '--check'], {
    cwd: path.resolve(__dirname, '../..'), encoding: 'utf8', timeout: 55000,
  });
  assert.equal(result.status, 0, result.stderr || result.error?.message);
  assert.match(result.stdout, /actual Go Specs, snapshot, manifest and browser media schema/);
});
