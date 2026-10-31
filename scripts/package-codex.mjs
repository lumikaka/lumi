#!/usr/bin/env node
// Package the portable source, never a machine-specific generated marketplace.
import { cpSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const [version = '0.1.0', output = 'build/codex'] = process.argv.slice(2);
if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$/.test(version)) {
  throw new Error('Version must be SemVer without v');
}
const out = resolve(output);
mkdirSync(out, { recursive: true });
const stage = mkdtempSync(join(tmpdir(), 'lumi-plugin-package-'));
try {
  cpSync(join(repo, 'codex-plugins/lumi'), join(stage, 'lumi'), { recursive: true });
  const manifestPath = join(stage, 'lumi/.codex-plugin/plugin.json');
  const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
  if (manifest.name !== 'lumi') throw new Error('Unexpected plugin name');
  manifest.version = version;
  writeFileSync(manifestPath, JSON.stringify(manifest, null, 2) + '\n');
  const mcp = JSON.parse(readFileSync(join(stage, 'lumi/.mcp.json'), 'utf8'));
  if (mcp.mcpServers.lumi.command !== '__LUMI_ABSOLUTE_BINARY__' || 'cwd' in mcp.mcpServers.lumi) {
    throw new Error('Source MCP must retain the installer placeholder and omit cwd');
  }
  const archive = join(out, 'lumi-codex-plugin.tar.gz');
  execFileSync('tar', ['-czf', archive, '-C', stage, 'lumi'], {
    env: { ...process.env, COPYFILE_DISABLE: '1' },
  });
  cpSync(join(repo, 'scripts/install-codex.sh'), join(out, 'install-codex.sh'));
  writeFileSync(join(out, 'lumi-codex-version.txt'), version + '\n');
  for (const name of ['lumi-codex-plugin.tar.gz', 'install-codex.sh']) {
    const sha = createHash('sha256').update(readFileSync(join(out, name))).digest('hex');
    writeFileSync(join(out, `${name}.sha256`), `${sha}  ${name}\n`);
  }
  console.log(`Lumi Codex plugin ${version}: ${out}`);
} finally {
  rmSync(stage, { recursive: true, force: true });
}
