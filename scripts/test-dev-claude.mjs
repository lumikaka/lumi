// Claude Code development plugin and write-protection hook. Every test uses a
// temporary output, data dir and a recording fake `claude` CLI; the real
// Claude Code configuration is never touched.
import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import { decide, lumiProjectRoot, protectedReference } from '../claude-plugins/lumi/hooks/guard-lumi-project.mjs';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const script = join(repo, 'scripts/dev-claude.mjs');
const hook = join(repo, 'claude-plugins/lumi/hooks/guard-lumi-project.mjs');
const json = path => JSON.parse(readFileSync(path, 'utf8'));

function temp(t, prefix) {
  const root = realpathSync(mkdtempSync(join(tmpdir(), prefix)));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  return root;
}

function fixture(t) {
  const root = temp(t, 'lumi-claude-dev-');
  const out = join(root, "checkout with spaces and 'quote'");
  mkdirSync(out);
  writeFileSync(join(out, 'lumi_dev'), '#!/bin/sh\nexit 0\n');
  chmodSync(join(out, 'lumi_dev'), 0o755);
  const data = join(root, "dev data 'quote'");
  // Fake claude CLI: keeps marketplace/plugin state in a JSON file and logs argv.
  const state = join(root, 'claude-state.json');
  const log = join(root, 'claude-calls.jsonl');
  const fake = join(root, 'claude');
  writeFileSync(fake, `#!/usr/bin/env node
const fs = require('fs');
const state = ${JSON.stringify(state)}, log = ${JSON.stringify(log)};
const args = process.argv.slice(2);
fs.appendFileSync(log, JSON.stringify(args) + '\\n');
const s = fs.existsSync(state) ? JSON.parse(fs.readFileSync(state, 'utf8')) : { marketplaces: [], plugins: [] };
const save = () => fs.writeFileSync(state, JSON.stringify(s));
const key = args.slice(0, 3).join(' ');
if (args[0] === '--version') console.log('2.1.319 (Claude Code)');
else if (key === 'plugin marketplace list') console.log(JSON.stringify(s.marketplaces));
else if (key === 'plugin marketplace add') {
  const path = args[3];
  const name = JSON.parse(fs.readFileSync(path + '/.claude-plugin/marketplace.json', 'utf8')).name;
  s.marketplaces.push({ name, source: 'directory', path, installLocation: path }); save();
  console.log(JSON.stringify({ command: 'marketplace add', outcome: 'success', message: 'added' }));
} else if (key === 'plugin marketplace update') console.log(JSON.stringify({ command: 'marketplace update', outcome: 'success', message: 'updated' }));
else if (key === 'plugin marketplace remove') {
  s.marketplaces = s.marketplaces.filter(m => m.name !== args[3]);
  s.plugins = s.plugins.filter(p => !p.id.endsWith('@' + args[3])); save();
  console.log(JSON.stringify({ command: 'marketplace remove', outcome: 'success', message: 'removed' }));
} else if (args[0] === 'plugin' && args[1] === 'list') console.log(JSON.stringify(s.plugins));
else if (args[0] === 'plugin' && args[1] === 'install') {
  if (process.env.FAKE_CLAUDE_FAIL_INSTALL) { console.error('install failed'); process.exit(1); }
  s.plugins.push({ id: args[2], version: '0.1.0-dev', scope: 'user', enabled: true, installPath: '' }); save();
  console.log('Installing...');
  console.log(JSON.stringify({ outcome: 'success', plugin: args[2] }));
} else if (args[0] === 'plugin' && args[1] === 'uninstall') {
  s.plugins = s.plugins.filter(p => p.id !== args[2]); save();
  console.log(JSON.stringify({ outcome: 'success', plugin: args[2] }));
} else { console.error('unexpected ' + args.join(' ')); process.exit(1); }
`);
  chmodSync(fake, 0o755);
  const env = { ...process.env, CLAUDE_BIN: fake, HOME: root };
  const calls = () => existsSync(log) ? readFileSync(log, 'utf8').trim().split('\n').map(line => JSON.parse(line)) : [];
  const args = [script, '--out', out, '--data-dir', data];
  return { root, out, data, args, env, state, calls, marketplace: join(out, 'marketplace'), plugin: join(out, 'marketplace/plugins/lumi-dev') };
}

