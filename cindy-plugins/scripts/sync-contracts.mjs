// Always reads the actual Go contract; checking two stale snapshots is not a
// drift check. Does not open projects, contact models or expand the allowlist.
import { spawnSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { isDeepStrictEqual } from 'node:util';

const repo = fileURLToPath(new URL('../../', import.meta.url));
const require = createRequire(import.meta.url);
const { receiveMediaSchema } = require('../lumi/node/media-contract.cjs');
const check = process.argv.includes('--check');
const result = spawnSync('go', ['run', './cindy-plugins/schema'], { cwd: repo, encoding: 'utf8', maxBuffer: 8 * 1024 * 1024 });
if (result.status !== 0) throw new Error(result.stderr || result.error?.message || 'Go contract generation failed');
const snapshot = JSON.parse(result.stdout), schema = receiveMediaSchema(snapshot);
const snapshotPath = new URL('../lumi/node/business-specs.json', import.meta.url);
const manifestPath = new URL('../lumi/ghost.json', import.meta.url);
const mainPath = new URL('../lumi/main.js', import.meta.url);
const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
const tool = manifest.tools.find(tool => tool.name === 'receive_media');
if (!tool) throw new Error('Explicit receive_media declaration missing');
const start = '  // BEGIN GENERATED MEDIA CONTRACT\n', end = '  // END GENERATED MEDIA CONTRACT';
const main = readFileSync(mainPath, 'utf8');
const from = main.indexOf(start), to = main.indexOf(end, from);
if (from < 0 || to < 0) throw new Error('Missing generated media contract markers');
const generated = `${start}  schemas.receive_media = ${JSON.stringify(schema)};\n${end}`;
const next = main.slice(0, from) + generated + main.slice(to + end.length);
const drift = [];
if (!isDeepStrictEqual(JSON.parse(readFileSync(snapshotPath, 'utf8')), snapshot)) drift.push('business-specs.json');
if (!isDeepStrictEqual(tool.parameters, schema)) drift.push('ghost.json receive_media');
if (main !== next) drift.push('main.js receive_media');
if (check && drift.length) throw new Error(`Cindy contract drift: ${drift.join(', ')}. Run node cindy-plugins/scripts/sync-contracts.mjs`);
if (!check) {
  writeFileSync(snapshotPath, JSON.stringify(snapshot) + '\n');
  tool.parameters = schema;
  writeFileSync(manifestPath, JSON.stringify(manifest, null, 2) + '\n');
  writeFileSync(mainPath, next);
}
console.log(`CINDY CONTRACT ${check ? 'CHECK' : 'SYNC'} PASS: actual Go Specs, snapshot, manifest and browser media schema (${snapshot.length} business tools; explicit allowlist unchanged)`);
