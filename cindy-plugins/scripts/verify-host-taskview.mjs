// Read-only audit of the actually installed application, not a claim about main
// or the public release tag. No Host state, history or private DB is inspected.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
const repo = fileURLToPath(new URL('../../', import.meta.url));
const archive = '/Applications/Cindy.app/Contents/Resources/app.asar';
const fd = fs.openSync(archive, 'r'), header = Buffer.alloc(16);
fs.readSync(fd, header, 0, 16, 0);
const json = Buffer.alloc(header.readUInt32LE(12)); fs.readSync(fd, json, 0, json.length, 16);
const tree = JSON.parse(json.toString()), offset = 8 + header.readUInt32LE(4);
const hash = value => crypto.createHash('sha256').update(value).digest('hex');
function read(parts) {
  let node = tree;
  for (const part of parts) node = node.files[part];
  assert.ok(node && Number.isSafeInteger(node.size));
  const bytes = Buffer.alloc(node.size); fs.readSync(fd, bytes, 0, bytes.length, offset + Number(node.offset));
  return { text: bytes.toString(), sha256: hash(bytes), path: parts.join('/') };
}
let record;
try {
  const buildFiles = tree.files['.vite'].files.build.files;
  const bootstrap = read(['.vite', 'build', Object.keys(buildFiles).find(name => /^bootstrap-electron-.*\.js$/.test(name))]);
  const assetFiles = tree.files['.vite'].files.renderer.files.main_window.files.assets.files;
  const sidebar = read(['.vite', 'renderer', 'main_window', 'assets', Object.keys(assetFiles).find(name => /^SidebarWindowLayout-.*\.js$/.test(name))]);
  const start = bootstrap.text.indexOf('function ensureGhostProtocolRegistered(');
  const end = bootstrap.text.indexOf('async function serveGhostMedia(', start);
  assert.ok(start >= 0 && end > start, 'locate the actual plugin protocol handler');
  const handler = bootstrap.text.slice(start, end);
  const routes = [...new Set([...handler.matchAll(/\.pathname==="([^"]+)"/g)].map(m => m[1]))].sort();
  assert.ok(routes.includes('/wake') && routes.includes('/app-context'));
  const contextAt = handler.indexOf('.pathname==="/app-context"');
  const contextBranch = handler.slice(contextAt, handler.indexOf('.pathname===', contextAt + 20));
  assert.match(contextBranch, /ghostAppContextProvider\(\)/);
  const queryPattern = 'query:`session=${encodeURIComponent(t.sessionId)}`';
  const queryAt = sidebar.text.indexOf(queryPattern);
  assert.ok(queryAt >= 0, 'taskView renderer supplies the session routing query');
  record = {
    date: new Date().toISOString(), kind: 'installed-host-compiled-contract-audit-not-ui-e2e',
    source: JSON.parse(fs.readFileSync('/Applications/Cindy.app/Contents/Resources/cindy-source.json')),
    bootstrap: { entry: bootstrap.path, sha256: bootstrap.sha256 },
    renderer: { entry: sidebar.path, sha256: sidebar.sha256 },
    protocol_handler_sha256: hash(handler), exact_public_routes: routes,
    task_view_query: { fields: ['session'], excerpt_sha256: hash(sidebar.text.slice(queryAt - 120, queryAt + queryPattern.length)) },
    app_context_handler: { provider: 'ghostAppContextProvider', branch_sha256: hash(contextBranch) },
    documented_app_context_fields: ['region', 'locale'],
    verified_page_identity: false,
    conclusion: 'The inspected/documented route and BroadcastChannel contracts do not attest a taskView sender session. A session query is routing data, not project authorization.',
    limitations: ['No application source ref matching sourceCommit is publicly available.', 'This audit does not enumerate undocumented capabilities or replace a real taskView test.'],
  };
} finally { fs.closeSync(fd); }
const evidence = path.join(repo, 'docs/features/cindy-plugins/acceptance/step45-evidence');
fs.mkdirSync(evidence, { recursive: true });
fs.writeFileSync(path.join(evidence, 'host-contract.json'), JSON.stringify(record, null, 2) + '\n');
console.log(JSON.stringify({ result: 'PASS', checked: 'actual taskView query and public route handler',
  verified_page_identity: false, gate: 'business reads remain closed pending a verified binding mechanism', routes: record.exact_public_routes }, null, 2));
