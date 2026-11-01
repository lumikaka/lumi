import { spawnSync } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync, chmodSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const repo = fileURLToPath(new URL('../../', import.meta.url));
const author = path.join(repo, 'cindy-plugins/lumi');
if (process.platform !== 'darwin' || process.arch !== 'arm64') throw new Error('Step 1 currently packages only macOS arm64');
mkdirSync(path.join(author, 'bin'), { recursive: true });
const binary = path.join(author, 'bin/lumi');
const run = (command, args) => {
  const result = spawnSync(command, args, { cwd: repo, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  if (result.status !== 0) throw new Error(result.stderr || result.error?.message || `${command} failed`);
  return result.stdout.trim();
};
run('go', ['build', '-trimpath', '-o', binary, './cmd/lumi']);
chmodSync(binary, 0o755);
const info = {
  source_commit: run('git', ['rev-parse', 'HEAD']),
  // Scope only files compiled into the CLI; unrelated author docs do not taint it.
  source_dirty: !!run('git', ['status', '--porcelain', '--', 'cmd', 'internal', 'web', 'go.mod', 'go.sum']),
  go_version: run('go', ['version']), platform: process.platform, arch: process.arch,
  cli_sha256: createHash('sha256').update(readFileSync(binary)).digest('hex'),
};
writeFileSync(path.join(author, 'build-info.json'), JSON.stringify(info, null, 2) + '\n');
console.log(JSON.stringify(info, null, 2));
