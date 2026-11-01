'use strict';
const readline = require('node:readline');
const mode = process.argv[2] || 'normal';
const write = value => process.stdout.write(JSON.stringify(value) + '\n');
readline.createInterface({ input: process.stdin }).on('line', line => {
  const req = JSON.parse(line);
  if (!req.id) return;
  if (req.method === 'initialize') {
    write({ jsonrpc: '2.0', id: req.id, result: { protocolVersion: '2025-03-26', serverInfo: { name: 'lumi-text', version: mode === 'version' ? '9.0.0' : '1.0.0' }, capabilities: { tools: {} } } });
  } else if (req.method === 'tools/list') {
    write({ jsonrpc: '2.0', id: req.id, result: { tools: ['project_overview', 'project_init', 'project_setup'].map(name => ({ name })) } });
  } else if (req.method === 'ping') {
    write({ jsonrpc: '2.0', id: req.id, result: {} });
  } else if (req.method === 'tools/call') {
    if (mode === 'crash') return process.exit(7);
    if (mode === 'malformed') return process.stdout.write('not JSON\n');
    if (mode === 'oversize') return process.stdout.write('x'.repeat(1024 * 1024 + 1));
    if (mode === 'reverse') write({ jsonrpc: '2.0', id: 'server-1', method: 'roots/list', params: {} });
    if (mode === 'wrongid') return write({ jsonrpc: '2.0', id: 'not-pending', result: {} });
    if (mode === 'invalid-envelope') return write({ jsonrpc: '2.0', id: req.id, result: { structuredContent: { success: true } } });
    if (mode === 'rpc-error') return write({ jsonrpc: '2.0', id: req.id, error: { code: -32602, message: 'private path must not escape' } });
    const result = { success: true, data: { project: { uuid: '01989abc-def0-7000-8000-000000000001' } } };
    if (mode === 'business-error') Object.assign(result, { success: false, data: null, error: { code: 'project_locked', message: 'locked', details: 'release owner' } });
    setTimeout(() => write({ jsonrpc: '2.0', id: req.id, result: { isError: !result.success, structuredContent: result } }), mode === 'slow' ? 250 : 0);
  }
});
