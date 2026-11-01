'use strict';
// Standalone automation harness, excluded from the plugin package. This is not
// a Cindy Host test: it substitutes spawnEntry using the test's own Node runtime.
const path = require('node:path');
const fs = require('node:fs');
const os = require('node:os');
const { spawn } = require('node:child_process');
const { Runtime } = require('../../lumi/node/runtime.cjs');
const { MCPConnection } = require('../../lumi/node/protocol.cjs');
const { createWorker } = require('../../lumi/node/worker-transport.cjs');
const author = path.join(__dirname, '../../lumi');
const runtime = new Runtime({ executable: path.join(author, 'bin/lumi'),
  build: JSON.parse(fs.readFileSync(path.join(author, 'build-info.json'), 'utf8')),
  dataDir: process.argv[2], env: { HOME: process.argv[3], PATH: '/usr/bin:/bin', TMPDIR: os.tmpdir() },
  connectionFactory(options) {
    const peer = new MCPConnection(options);
    process.send?.({ kind: 'proxy', pid: peer.child.pid });
    return peer;
  },
});
createWorker({ input: process.stdin, output: process.stdout, runtime,
  spawnLifetime() {
    return Promise.resolve(spawn(process.execPath, [path.join(author, 'node/request-lifetime.cjs')], { stdio: ['pipe', 'ignore', 'ignore'] }));
  },
});
