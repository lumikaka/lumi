'use strict';
// Spawned by the Host, bound to exactly one cancelWithCall RPC. Its exit is the
// public child-lifecycle cancellation signal; it owns no project/server process.
process.stdin.resume();
process.stdin.on('end', () => process.exit(0));
