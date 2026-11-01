'use strict';
const { spawn } = require('node:child_process');
const { RuntimeError, aborted, validEnvelope } = require('./errors.cjs');

const MAX_PARAMS = 240 * 1024;
const MAX_LINE = 1024 * 1024 - 1024;
const protocolError = () => new RuntimeError('protocol_error', 'CLI 协议响应无效。', '运行诊断后重连；不要自动重放写操作。');

// One transport owns one fixed cwd. It never owns the detached CLI server.
class MCPConnection {
  constructor({ executable, args, cwd, env, timeoutMs = 25000, onClose = () => {} }) {
    this.pending = new Map();
    this.ignored = new Set();
    this.nextID = 0;
    this.buffer = Buffer.alloc(0);
    this.closed = false;
    this.timeoutMs = timeoutMs;
    this.onClose = onClose;
    this.child = spawn(executable, args, { cwd, env, shell: false, stdio: ['pipe', 'pipe', 'pipe'] });
    this.exited = new Promise(resolve => this.child.once('close', resolve));
    this.child.stdout.on('data', bytes => this.feed(bytes));
    this.child.stderr.on('data', () => {}); // CLI details may contain private paths.
    this.child.stdin.on('error', () => this.fail(new RuntimeError('cli_exited', 'CLI 连接已关闭。', '重新调用以建立连接。')));
    this.child.on('error', error => this.fail(new RuntimeError(
      error.code === 'ENOENT' ? 'cli_missing' : 'cli_unavailable',
      '随包 CLI 无法启动。', '检查插件包完整性、执行位与电脑架构后重新打包。')));
    this.child.once('close', () => this.fail(new RuntimeError('cli_exited', 'CLI 子进程已退出。', '重新调用以建立连接。')));
  }

  send(value) {
    if (this.closed) throw new RuntimeError('cli_exited', 'CLI 连接已关闭。', '重新调用。');
    const line = JSON.stringify(value);
    if (Buffer.byteLength(line) > MAX_LINE) throw new RuntimeError('arguments_too_large', '请求过大。', '缩小单次调用参数。');
    this.child.stdin.write(line + '\n');
  }

