'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const { Runtime } = require('../lumi/node/runtime.cjs');
const { RuntimeError } = require('../lumi/node/errors.cjs');
const uuid = '01989abc-def0-7000-8000-000000000001';

test('oversize write is a known pre-dispatch rejection with the correct schema', async () => {
  let touched = false;
  const runtime = new Runtime();
  runtime.binary = async () => { touched = true; };
  await assert.rejects(runtime.execute('call_tool', { arguments: { name: 'chapters', args: {
    action: 'write', chapter_uuid: uuid, content: '文'.repeat(100000), expected_revision: 1,
  } } }), error => error.code === 'arguments_too_large' && error.correction.tool.name === 'chapters');
  assert.equal(touched, false);
});

test('unknown write result is not replayed; reread recovers the committed text', async t => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'lumi-cindy-recovery-'));
  let writes = 0, committed = '';
  const peer = { closed: false, async initialize() {}, async close() { this.closed = true; },
    async overview() { return { success: true, data: { project: { uuid, name: 'book', revision: 1 }, setup: { project_uuid: uuid, revision: 1, setup_status: 'ready' } } }; },
    async call(name, args) {
      assert.equal(name, 'story_profile');
      if (args.action === 'write') {
        writes++; committed = args.story_md;
        throw new RuntimeError('request_timeout', 'simulated lost acknowledgement');
      }
      return { success: true, data: { uuid, revision: 2, story_md: committed } };
    },
  };
  const runtime = new Runtime({ executable: process.execPath, build: { platform: process.platform, arch: process.arch, cli_sha256: 'fixture' },
    dataDir: root, verifyBinary: false, connectionFactory: () => peer });
  runtime.inspect = async () => ({ running: true, nonce: 'a'.repeat(64), pid: 1, endpoint: 'http://127.0.0.1:1' });
  t.after(async () => { await runtime.close(); await fs.rm(root, { recursive: true }); });
  const call = args => runtime.execute('call_tool', { context: { session_id: 'writer', workdir: root, workdir_is_local: true, workdir_is_read_only: false },
    arguments: { name: 'story_profile', args } });
  await assert.rejects(call({ action: 'write', story_md: '# Saved before timeout', expected_revision: 1 }), { code: 'result_unknown' });
  assert.equal(writes, 1);
  const read = await call({ action: 'get' });
  assert.equal(read.data.story_md, '# Saved before timeout');
  assert.equal(read.data.revision, 2);
  assert.equal(writes, 1);
});
