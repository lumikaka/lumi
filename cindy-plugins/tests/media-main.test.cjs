'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const vm = require('node:vm');
const source = fs.readFileSync(path.resolve(__dirname, '../lumi/main.js'), 'utf8');
const png = fs.readFileSync(path.resolve(__dirname, '../../docs/features/cindy-plugins/spike/media/core-fox.png'));
const hash = crypto.createHash('sha256').update(png).digest('hex');
const context = { session_id: 'host-task', workdir: '/temporary/project', workdir_is_local: true, workdir_is_read_only: false };
const input = { target_uuid: '01989abc-def0-7000-8000-000000000001', slot: 'page_image', expected_revision: 0, prompt: 'Exact complete prompt', preparation: { project_uuid: '01989abc-def0-7000-8000-000000000001', page_uuid: '01989abc-def0-7000-8000-000000000001', expected_revision: 0, setting_uuids: [], additional_direction: '', digest: 'a'.repeat(64), signature: 'b'.repeat(64) } };
function host({ bytes = png, contentLength = bytes.length, mime = 'image/png', responseHash = hash, nodeResult, denied = false } = {}) {
  let callback;
  const requests = [], replies = [], urls = [];
  vm.runInNewContext(source, { crypto: crypto.webcrypto, Uint8Array, AbortController, setTimeout, clearTimeout, btoa,
    fetch: async url => { urls.push(url); return denied ? new Response('', { status: 404 }) : new Response(bytes, { headers: { 'content-length': String(contentLength), 'content-type': mime } }); },
    cindy: { onHostMessage(fn) { callback = fn; }, node: { async request(request) {
      requests.push(request);
      assert.equal(request.callId, 'host-call'); assert.equal(request.cancelWithCall, true);
      if (nodeResult) return { ok: true, result: nodeResult };
      return { ok: true, result: { success: true, data: request.method === 'media_begin' ? { transfer: 'internal-transfer' } : request.method === 'media_commit' ? { asset: { uuid: input.target_uuid }, source_sha256: responseHash } : {} } };
    } }, async send(reply) {
      replies.push(reply);
      if (reply.type === 'cindy-request') return { ok: true, url: 'cindy-media://blobs/' + hash + '.jpg' };
    } },
  });
  return { requests, replies, urls, invokeExact: args => callback({ type: 'tool-call', tool: 'receive_media', callId: 'host-call', args: { session_context: context, attachments: [hash], ...args } }), invoke: (args = {}, tool = 'receive_media') => callback({ type: 'tool-call', tool, callId: 'host-call', args: { ...input, attachments: [hash], session_context: context, ...args } }) };
}

test('authorized board and page review evidence cross the browser adapter without model paths or lost fields', async () => {
  const board = { slot: 'setting_board', expected_revision: 2, source_text: 'Fox and lantern', style_snapshot: 'flat ochre', prompt: 'Complete board prompt', idempotency_key: 'board-1' };
  const h = host(); await h.invokeExact(board);
  assert.equal(h.replies.at(-1).ok, true);
  assert.deepEqual(JSON.parse(JSON.stringify(h.requests.find(r => r.method === 'media_begin').params.input)), board);
  const reviews = [{ setting_uuid: input.target_uuid, variant_uuid: input.target_uuid, expected_revision: 2, style_digest: 'c'.repeat(64), verdict: 'passed', reference_basis: 'Saved style', comparison: 'Thin dark contours match.' }];
  const page = host(); await page.invoke({ preparation: { ...input.preparation, setting_reviews: reviews } });
  assert.equal(page.replies.at(-1).ok, true);
  assert.deepEqual(JSON.parse(JSON.stringify(page.requests.find(r => r.method === 'media_begin').params.input.preparation.setting_reviews)), reviews);
});