  feed(bytes) {
    if (this.closed) return;
    this.buffer = Buffer.concat([this.buffer, bytes]);
    for (;;) {
      const end = this.buffer.indexOf(10);
      if (end < 0) {
        if (this.buffer.length > MAX_LINE) this.fail(new RuntimeError('result_too_large', 'CLI 响应超过单行限制。', '缩小读取范围后重连。'));
        return;
      }
      if (end > MAX_LINE) {
        this.fail(new RuntimeError('result_too_large', 'CLI 响应超过单行限制。', '缩小读取范围后重连。'));
        return;
      }
      const line = this.buffer.subarray(0, end);
      this.buffer = this.buffer.subarray(end + 1);
      if (!line.length) continue;
      try {
        const value = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(line));
        this.receive(value);
      } catch { this.fail(protocolError()); }
      if (this.closed) return;
    }
  }

  receive(value) {
    if (!value || Array.isArray(value) || value.jsonrpc !== '2.0') throw protocolError();
    if (typeof value.method === 'string') {
      if (Object.hasOwn(value, 'result') || Object.hasOwn(value, 'error')) throw protocolError();
      if (Object.hasOwn(value, 'id')) {
        if (!['string', 'number'].includes(typeof value.id)) throw protocolError();
        this.send({ jsonrpc: '2.0', id: value.id, error: { code: -32601, message: 'Client methods are unavailable' } });
      }
      // Progress is not a business-state update and must not keep the worker alive.
      return;
    }
    if (typeof value.id !== 'string' || Object.hasOwn(value, 'result') === Object.hasOwn(value, 'error')) throw protocolError();
    if (this.ignored.delete(value.id)) return; // A cancelled call may finish late.
    const request = this.pending.get(value.id);
    if (!request) throw protocolError();
    if (value.error && (!Number.isInteger(value.error.code) || typeof value.error.message !== 'string')) throw protocolError();
    request.finish(value.error ? new RuntimeError('mcp_error', 'CLI 拒绝了协议请求。', `MCP code ${value.error.code}；运行诊断后重试。`) : null, value.result);
  }

  request(method, params = {}, { signal, timeoutMs = this.timeoutMs, mutation = false } = {}) {
    aborted(signal);
    if (this.closed) return Promise.reject(new RuntimeError('cli_exited', 'CLI 连接已关闭。', '重新调用。'));
    if (Buffer.byteLength(JSON.stringify(params)) > MAX_PARAMS) return Promise.reject(new RuntimeError('arguments_too_large', '参数超过 240 KiB。', '缩小单次调用范围。'));
    if (this.pending.size >= 32) return Promise.reject(new RuntimeError('runtime_busy', 'CLI 在途请求已满。', '等待当前调用结束。'));
    const id = String(++this.nextID);
    return new Promise((resolve, reject) => {
      let timer;
      const finish = (error, result) => {
        if (!this.pending.delete(id)) return;
        clearTimeout(timer);
        signal?.removeEventListener('abort', cancel);
        error ? reject(error) : resolve(result);
      };
      const stop = (code, message) => {
        if (!this.pending.has(id)) return;
        this.ignored.add(id);
        if (this.ignored.size > 128) this.ignored.delete(this.ignored.values().next().value);
        try { this.send({ jsonrpc: '2.0', method: 'notifications/cancelled', params: { requestId: id, reason: code } }); } catch {}
        finish(new RuntimeError(mutation ? 'result_unknown' : code, message,
          mutation ? '写入结果未知；先重读目标状态，不自动重放。' : '仅此请求被取消；可重新执行读取。'));
      };
      const cancel = () => stop('request_cancelled', '调用已取消。');
      this.pending.set(id, { finish });
      signal?.addEventListener('abort', cancel, { once: true });
      timer = setTimeout(() => stop('request_timeout', 'CLI 请求超时。'), timeoutMs);
      try { this.send({ jsonrpc: '2.0', id, method, params }); } catch (error) { finish(error); }
    });
  }

  async initialize() {
    const result = await this.request('initialize', {
      protocolVersion: '2025-03-26', capabilities: {}, clientInfo: { name: 'lumi-cindy', version: '0.1.0' },
    });
    if (!['2025-03-26', '2025-06-18', '2024-11-05'].includes(result?.protocolVersion)
      || result.serverInfo?.name !== 'lumi-text' || result.serverInfo.version !== '1.0.0' || !result.capabilities?.tools) {
      this.fail(new RuntimeError('cli_incompatible', 'CLI MCP 版本或能力不兼容。', '检查插件包版本；不会自动停止已有服务。'));
      throw new RuntimeError('cli_incompatible', 'CLI MCP 版本或能力不兼容。', '检查插件包版本。');
    }
    this.send({ jsonrpc: '2.0', method: 'notifications/initialized' });
    const listed = await this.request('tools/list');
    if (!Array.isArray(listed?.tools) || !listed.tools.some(tool => tool.name === 'project_overview')) {
      this.fail(protocolError());
      throw protocolError();
    }
    this.info = { protocol_version: result.protocolVersion, server_name: result.serverInfo.name, server_version: result.serverInfo.version };
    return this.info;
  }

  async overview(signal) {
    const result = await this.request('tools/call', { name: 'project_overview', arguments: {} }, { signal });
    let value = result?.structuredContent;
    if (!value && result?.content?.length === 1 && result.content[0].type === 'text') {
      try { value = JSON.parse(result.content[0].text); } catch { throw protocolError(); }
    }
    if (!validEnvelope(value) || (result.isError === true) !== !value.success) throw protocolError();
    return value;
  }

  fail(error) {
    if (this.closed) return;
    this.closed = true;
    this.buffer = Buffer.alloc(0);
    for (const request of this.pending.values()) request.finish(error);
    this.child.stdin.end();
    const term = setTimeout(() => this.child.kill('SIGTERM'), 500);
    const kill = setTimeout(() => this.child.kill('SIGKILL'), 1500);
    term.unref(); kill.unref();
    this.exited.then(() => { clearTimeout(term); clearTimeout(kill); });
    this.onClose(this);
  }
  async close() {
    this.fail(new RuntimeError('connection_released', '连接已释放。', '由新的工具调用重新建立连接。'));
    await this.exited;
  }
}
module.exports = { MCPConnection, MAX_PARAMS, MAX_LINE };
