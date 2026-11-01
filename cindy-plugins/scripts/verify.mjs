import { spawnSync } from 'node:child_process';
import { readFileSync, writeFileSync, readdirSync, mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const repo = fileURLToPath(new URL('../../', import.meta.url));
const step = process.argv[2] || 'step2';
if (!/^step[0-9]+$/.test(step)) throw new Error('Expected an acceptance step such as step3');
const evidence = path.join(repo, `docs/features/cindy-plugins/acceptance/${step}-evidence`);
const tests = readdirSync(path.join(repo, 'cindy-plugins/tests')).filter(name => name.endsWith('.test.cjs')).sort();
const args = ['--test', '--test-reporter=spec', ...tests.map(name => `cindy-plugins/tests/${name}`)];
const result = spawnSync(process.execPath, args, { cwd: repo, encoding: 'utf8', maxBuffer: 8 * 1024 * 1024 });
process.stdout.write(result.stdout || ''); process.stderr.write(result.stderr || '');
mkdirSync(evidence, { recursive: true });
const record = { date: new Date().toISOString(), kind: 'automated-not-cindy-host', command: `node cindy-plugins/scripts/verify.mjs ${step}`,
  node: process.version, platform: process.platform, arch: process.arch,
  build: JSON.parse(readFileSync(path.join(repo, 'cindy-plugins/lumi/build-info.json'), 'utf8')),
  exit_code: result.status, stdout: result.stdout, stderr: result.stderr };
writeFileSync(path.join(evidence, 'automated.json'), JSON.stringify(record, null, 2) + '\n');
process.exit(result.status ?? 1);
