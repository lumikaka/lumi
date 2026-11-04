import { spawnSync } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync, chmodSync, readdirSync, copyFileSync, rmSync, lstatSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const repo = fileURLToPath(new URL('../../', import.meta.url));
const args = process.argv.slice(2);
let channel = 'production', dataDir = null;
while (args.length) {
  const flag = args.shift(), value = args.shift();
  if (!value) throw new Error('Expected --channel production|development [--data-dir /absolute/development/path]');
  if (flag === '--channel') channel = value;
  else if (flag === '--data-dir') dataDir = value;
  else throw new Error(`Unknown argument ${flag}`);
}
if (!['production', 'development'].includes(channel) || (dataDir !== null
  && (channel !== 'development' || !path.isAbsolute(dataDir) || dataDir.startsWith('//') || dataDir.includes('\0')))) throw new Error('Invalid channel or development data-dir');
const source = path.join(repo, 'cindy-plugins/lumi');
const synced = spawnSync(process.execPath, [path.join(repo, 'cindy-plugins/scripts/sync-contracts.mjs')], { cwd: repo, encoding: 'utf8' });
if (synced.status !== 0) throw new Error(synced.stderr || synced.error?.message || 'Contract generation failed');
const workspace = spawnSync(process.execPath, [path.join(repo, 'web/cindy-app/build.mjs')], { cwd: repo, encoding: 'utf8' });
if (workspace.status !== 0) throw new Error(workspace.stderr || workspace.error?.message || 'Cindy workspace build failed');
const development = channel === 'development';
const author = development ? path.join(repo, 'build/cindy-dev/lumi-dev') : source;
if (process.platform !== 'darwin' || process.arch !== 'arm64') throw new Error('Currently packages only macOS arm64');
// Copy only author sources, never a previous package, binary, or installed copy.
function copySources(from, to) {
  mkdirSync(to, { recursive: true });
  for (const entry of readdirSync(from, { withFileTypes: true })) {
    if (entry.name.startsWith('.') || entry.name === 'bin' || entry.name === 'build-info.json' || entry.name.endsWith('.cindy')) continue;
    if (entry.isSymbolicLink()) throw new Error('Author sources must not contain symlinks');
    if (entry.isDirectory()) copySources(path.join(from, entry.name), path.join(to, entry.name));
    else if (entry.isFile()) copyFileSync(path.join(from, entry.name), path.join(to, entry.name));
  }
}
if (development) {
  // Fixed repository build destination; never remove a user-selected path.
  for (const dir of ['build', 'build/cindy-dev', 'build/cindy-dev/lumi-dev']) {
    try { if (lstatSync(path.join(repo, dir)).isSymbolicLink()) throw new Error('Build destination must not be a symlink'); }
    catch (error) { if (error.code !== 'ENOENT') throw error; }
  }
  rmSync(author, { recursive: true, force: true });
  copySources(source, author);
}
mkdirSync(path.join(author, 'bin'), { recursive: true });
const binaryName = development ? 'lumi_dev' : 'lumi';
const binary = path.join(author, 'bin', binaryName);
const run = (command, args) => {
  const result = spawnSync(command, args, { cwd: repo, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  if (result.status !== 0) throw new Error(result.stderr || result.error?.message || `${command} failed`);
  return result.stdout.trim();
};
run('go', ['build', '-trimpath', '-ldflags', `-X main.defaultEnvironment=${channel}`, '-o', binary, './cmd/lumi']);
chmodSync(binary, 0o755);
const manifest = JSON.parse(readFileSync(path.join(source, 'ghost.json'), 'utf8'));
if (development) {
  manifest.id = manifest.command = 'lumi-dev';
  manifest.name = 'Lumi（开发版）';
  manifest.description = '独立 development 环境的 Lumi 本地开发版。' + manifest.description;
  writeFileSync(path.join(author, 'ghost.json'), JSON.stringify(manifest, null, 2) + '\n');
}
writeFileSync(path.join(author, 'node/runtime-config.json'), JSON.stringify({ channel, environment: channel, binary: binaryName, data_dir: dataDir === null ? null : path.normalize(dataDir) }, null, 2) + '\n');
const info = {
  channel,
  source_commit: run('git', ['rev-parse', 'HEAD']),
  // Scope only files compiled into the CLI; unrelated author docs do not taint it.
  source_dirty: !!run('git', ['status', '--porcelain', '--', 'cmd', 'internal', 'web', 'go.mod', 'go.sum']),
  go_version: run('go', ['version']), platform: process.platform, arch: process.arch,
  cli_sha256: createHash('sha256').update(readFileSync(binary)).digest('hex'),
};
// Fingerprint every shipped source plus CLI; same-version builds remain distinct.
const digest = createHash('sha256');
function fingerprint(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name, 'en'))) {
    if (entry.name.startsWith('.') || entry.name === 'build-info.json' || entry.name.endsWith('.cindy')) continue;
    const file = path.join(dir, entry.name);
    if (entry.isDirectory()) fingerprint(file);
    else { digest.update(path.relative(author, file) + '\0'); digest.update(readFileSync(file)); }
  }
}
fingerprint(author);
info.build_id = digest.digest('hex');
info.author_source_dirty = !!run('git', ['status', '--porcelain', '--', 'cindy-plugins', 'Makefile']);
writeFileSync(path.join(author, 'build-info.json'), JSON.stringify(info, null, 2) + '\n');
console.log(JSON.stringify(info, null, 2));
