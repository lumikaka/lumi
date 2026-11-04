import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, readFileSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import { createHash } from 'node:crypto';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..');

test('release archive includes skill references that resolve after relocation', () => {
  const root = mkdtempSync(join(tmpdir(), 'lumi-plugin-contract-'));
  try {
    const assets = join(root, 'assets');
    execFileSync(process.execPath, [join(repo, 'scripts/package-codex.mjs'), '0.1.0-test', assets]);
    const installed = join(root, 'installed with spaces');
    mkdirSync(installed);
    execFileSync('tar', ['-xzf', join(assets, 'lumi-codex-plugin.tar.gz'), '-C', installed]);
    // Independent standard-library reader checks CRC, hidden files and exact
    // parity, not a second implementation of the writer under test.
    execFileSync(process.platform === 'win32' ? 'python' : 'python3', ['-c', `
import pathlib, sys, zipfile
archive, installed = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
with zipfile.ZipFile(archive) as z:
    assert z.testzip() is None
    expected = {p.relative_to(installed).as_posix(): p.read_bytes() for p in installed.rglob('*') if p.is_file()}
    actual = {n: z.read(n) for n in z.namelist()}
    assert actual == expected
    assert 'lumi/.codex-plugin/plugin.json' in actual
    assert 'lumi/.mcp.json' in actual
`, join(assets, 'lumi-codex-plugin.zip'), installed]);
    for (const name of ['install-codex.sh', 'install-codex.ps1', 'lumi-codex-plugin.zip', 'lumi-codex-plugin.tar.gz']) {
      const digest = createHash('sha256').update(readFileSync(join(assets, name))).digest('hex');
      assert.equal(readFileSync(join(assets, `${name}.sha256`), 'utf8'), `${digest}  ${name}\n`);
    }
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
