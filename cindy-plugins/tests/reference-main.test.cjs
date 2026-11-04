'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const crypto = require('node:crypto');
const source = fs.readFileSync(path.resolve(__dirname, '../lumi/main.js'), 'utf8');
const bytes = fs.readFileSync(path.resolve(__dirname, '../../docs/features/cindy-plugins/spike/media/core-fox.png'));
const hash = crypto.createHash('sha256').update(bytes).digest('hex');
const context = { session_id: 'real-host-task', workdir: '/temporary/project', workdir_is_local: true, workdir_is_read_only: false };
const prepared = { generation_prompt: 'Exact full business prompt, including labels and all text.', preparation: { digest: 'unchanged', signature: 'unchanged' }, reference_mode: 'collage', reference_image_path: '/private/tmp/lumi-page-premise-1234.png' };

async function invoke({ corrupt = false, deny = false, incomplete = false, none = false, badReceipt = false } = {}) {
  let callback; const requests = [], deposits = [], replies = [];
  vm.runInNewContext(source, { Uint8Array, crypto: crypto.webcrypto, atob, btoa,
    cindy: { onHostMessage(fn) { callback = fn; }, node: { async request(request) {
      requests.push(request); assert.equal(request.callId, 'actual-host-call'); assert.equal(request.cancelWithCall, true);
      assert.equal(request.params.invocation, 'actual-host-call'); assert.equal(JSON.stringify(request.params.context), JSON.stringify(context));
      let data;
      if (request.method === 'reference_prepare') return { ok: true, result: { success: true, data: none ? { ...prepared, reference_mode: 'none', reference_image_path: '' } : structuredClone(prepared), ...(!none ? { transport_reference: { transfer: 'opaque-worker-token', bytes: bytes.length, sha256: hash, mime_type: 'image/png', width: bytes.readUInt32BE(16), height: bytes.readUInt32BE(20), chunk_bytes: 48 * 1024 } } : {}) } };
      assert.equal(request.params.transfer, 'opaque-worker-token');
      if (request.method === 'reference_chunk') {
        const index = request.params.index, chunk = Buffer.from(bytes.subarray(index * 48 * 1024, (index + 1) * 48 * 1024));
        if (corrupt && index === 0) chunk[0] ^= 1;
        data = { index, base64: (incomplete ? chunk.subarray(1) : chunk).toString('base64') };
      } else { assert.equal(request.method, 'reference_release'); data = { released: true }; }
      return { ok: true, result: { success: true, data } };
    } }, async send(msg) {
      if (msg.type === 'cindy-request') {
        deposits.push(msg); return { ok: !deny, url: 'cindy-media://blobs/' + (badReceipt ? 'a'.repeat(64) : hash) + '.png', hash, bytes: bytes.length };
      }
      replies.push(msg);
    } },
  });
  await callback({ type: 'tool-call', tool: 'call_tool', callId: 'actual-host-call', args: { session_context: context, name: 'prepare_page_image', args: { page_uuid: '01989abc-def0-7000-8000-000000000001', setting_uuids: [] } } });
  return { requests, deposits, receipt: replies.at(-1) };
}

test('full prepared collage enters owned Core media unchanged; prompt/receipt survive and no token/base64 leaks', async () => {
  const { requests, deposits, receipt } = await invoke();
  assert.equal(receipt.ok, true); assert.equal(deposits.length, 1);
  assert.deepEqual(Buffer.from(deposits[0].data, 'base64'), bytes);
  assert.equal(deposits[0].callId, 'actual-host-call'); assert.equal(deposits[0].kind, 'deposit_media');
  assert.equal(requests.at(-1).method, 'reference_release');
  assert.equal(receipt.result.data.reference_media_url, 'cindy-media://blobs/' + hash + '.png');
  assert.equal(receipt.result.data.reference_delivery.status, 'ready');
  assert.equal(receipt.result.data.reference_delivery.byte_size, bytes.length);
  for (const key of Object.keys(prepared)) assert.equal(JSON.stringify(receipt.result.data[key]), JSON.stringify(prepared[key]));
  assert.equal(JSON.stringify(receipt).includes('opaque-worker-token'), false);
  assert.equal(JSON.stringify(receipt).includes('base64'), false);
  assert.equal(receipt.images, undefined, 'no bounded preview masquerades as the full collage');
});

test('corrupt/incomplete transfer and denied/wrong Host receipt explicitly fail Core delivery and always release', async () => {
  for (const options of [{ corrupt: true }, { incomplete: true }, { deny: true }, { badReceipt: true }]) {
    const { receipt, requests, deposits } = await invoke(options);
    assert.equal(receipt.result.data.reference_delivery.status, 'failed');
    assert.equal(receipt.result.data.reference_media_url, undefined);
    assert.equal(receipt.result.data.generation_prompt, prepared.generation_prompt);
    assert.equal(requests.at(-1).method, 'reference_release');
    if (options.corrupt || options.incomplete) assert.equal(deposits.length, 0);
  }
});

test('genuine no-reference preparation does not deposit or synthesize a reference', async () => {
  const { requests, deposits, receipt } = await invoke({ none: true });
  assert.equal(receipt.ok, true); assert.equal(requests.length, 1); assert.equal(deposits.length, 0);
  assert.equal(receipt.result.data.reference_media_url, undefined);
});