test('Host-authorized original is hashed, split below Node params limit and scoped to the actual call', async () => {
  const h = host(); await h.invoke();
  assert.equal(h.replies.at(-1).ok, true);
  assert.deepEqual(h.urls, [`/media/${hash}.png`]);
  const begin = h.requests.find(r => r.method === 'media_begin');
  assert.equal(begin.params.sha256, hash); assert.equal(begin.params.bytes, png.length);
  assert.equal(begin.params.invocation, 'host-call');
  assert.equal(JSON.stringify(begin.params.input.preparation), JSON.stringify(input.preparation));
  assert.equal(begin.params.input.prompt, input.prompt);
  const chunks = h.requests.filter(r => r.method === 'media_chunk');
  assert.ok(chunks.length > 1);
  assert.deepEqual(Buffer.concat(chunks.map(r => Buffer.from(r.params.base64, 'base64'))), png);
  for (const [index, r] of chunks.entries()) {
    assert.equal(r.params.index, index); assert.ok(Buffer.byteLength(JSON.stringify(r.params)) < 256 * 1024);
  }
  assert.equal(h.replies.at(-1).result.data.source_sha256, hash);
  assert.equal(JSON.stringify(h.replies).includes('base64'), false);
});

test('attachment absence, forged fields, read-only, denied media, wrong checksum, MIME and size fail before transfer', async () => {
  for (const [options, args, code] of [
    [{}, { attachments: [] }, 'media_attachment_required'],
    [{}, { path: '/private/forged.png' }, 'invalid_arguments'],
    [{}, { session_context: { ...context, workdir_is_read_only: true } }, 'read_only_context'],
    [{ denied: true }, {}, 'media_unavailable'],
    [{ bytes: Buffer.from('unexpected') }, {}, 'media_format_mismatch'],
    [{ bytes: Buffer.concat([png, Buffer.from('different')]) }, {}, 'media_checksum_mismatch'],
    [{ mime: 'image/jpeg' }, {}, 'media_format_mismatch'],
    [{ contentLength: 21 * 1024 * 1024 }, {}, 'media_too_large'],
  ]) {
    const h = host(options); await h.invoke(args);
    assert.equal(h.replies.at(-1).result.error.code, code); assert.equal(h.requests.length, 0);
  }
});

test('reference preview is removed from result JSON and delivered via Host images plus an owned Core reference URL', async () => {
  const h = host({ nodeResult: { success: true, data: { asset_uuid: input.target_uuid, width: 256, height: 256 }, transport_image: { data: 'bounded-jpeg-preview', mime_type: 'image/jpeg' } } });
  await h.invoke({ name: 'read_image', args: { asset_uuid: input.target_uuid }, target_uuid: undefined, slot: undefined, expected_revision: undefined, attachments: undefined }, 'call_tool');
  // Top-level extra keys are rejected even if their value is undefined.
  assert.equal(h.replies.at(-1).result.error.code, 'invalid_arguments');
  let callback, receipt, deposited;
  vm.runInNewContext(source, { cindy: { onHostMessage(fn) { callback = fn; }, node: { request: async () => ({ ok: true, result: { success: true, data: { asset_uuid: input.target_uuid }, transport_image: { data: 'bounded-jpeg-preview', mime_type: 'image/jpeg' } } }) }, send: async msg => {
    if (msg.type === 'cindy-request') { deposited = msg; return { ok: true, url: 'cindy-media://blobs/' + hash + '.jpg' }; }
    receipt = msg;
  } } });
  await callback({ type: 'tool-call', tool: 'call_tool', callId: 'reference-call', args: { session_context: context, name: 'read_image', args: { asset_uuid: input.target_uuid } } });
  assert.equal(deposited.kind, 'deposit_media'); assert.equal(deposited.callId, 'reference-call');
  assert.equal(receipt.images[0].data, 'bounded-jpeg-preview');
  assert.equal(JSON.stringify(receipt.result).includes('bounded-jpeg-preview'), false);
  assert.match(receipt.result.data.reference_media_url, /^cindy-media:/);
});
