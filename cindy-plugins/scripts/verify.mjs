import { spawnSync } from 'node:child_process';
import { readFileSync, writeFileSync, readdirSync, mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const repo = fileURLToPath(new URL('../../', import.meta.url));
const evidence = path.join(repo, 'docs/features/cindy-plugins/acceptance/step2-evidence');
const tests = readdirSync(path.join(repo, 'cindy-plugins/tests')).filter(name => name.endsWith('.test.cjs')).sort();
const args = ['--test', '--test-reporter=spec', ...tests.map(name => `cindy-plugins/tests/${name}`)];
const result = spawnSync(process.execPath, args, { cwd: repo, encoding: 'utf8', maxBuffer: 8 * 1024 * 1024 });
process.stdout.write(result.stdout || ''); process.stderr.write(result.stderr || '');
mkdirSync(evidence, { recursive: true });
const record = { date: new Date().toISOString(), kind: 'automated-not-cindy-host', command: 'node cindy-plugins/scripts/verify.mjs',
  node: process.version, platform: process.platform, arch: process.arch,
  build: JSON.parse(readFileSync(path.join(repo, 'cindy-plugins/lumi/build-info.json'), 'utf8')),
  exit_code: result.status, stdout: result.stdout, stderr: result.stderr };
writeFileSync(path.join(evidence, 'automated.json'), JSON.stringify(record, null, 2) + '\n');
process.exit(result.status ?? 1);
