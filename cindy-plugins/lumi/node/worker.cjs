'use strict';
const path = require('node:path');
const os = require('node:os');
const fs = require('node:fs');
const { Runtime } = require('./runtime.cjs');
const { createWorker } = require('./worker-transport.cjs');

// Host loads this entry with require(), so startup must be unconditional.
let build;
try { build = JSON.parse(fs.readFileSync(path.join(__dirname, '..', 'build-info.json'), 'utf8')); }
catch { build = { platform: process.platform, arch: process.arch, cli_sha256: 'missing' }; }
const runtime = new Runtime({ executable: path.join(__dirname, '..', 'bin', 'lumi'), build,
  dataDir: path.join(os.homedir(), '.lumi') });
const worker = createWorker({ input: process.stdin, output: process.stdout, runtime,
  spawnLifetime: globalThis.__CINDY_NODE__?.spawnEntry });
process.once('SIGTERM', () => worker.close().finally(() => process.exit(0)));
process.once('SIGINT', () => worker.close().finally(() => process.exit(0)));