test('development plugin shares release skill, hooks and assets with a dev-only MCP entry', t => {
  const f = fixture(t);
  execFileSync(process.execPath, f.args, { env: f.env });
  const manifest = json(join(f.plugin, '.claude-plugin/plugin.json'));
  assert.equal(manifest.name, 'lumi-dev');
  for (const field of ['mcpServers', 'interface', 'capabilities', 'apps']) assert.ok(!(field in manifest), field);
  const market = json(join(f.marketplace, '.claude-plugin/marketplace.json'));
  assert.equal(market.name, 'lumi-dev-local');
  assert.equal(market.owner.name, 'Lumi');
  assert.deepEqual(market.plugins.map(p => [p.name, p.source]), [['lumi-dev', './plugins/lumi-dev']]);
  const mcp = json(join(f.plugin, '.mcp.json'));
  assert.deepEqual(mcp, { mcpServers: { 'lumi-dev': {
    command: join(f.out, 'lumi_dev'),
    args: ['mcp', '--host', 'claude', '--environment', 'development', '--data-dir', f.data],
  } } });
  for (const path of ['skills/create-picture-book/SKILL.md', 'skills/create-picture-book/references/mcp-usage.md',
    'skills/create-picture-book/references/page-images.md', 'skills/create-picture-book/references/batch-settings.md',
    'hooks/hooks.json', 'hooks/guard-lumi-project.mjs', 'assets/icon.png']) {
    assert.deepEqual(readFileSync(join(f.plugin, path)), readFileSync(join(repo, 'claude-plugins/lumi', path)), path);
  }
  const skill = readFileSync(join(f.plugin, 'skills/create-picture-book/SKILL.md'), 'utf8');
  for (const [, link] of skill.matchAll(/\]\((references\/[^)#]+)/g)) {
    assert.ok(readFileSync(join(f.plugin, 'skills/create-picture-book', link)).length, link);
  }
  // The generated shell example reaches the CLI with spaces and quotes intact.
  writeFileSync(join(f.out, 'lumi_dev'), '#!/usr/bin/env node\nprocess.stdout.write(JSON.stringify(process.argv.slice(2)))\n');
  const guide = readFileSync(join(f.plugin, 'skills/create-picture-book/references/cli.md'), 'utf8');
  assert.match(guide, /cli_upgrade_required/);
  assert.match(guide, /claude-dev-stop/);
  const example = guide.split('\n').find(line => line.includes(' init --environment'));
  const received = JSON.parse(execFileSync('/bin/sh', ['-c', example], { encoding: 'utf8' }));
  assert.deepEqual(received, ['init', '--environment', 'development', '--data-dir', f.data, '--json']);
  assert.deepEqual(f.calls(), [], 'generation without --install must not call claude');
});

test('release plugin uses the claude host and declares no App capability', () => {
  const plugin = join(repo, 'claude-plugins/lumi');
  const manifest = json(join(plugin, '.claude-plugin/plugin.json'));
  assert.equal(manifest.name, 'lumi');
  assert.ok(!('interface' in manifest) && !('capabilities' in manifest));
  assert.deepEqual(json(join(plugin, '.mcp.json')), { mcpServers: { lumi: { command: '__LUMI_ABSOLUTE_BINARY__', args: ['mcp', '--host', 'claude'] } } });
  const hooks = json(join(plugin, 'hooks/hooks.json')).hooks.PreToolUse;
  assert.equal(hooks[0].matcher, 'Bash');
  assert.deepEqual(hooks[0].hooks[0].args, ['${CLAUDE_PLUGIN_ROOT}/hooks/guard-lumi-project.mjs']);
  assert.ok(!existsSync(join(plugin, 'CLAUDE.md')));
});

test('install registers through the official CLI once, refresh updates in place, uninstall removes both', t => {
  const f = fixture(t);
  execFileSync(process.execPath, [...f.args, '--install'], { env: f.env, encoding: 'utf8' });
  const first = f.calls().map(args => args.slice(0, 3).join(' '));
  assert.deepEqual(first, ['--version', 'plugin marketplace list', 'plugin marketplace add', 'plugin install lumi-dev@lumi-dev-local']);
  assert.deepEqual(f.calls()[2], ['plugin', 'marketplace', 'add', f.marketplace, '--scope', 'user', '--json']);
  assert.deepEqual(f.calls()[3], ['plugin', 'install', 'lumi-dev@lumi-dev-local', '--scope', 'user', '--json']);

  writeFileSync(join(f.plugin, 'stale'), 'old');
  execFileSync(process.execPath, [...f.args, '--install'], { env: f.env, encoding: 'utf8' });
  const second = f.calls().slice(4).map(args => args.slice(0, 3).join(' '));
  assert.deepEqual(second, ['--version', 'plugin marketplace list', 'plugin list --json', 'plugin marketplace update']);
  assert.ok(!existsSync(join(f.plugin, 'stale')), 'refresh replaces plugin files');

  const output = execFileSync(process.execPath, [...f.args, '--uninstall'], { env: f.env, encoding: 'utf8' });
  assert.match(output, /preserved/);
  const third = f.calls().slice(8).map(args => args.slice(0, 3).join(' '));
  assert.deepEqual(third, ['--version', 'plugin marketplace list', 'plugin list --json', 'plugin uninstall lumi-dev@lumi-dev-local', 'plugin marketplace remove']);
  assert.ok(!existsSync(f.marketplace));
  assert.ok(existsSync(join(f.out, 'lumi_dev')), 'uninstall keeps the CLI');
  assert.deepEqual(json(f.state), { marketplaces: [], plugins: [] });
});

test('install refuses a same-named marketplace from another checkout', t => {
  const f = fixture(t);
  writeFileSync(f.state, JSON.stringify({ marketplaces: [{ name: 'lumi-dev-local', source: 'directory', path: '/elsewhere/marketplace' }], plugins: [] }));
  const result = spawnSync(process.execPath, [...f.args, '--install'], { env: f.env, encoding: 'utf8' });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /already belongs to \/elsewhere\/marketplace/);
  assert.ok(!existsSync(f.marketplace));
});

test('a failed install keeps the generated files so the command can be rerun', t => {
  const f = fixture(t);
  const result = spawnSync(process.execPath, [...f.args, '--install'], { env: { ...f.env, FAKE_CLAUDE_FAIL_INSTALL: '1' }, encoding: 'utf8' });
  assert.notEqual(result.status, 0);
  assert.ok(existsSync(join(f.plugin, '.mcp.json')));
  execFileSync(process.execPath, [...f.args, '--install'], { env: f.env });
  assert.deepEqual(json(f.state).plugins.map(p => p.id), ['lumi-dev@lumi-dev-local']);
});

test('generation refuses an unmanaged marketplace without changing its files', t => {
  const f = fixture(t);
  mkdirSync(f.marketplace);
  writeFileSync(join(f.marketplace, 'keep'), 'unrelated');
  const result = spawnSync(process.execPath, f.args, { env: f.env, encoding: 'utf8' });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /unmanaged marketplace/);
  assert.equal(readFileSync(join(f.marketplace, 'keep'), 'utf8'), 'unrelated');
});

