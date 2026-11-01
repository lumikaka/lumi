import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, readFileSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..');

test('release archive includes skill references that resolve after relocation', () => {
  const root = mkdtempSync(join(tmpdir(), 'lumi-plugin-contract-'));
  try {
    const assets = join(root, 'assets');
    execFileSync(process.execPath, [join(repo, 'scripts/package-codex.mjs'), '0.1.0-test', assets]);
    const installed = join(root, 'installed with spaces');
    mkdirSync(installed);
    execFileSync('tar', ['-xzf', join(assets, 'lumi-codex-plugin.tar.gz'), '-C', installed]);
    const skill = realpathSync(join(installed, 'lumi/skills/create-picture-book'));
    const entry = readFileSync(join(skill, 'SKILL.md'), 'utf8');
    const links = [...entry.matchAll(/\]\((references\/[^)]+)\)/g)];
    assert.ok(links.length > 0, 'missing skill reference links');
    for (const [, link] of links) {
      const relative = link.split('#')[0];
      const target = realpathSync(join(skill, relative));
      assert.ok(target.startsWith(skill + sep), `reference escapes installed skill: ${link}`);
      assert.deepEqual(readFileSync(target), readFileSync(join(repo, 'codex-plugins/lumi/skills/create-picture-book', relative)));
    }
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
