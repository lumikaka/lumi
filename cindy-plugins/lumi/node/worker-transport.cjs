'use strict';
const { MAX_LINE } = require('./protocol.cjs');
const { RuntimeError, envelope, aborted } = require('./errors.cjs');
const catalog = require('./catalog.cjs');

function createWorker({ input, output, runtime, spawnLifetime }) {
  let buffer = Buffer.alloc(0), discard = false, stopped = false;
  const calls = new Map();
  const write = value => {
    let line = JSON.stringify(value);
    if (Buffer.byteLength(line) > MAX_LINE) line = JSON.stringify({ jsonrpc: '2.0', id: value.id,
      result: envelope(new RuntimeError('result_too_large', '运行时结果超过单行限制。', '缩小读取范围。')) });
    if (!stopped) output.write(line + '\n');
  };
  const rpcError = (id, code, message) => write({ jsonrpc: '2.0', id, error: { code, message } });
  function dispatch(line) {
    let request;
    try { request = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(line)); }
    catch { rpcError(null, -32700, 'Invalid JSON'); return; }
    if (!request || Array.isArray(request) || request.jsonrpc !== '2.0' || typeof request.method !== 'string') {
      rpcError(null, -32600, 'Invalid request'); return;
    }
    if (request.method === 'notifications/cancelled' && request.id === undefined) {
      calls.get(request.params?.requestId)?.controller.abort();
      return;
    }
    if (typeof request.id !== 'string' && !Number.isSafeInteger(request.id)) { rpcError(null, -32600, 'Request id required'); return; }
    if (calls.has(request.id)) { rpcError(request.id, -32600, 'Duplicate request id'); return; }
    if (calls.size >= 4) {
      write({ jsonrpc: '2.0', id: request.id, result: envelope(new RuntimeError('runtime_busy', '同时调用上限为 4。', '等待一个调用完成后重试。')) });
      return;
    }
    if (Buffer.byteLength(JSON.stringify(request.params ?? {})) > 256 * 1024) {
      const error = new RuntimeError('arguments_too_large', 'Node 参数超过 256 KiB。', '缩小单次参数，不截断完整正文冒充成功。');
      error.correction = catalog.repair(request.method === 'call_tool' ? request.params?.arguments?.name : request.method);
      write({ jsonrpc: '2.0', id: request.id, result: envelope(error) });
      return;
    }
    const controller = new AbortController();
    const call = { controller };
    calls.set(request.id, call);
    // Capture Host AsyncLocalStorage synchronously, before any await. Only this
    // small sentinel is scoped to the request; Go project proxies remain shared.
    let lifetime;
    try {
      if (request.cindy?.cancelWithCall !== true || !spawnLifetime) throw new RuntimeError('cancellation_unavailable', '宿主未提供调用取消绑定。', '使用支持 cancelWithCall 与 childSpawn 的 Cindy 构建。');
      lifetime = spawnLifetime('node/request-lifetime.cjs');
    } catch (error) { lifetime = Promise.reject(error); }
    (async () => {
      let child, settled = false;
      const abort = () => { if (!settled) controller.abort(); };
      const deadline = setTimeout(abort, 50000);
      try {
        child = await lifetime;
        child.once('exit', abort);
        child.once('error', abort);
        // Child exit may precede spawnEntry's resolution. The Host proxy
        // retains that fact in killed; an exit listener alone can miss it.
        if (child.killed === true) controller.abort();
        aborted(controller.signal);
        const result = await runtime.execute(request.method, request.params, controller.signal);
        write({ jsonrpc: '2.0', id: request.id, result });
      } catch (error) {
        write({ jsonrpc: '2.0', id: request.id, result: envelope(error) });
      } finally {
        settled = true;
        clearTimeout(deadline);
        // The Host returns a narrow child proxy with on/once/off, not a Node
        // ChildProcess. Release capacity before best-effort external cleanup;
        // a late child exit or cleanup error must not leak a completed slot.
        calls.delete(request.id);
        try { child?.off('exit', abort); } catch {}
        try { child?.off('error', abort); } catch {}
        try { child?.kill(); } catch {}
      }
    })();
  }
  input.on('data', bytes => {
    if (stopped) return;
    buffer = Buffer.concat([buffer, bytes]);
    for (;;) {
      const end = buffer.indexOf(10);
      if (end < 0) {
        if (buffer.length > MAX_LINE) { buffer = Buffer.alloc(0); discard = true; }
        return;
      }
      const line = buffer.subarray(0, end);
      buffer = buffer.subarray(end + 1);
      if (discard || line.length > MAX_LINE) { discard = false; rpcError(null, -32600, 'Request line too large'); }
      else if (line.length) dispatch(line);
    }
  });
  const close = async () => {
    stopped = true;
    for (const { controller } of calls.values()) controller.abort();
    await runtime.close();
  };
  input.on('end', close);
  return { close };
}

module.exports = { createWorker };