function runHook(input) {
  const result = spawnSync(process.execPath, [hook], { input: JSON.stringify(input), encoding: 'utf8', env: { ...process.env, CLAUDE_PROJECT_DIR: '' } });
  assert.equal(result.status, 0, result.stderr);
  return result.stdout.trim() ? JSON.parse(result.stdout) : null;
}

test('write-protection hook denies protected paths only inside a Lumi project', t => {
  const root = temp(t, 'lumi-claude-hook-');
  const book = join(root, 'book');
  const other = join(root, 'other');
  mkdirSync(join(book, 'chapters'), { recursive: true });
  writeFileSync(join(book, 'project.sqlite'), '');
  mkdirSync(other);
  assert.equal(lumiProjectRoot(join(book, 'chapters')), book);
  assert.equal(lumiProjectRoot(other), null);

  const blocked = [
    'sqlite3 project.sqlite "select 1"', 'rm -f ./project.sqlite-wal', 'ls assets/', 'cp x.png assets/pages/a.png',
    'cat .lumi/project.lock', 'cd .lumi && ls', `rm ${book}/assets/a.png`, `cat ${book}//.lumi/tmp/x`, 'echo hi > assets',
  ];
  for (const command of blocked) {
    const result = runHook({ tool_name: 'Bash', cwd: book, tool_input: { command } });
    assert.equal(result?.hookSpecificOutput?.permissionDecision, 'deny', command);
    assert.equal(result.hookSpecificOutput.hookEventName, 'PreToolUse');
    assert.match(result.hookSpecificOutput.permissionDecisionReason, /Lumi 项目写保护/);
  }
  // A subdirectory of the project is still protected.
  assert.ok(runHook({ tool_name: 'Bash', cwd: join(book, 'chapters'), tool_input: { command: 'ls ../assets/ assets' } }));

  const allowed = ['ls', 'git status', 'cat web/assets/logo.svg', 'ls ~/.lumi_dev', 'lumi init --json', 'cat README.md', 'grep -r lumi .'];
  for (const command of allowed) assert.equal(runHook({ tool_name: 'Bash', cwd: book, tool_input: { command } }), null, command);
  // Outside a Lumi project, even protected names pass through untouched.
  for (const command of blocked) assert.equal(runHook({ tool_name: 'Bash', cwd: other, tool_input: { command } }), null, command);
  // Non-Bash input, missing fields and unparseable stdin make no decision.
  assert.equal(decide({ tool_name: 'Read', cwd: book, tool_input: { file_path: 'project.sqlite' } }, {}), null);
  assert.equal(decide({ tool_name: 'Bash', cwd: book, tool_input: {} }, {}), null);
  const garbage = spawnSync(process.execPath, [hook], { input: 'not json', encoding: 'utf8' });
  assert.equal(garbage.status, 0);
  assert.equal(garbage.stdout, '');
  // CLAUDE_PROJECT_DIR covers a cwd that moved outside the project.
  assert.ok(decide({ tool_name: 'Bash', cwd: other, tool_input: { command: 'ls assets' } }, { CLAUDE_PROJECT_DIR: book }));
  // A .lumi directory alone also marks a project.
  mkdirSync(join(other, '.lumi'));
  assert.equal(protectedReference('ls .lumi', other), '.lumi');
  assert.ok(runHook({ tool_name: 'Bash', cwd: other, tool_input: { command: 'ls .lumi' } }));
});
