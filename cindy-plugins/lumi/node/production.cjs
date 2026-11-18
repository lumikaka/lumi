'use strict';
const fs = require('node:fs/promises');
const path = require('node:path');
const crypto = require('node:crypto');
const { RuntimeError, aborted } = require('./errors.cjs');
const { UUID } = require('./project.cjs');
const hash = value => crypto.createHash('sha256').update(JSON.stringify(value)).digest('hex');
const problem = (code, message) => { throw new RuntimeError(code, message, '先读现有记录与 Lumi 当前事实；不换键重发未知操作。'); };
const snake = value => Array.isArray(value) ? value.map(snake) : value && typeof value === 'object'
  ? Object.fromEntries(Object.entries(value).map(([k, v]) => [k.replace(/[A-Z]/g, c => '_' + c.toLowerCase()), snake(v)])) : value;
const recordPattern = /^(index\.json|chapters\/([0-9a-f-]{36})\/(chapter|settings|storyboard-commit|scene-[1-9][0-9]{0,4}|storyboard\/(manifest|page-[1-9][0-9]{0,4}))\.json)$/;
// This is a coordination ledger, never a second source of business facts. Its
// root comes exclusively from the current trusted Host directory.
class ProductionStore {
  constructor(root, projectUUID, readOnly = false) { this.root = root; this.projectUUID = projectUUID; this.readOnly = readOnly; }
  async location(name, create) {
    if (name !== 'tasks.json') {
      const match = recordPattern.exec(name);
      if (!match || (match[2] && !UUID.test(match[2]))) problem('invalid_arguments', '制作记录名称无效。');
    }
    const parts = ['user-data', 'lumi-production', ...name.split('/')];
    let dir = this.root;
    for (const part of parts.slice(0, -1)) {
      dir = path.join(dir, part);
      if (create) await fs.mkdir(dir).catch(e => { if (e.code !== 'EEXIST') throw e; });
      try { const stat = await fs.lstat(dir); if (!stat.isDirectory() || stat.isSymbolicLink()) problem('production_path_unsafe', '制作目录不能是符号链接。'); }
      catch (e) { if (e.code === 'ENOENT' && !create) return null; throw e; }
    }
    return path.join(dir, parts.at(-1));
  }
  async load(file) {
    if (!file) return { revision: 0, data: null };
    let handle;
    try {
      handle = await fs.open(file, fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW);
      const stat = await handle.stat();
      if (!stat.isFile() || stat.size > 2 * 1024 * 1024) problem('production_record_invalid', '制作记录过大或不是普通文件。');
      const value = JSON.parse(await handle.readFile('utf8'));
      if (value.project_uuid !== this.projectUUID) problem('project_identity_mismatch', '制作记录属于另一 Lumi 项目。');
      if (!Number.isSafeInteger(value.revision) || value.revision < 1 || !value.data || typeof value.data !== 'object') problem('production_record_invalid', '制作记录内容无效。');
      return value;
    } catch (e) { if (e.code === 'ENOENT') return { revision: 0, data: null }; throw e; }
    finally { await handle?.close(); }
  }
  async read(name) { const value = await this.load(await this.location(name, false)); return { record: name, revision: value.revision, data: value.data }; }
  async change(name, transform, signal) {
    if (this.readOnly) problem('read_only_context', '只读任务不能保存制作记录或派发任务。');
    aborted(signal);
    const file = await this.location(name, true), lock = file + '.lock';
    let handle;
    try { handle = await fs.open(lock, 'wx', 0o600); }
    catch (e) { if (e.code === 'EEXIST') problem('production_record_busy', '记录正在写入或上次写入尚未核实；保留锁并核对原调用。'); throw e; }
    const temporary = file + '.' + crypto.randomBytes(12).toString('hex') + '.tmp';
    try {
      const old = await this.load(file), update = await transform(old);
      if (!update) return old;
      const value = { project_uuid: this.projectUUID, revision: old.revision + 1, ...update };
      const bytes = JSON.stringify(value, null, 2) + '\n';
      if (Buffer.byteLength(bytes) > 2 * 1024 * 1024) problem('production_record_too_large', '制作记录超过 2 MiB，请按章和逐页拆分。');
      aborted(signal);
      const output = await fs.open(temporary, 'wx', 0o600);
      try { await output.writeFile(bytes); await output.sync(); } finally { await output.close(); }
      await fs.rename(temporary, file);
      const dir = await fs.open(path.dirname(file), 'r');
      try { await dir.sync(); } finally { await dir.close(); }
      return value;
    } finally { await fs.rm(temporary, { force: true }); await handle.close(); await fs.unlink(lock); }
  }
  async write(input, signal) {
    const digest = hash([input.expected_revision, input.data]);
    const value = await this.change(input.record, old => {
      if (old.last_write?.request_key === input.request_key) {
        if (old.last_write.digest !== digest) problem('request_key_conflict', '同一记录写入键的参数已经改变。');
        return null;
      }
      if (old.revision !== input.expected_revision) problem('production_revision_conflict', '制作记录已有更新，先读再合并。');
      return { data: input.data, last_write: { request_key: input.request_key, digest } };
    }, signal);
    return { record: input.record, revision: value.revision, data: value.data };
  }
  async task(input, caps, invocation, signal) {
    if (input.action === 'retry_create') return this.retryCreate(input, caps, invocation, signal);
    const ledger = (await this.read('tasks.json')).data || { slots: {}, requests: {} };
    const key = input.request_key;
    if (input.action === 'request_status') return { request: ledger.requests[key] ? this.publicRequest(ledger.requests[key]) : null };
    if (input.action === 'resume_request') {
      const old = ledger.requests[key];
      if (!old) problem('production_request_missing', '没有这个已保存的请求。');
      if (old.status === 'succeeded' || old.status === 'failed') return this.publicRequest(old);
      if (this.readOnly) problem('read_only_context', '只读任务不能续接未决请求。');
      return { host_action: { operation: old.operation, params: old.params, request_key: key, digest: old.digest } };
    }
    const slot = input.chapter_uuid && `${input.chapter_uuid}:${input.role}${input.role === 'scene' ? ':' + input.scene : ''}`;
    if (input.role === 'scene' && !Number.isSafeInteger(input.scene)) problem('invalid_arguments', '场景任务必须指定 scene。');
    if (input.role !== 'scene' && input.scene !== undefined) problem('invalid_arguments', '只有场景任务使用 scene。');
    const operation = { create: 'create', get: 'get', send: 'send', get_run: 'getRun', read_messages: 'readMessages' }[input.action];
    if (!operation || !caps?.operations?.includes(operation)) problem('host_capability_unavailable', '当前 Cindy 未提供该普通任务操作。');
    let params;
    if (operation === 'getRun') {
      const sent = ledger.requests[key];
      if (sent?.operation !== 'send' || !sent.result?.runId) problem('production_request_pending', '发送回执尚未确认，不能猜测运行 ID。');
      params = { runId: sent.result.runId };
    } else if (operation === 'create') {
      // Persisted slot identity prevents a new request key from duplicating a
      // chapter, storyboard, settings or scene task after an uncertain create.
      const prior = ledger.slots[slot];
      if (prior && prior.create_key !== key) return { reused: true, task_key: slot, ...this.publicRequest(ledger.requests[prior.create_key]) };
      params = { requestKey: this.hostKey(key), title: input.title, isolatedWorkspace: true };
      if (caps.sourceCallContext === true) params.callId = invocation;
    } else {
      const created = ledger.requests[ledger.slots[slot]?.create_key];
      if (!created?.result?.taskId) problem('production_task_missing', '本项目该职责的任务尚未确认创建。');
      params = { taskId: created.result.taskId };
      if (operation === 'send') Object.assign(params, { expectedRevision: input.expected_revision, requestKey: this.hostKey(key), text: input.text });
      if (operation === 'readMessages') Object.assign(params, { limit: input.limit || 20, ...(input.after ? { after: input.after } : {}) });
    }
    if (!['create', 'send'].includes(operation)) return { host_action: { operation, params } };
    const logical = hash(input);
    let action;
    await this.change('tasks.json', old => {
      const data = old.data || { slots: {}, requests: {} };
      const existing = data.requests[key];
      if (existing) {
        if (existing.logical !== logical) problem('request_key_conflict', '任务请求键对应的原参数已经改变。');
        action = this.publicRequest(existing); // Explicit resume only; never silently submit again.
        return null;
      }
      if (Object.values(data.requests).some(r => r.task_key === slot && ['pending', 'unknown'].includes(r.status))) problem('production_request_pending', '该职责仍有未决请求，先核对原键；不能另建请求绕过。');
      if (operation === 'create' && data.slots[slot]) problem('production_record_busy', '此职责已由另一调用认领；先读原回执。');
      const request = { operation, params, logical, digest: hash([operation, params]), status: 'pending', task_key: slot };
      data.requests[key] = request;
      if (operation === 'create') data.slots[slot] = { create_key: key };
      action = { host_action: { operation, params, request_key: key, digest: request.digest } };
      return { data };
    }, signal);
    return action;
  }
  async retryCreate(input, caps, invocation, signal) {
    if (this.readOnly) problem('read_only_context', '只读任务不能重新创建职责任务。');
    const nextKey = input.new_request_key;
    if (typeof nextKey !== 'string' || !/^[A-Za-z0-9_.:-]{1,160}$/.test(nextKey) || nextKey === input.request_key) problem('invalid_arguments', '显式重建需要不同的稳定新请求键。');
    if (!caps?.operations?.includes('create')) problem('host_capability_unavailable', '当前 Cindy 未提供任务创建。');
    const logical = hash(input);
    let action;
    await this.change('tasks.json', old => {
      const data = old.data || { slots: {}, requests: {} }, existing = data.requests[nextKey];
      if (existing) {
        if (existing.logical !== logical) problem('request_key_conflict', '新请求键已用于其他原参数。');
        action = this.publicRequest(existing);
        return null;
      }
      const prior = data.requests[input.request_key];
      // Only a known pre-creation permission rejection is replaceable. An
      // unknown, cancelled, conflicting or accepted request retains its slot.
      if (prior?.operation !== 'create' || prior.status !== 'failed' || prior.error?.code !== 'PERMISSION_DENIED'
        || prior.result || data.slots[prior.task_key]?.create_key !== input.request_key) problem('production_retry_forbidden', '只能在用户修复授权并明确要求后，重新创建确定因权限拒绝且尚无任务的职责。');
      const params = { ...prior.params, requestKey: this.hostKey(nextKey) };
      delete params.callId;
      if (caps.sourceCallContext === true) params.callId = invocation;
      const request = { operation: 'create', params, logical, digest: hash(['create', params]), status: 'pending',
        task_key: prior.task_key, previous_request_key: input.request_key };
      data.requests[nextKey] = request;
      data.slots[prior.task_key] = { create_key: nextKey };
      action = { host_action: { operation: 'create', params, request_key: nextKey, digest: request.digest } };
      return { data };
    }, signal);
    return action;
  }
  hostKey(key) { return 'lumi-' + hash([this.root, this.projectUUID, key]); }
  publicRequest(request) {
    return { task_key: request.task_key, operation: request.operation, status: request.status,
      ...(request.previous_request_key ? { previous_request_key: request.previous_request_key } : {}),
      ...(request.result ? { host_result: snake(request.result) } : {}), ...(request.error ? { error: request.error } : {}) };
  }
  async receipt(input, signal) {
    let request;
    await this.change('tasks.json', old => {
      request = old.data?.requests[input.request_key];
      if (!request || request.digest !== input.digest) problem('production_request_mismatch', '任务回执与已保存请求不匹配。');
      if (request.status === 'succeeded' || request.status === 'failed') return null;
      if (input.error) {
        request.status = ['PERMISSION_DENIED', 'INVALID_PARAMS', 'ROUTE_UNAVAILABLE', 'REQUEST_KEY_CONFLICT'].includes(input.error.code) ? 'failed' : 'unknown';
        request.error = { code: input.error.code, message: '宿主任务操作未确认；保留原键和参数。' };
      } else {
        if (request.operation === 'create' && (!input.result?.taskId || typeof input.result.workingDir !== 'string')) problem('production_receipt_invalid', '创建回执缺少任务身份或宿主目录。');
        if (request.operation === 'send' && !input.result?.runId) problem('production_receipt_invalid', '发送回执缺少运行身份。');
        request.status = 'succeeded'; request.result = input.result; delete request.error;
      }
      return { data: old.data };
    }, signal);
    return this.publicRequest(request);
  }
}
module.exports = { ProductionStore, snake };
