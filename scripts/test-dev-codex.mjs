import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { chmodSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const script = join(repo, 'scripts/dev-codex.mjs');
const json = path => JSON.parse(readFileSync(path, 'utf8'));

function fixture(t) {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'lumi-dev-contract-')));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const out = join(root, "checkout with spaces and 'quote'");
  mkdirSync(out);
  writeFileSync(join(out, 'lumi_dev'), '#!/bin/sh\nexit 0\n');
  chmodSync(join(out, 'lumi_dev'), 0o755);
  const data = join(root, "dev data 'quote'");
  const args = [script, '--out', out, '--data-dir', data];
  return { root, out, data, args, plugin: join(out, 'marketplace/plugins/lumi-dev') };
}

test('development plugin shares authoring resources and keeps its CLI isolated after relocation', t => {
  const f = fixture(t);
  execFileSync(process.execPath, f.args);
  const manifest = json(join(f.plugin, '.codex-plugin/plugin.json'));
  assert.equal(manifest.name, 'lumi-dev');
  const market = json(join(f.out, 'marketplace/.agents/plugins/marketplace.json'));
  assert.equal(market.name, 'lumi-dev-local');
  assert.equal(market.plugins[0].name, manifest.name);
  const mcp = json(join(f.plugin, '.mcp.json'));
  assert.deepEqual(Object.keys(mcp.mcpServers), ['lumi-dev']);
  assert.deepEqual(mcp.mcpServers['lumi-dev'], {
    command: join(f.out, 'lumi_dev'),
    args: ['mcp', '--host', 'codex', '--environment', 'development', '--data-dir', f.data],
  });
  for (const path of ['skills/create-picture-book/SKILL.md', 'skills/create-picture-book/references/mcp-usage.md', 'skills/create-picture-book/references/page-images.md', 'skills/create-picture-book/references/batch-settings.md', 'assets/icon.png']) {
    assert.deepEqual(readFileSync(join(f.plugin, path)), readFileSync(join(repo, 'codex-plugins/lumi', path)));
  }
  const skill = readFileSync(join(f.plugin, 'skills/create-picture-book/SKILL.md'), 'utf8');
  for (const [, link] of skill.matchAll(/\]\((references\/[^)]+)\)/g)) {
    assert.ok(readFileSync(join(f.plugin, 'skills/create-picture-book', link.split('#')[0])).length);
  }
  // Execute the generated shell example with a recording stub: paths containing
  // spaces and quotes must reach the CLI unchanged, including the data dir.
  writeFileSync(join(f.out, 'lumi_dev'), '#!/usr/bin/env node\nprocess.stdout.write(JSON.stringify(process.argv.slice(2)))\n');
  const guide = readFileSync(join(f.plugin, 'skills/create-picture-book/references/cli.md'), 'utf8');
  const example = guide.split('\n').find(line => line.includes(' init --environment'));
  const received = JSON.parse(execFileSync('/bin/sh', ['-c', example], { encoding: 'utf8' }));
  assert.deepEqual(received, ['init', '--environment', 'development', '--data-dir', f.data, '--json']);
});

test('same-version regeneration replaces stale plugin files and updates the development data dir', t => {
  const f = fixture(t);
  execFileSync(process.execPath, f.args);
  const before = json(join(f.plugin, '.codex-plugin/plugin.json'));
  writeFileSync(join(f.plugin, 'skills/create-picture-book/SKILL.md'), 'stale skill');
  writeFileSync(join(f.plugin, 'obsolete-file'), 'stale');
  const newData = join(f.root, 'another data directory');
  execFileSync(process.execPath, [script, '--out', f.out, '--data-dir', newData]);
  assert.deepEqual(json(join(f.plugin, '.codex-plugin/plugin.json')), before);
  assert.deepEqual(readFileSync(join(f.plugin, 'skills/create-picture-book/SKILL.md')),
    readFileSync(join(repo, 'codex-plugins/lumi/skills/create-picture-book/SKILL.md')));
  assert.throws(() => readFileSync(join(f.plugin, 'obsolete-file')), { code: 'ENOENT' });
  assert.equal(json(join(f.plugin, '.mcp.json')).mcpServers['lumi-dev'].args.at(-1), newData);
});

test('generation refuses an unmanaged marketplace without changing its files', t => {
  const f = fixture(t);
  mkdirSync(join(f.out, 'marketplace'));
  const existing = join(f.out, 'marketplace/keep');
  writeFileSync(existing, 'unrelated');
  const result = spawnSync(process.execPath, f.args, { encoding: 'utf8' });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /unmanaged marketplace/);
  assert.equal(readFileSync(existing, 'utf8'), 'unrelated');
});
