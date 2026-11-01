// Read-only audit of the installed Host's compiled pure text-result functions.
// Constructed inputs and mocked transport are not an installed-plugin E2E test.
import fs from 'node:fs';
import crypto from 'node:crypto';
import vm from 'node:vm';
import assert from 'node:assert/strict';
import path from 'node:path';

const archive = '/Applications/Cindy.app/Contents/Resources/app.asar';
const fd = fs.openSync(archive, 'r');
let source, entry;
try {
  const header = Buffer.alloc(16); fs.readSync(fd, header, 0, 16, 0);
  const json = Buffer.alloc(header.readUInt32LE(12)); fs.readSync(fd, json, 0, json.length, 16);
  const tree = JSON.parse(json.toString('utf8'));
  const files = tree.files['.vite'].files.build.files;
  entry = Object.keys(files).find(name => /^bootstrap-electron-.*\.js$/.test(name));
  assert.ok(entry, 'expected installed Electron main bundle');
  const value = files[entry], bytes = Buffer.alloc(value.size);
  fs.readSync(fd, bytes, 0, bytes.length, 8 + header.readUInt32LE(4) + Number(value.offset));
  source = bytes.toString('utf8');
} finally { fs.closeSync(fd); }
const sha256 = value => crypto.createHash('sha256').update(value).digest('hex');
const hashes = {};
function extract(start, end) {
  const from = source.indexOf(start); assert.ok(from >= 0, `missing ${start}`);
  const to = source.indexOf(end, from + start.length); assert.ok(to > from, `missing end of ${start}`);
  const result = source.slice(from, to).trim(); hashes[start] = sha256(result); return result;
}
const textResult = extract('function textResult(', 'const SETUP_SECRET_MAX_LENGTH');
const handler = extract('async function handleGhostCall(', 'async function handleForgeGuide(');
const dispatcher = extract('handleToolResult(n,R){', 'onRuntimeState(n,R){');
const pluginError = extract('function isGhostPluginErrorCode(', 'function isGhostIdValue(');
const mobile = extract('function projectMobileToolResult(', 'function projectMobileToolMessage(');
const reserved = source.match(/GHOST_HOST_ERROR_CODES=new Set\((\[[^\]]+\])\)/)?.[1];
assert.ok(reserved);
const bytesLimit = source.match(/MOBILE_TOOL_RESULT_BYTES=([^,;]+)/)?.[1];
assert.ok(bytesLimit);
const suffix = source.match(/(?<![A-Z_])TRUNCATION_SUFFIX=(`[^`]+`|"[^"\n]+")/)?.[1];
assert.ok(suffix);
const sandbox = { TextEncoder, TextDecoder,
  // Lumi text-only test inputs have no media, setup requirements or files.
  normalizeGhostModelImages: value => { assert.equal(value, undefined); return { images: [], dropped: 0 }; },
  hoistMediaFields: value => { assert.equal('xdt_image_url' in value, false); return {}; },
  sanitizeGhostSetupAssessment: value => { assert.equal(value, undefined); return null; },
  parseToolResultPayload: value => JSON.parse(value),
  extractPayloadToolResultFiles: () => [], extractPayloadToolCardIds: () => [],
};
const functions = vm.runInNewContext(`
const GHOST_HOST_ERROR_CODES=new Set(${reserved});
const MOBILE_TOOL_RESULT_BYTES=${bytesLimit},TRUNCATION_SUFFIX=${suffix},encoder=new TextEncoder();
${textResult}\n${pluginError}\n${handler}\n${mobile}
({dispatch:({${dispatcher}}).handleToolResult, handleGhostCall,projectMobileToolResult,MOBILE_TOOL_RESULT_BYTES,TRUNCATION_SUFFIX})`, sandbox);
let settled;
const receiver = { pending: new Map([['audit-call', { ghostId: 'lumi', ownerScopeSnapshot: {} }]]), deps: {},
  ownerScopeUsable: () => true, settle: (_id, value) => { settled = value; } };
const body = '正文与画面描述🌟'.repeat(5000) + '\nCOMPLETE-END-MARKER';
const lumi = { success: true, data: { description_md: body } };
assert.equal(functions.dispatch.call(receiver, 'lumi', { callId: 'audit-call', ok: true, result: lumi }).accepted, true);
assert.equal(settled.result.data.description_md, body);
const model = await functions.handleGhostCall({ callGhostTool: async () => settled }, { ghost_id: 'lumi', tool: 'call_tool' });
const modelText = model.content[0].text;
assert.equal(JSON.parse(modelText).result.data.description_md, body);
assert.equal(model.isError, undefined);
const projected = functions.projectMobileToolResult(modelText);
assert.ok(Buffer.byteLength(projected) <= functions.MOBILE_TOOL_RESULT_BYTES);
assert.ok(projected.endsWith(functions.TRUNCATION_SUFFIX));
assert.equal(projected.includes('COMPLETE-END-MARKER'), false);
assert.equal(projected.includes('\uFFFD'), false, 'UTF-8 boundary remains intact');
assert.equal(functions.projectMobileToolResult('small complete result'), 'small complete result');
// Failure schemas/rules must live in message: Host intentionally drops result.
const correction = 'invalid_arguments: ' + JSON.stringify({ input_schema: { type: 'object', additionalProperties: false }, rules: ['re-read before retry'] });
functions.dispatch.call(receiver, 'lumi', { callId: 'audit-call', ok: false, errorCode: 'LUMI_INVALID_ARGUMENTS', message: correction, result: { correction: 'dropped' } });
assert.equal(settled.errorCode, 'LUMI_INVALID_ARGUMENTS'); assert.equal(settled.result, undefined);
const failed = await functions.handleGhostCall({ callGhostTool: async () => settled }, { ghost_id: 'lumi', tool: 'call_tool' });
assert.equal(failed.isError, true); assert.equal(JSON.parse(failed.content[0].text).message, correction);
const evidence = {
  date: new Date().toISOString(), kind: 'installed-host-compiled-function-audit',
  boundary: 'Constructed text inputs; transport, owner scope and unused media helpers mocked. Not an installed-plugin or autonomous Agent E2E test. Exact public source unavailable.',
  archive, entry, bundle_sha256: sha256(source), function_sha256: hashes,
  host: { version: '0.1.121', source_commit: '0a7d5a419f5cd3e0fbebc9642dde8388376a68aa' },
  agent_result: { bytes: Buffer.byteLength(modelText), complete: true, end_marker: true },
  mobile_tool_history: { limit_bytes: functions.MOBILE_TOOL_RESULT_BYTES, projected_bytes: Buffer.byteLength(projected), explicit_truncation: true, utf8_intact: true },
  host_failure: { is_error: true, code_preserved: true, schema_and_rules_in_message: true, arbitrary_failure_result_dropped: true },
};
const target = process.argv[2]; assert.ok(target, 'provide evidence JSON path');
fs.mkdirSync(path.dirname(target), { recursive: true });
fs.writeFileSync(target, JSON.stringify(evidence, null, 2) + '\n');
console.log(JSON.stringify(evidence, null, 2));
