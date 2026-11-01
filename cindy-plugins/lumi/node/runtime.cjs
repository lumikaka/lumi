'use strict';
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const crypto = require('node:crypto');
const { spawn } = require('node:child_process');
const { MCPConnection, MAX_LINE, MAX_PARAMS } = require('./protocol.cjs');
const { RuntimeError, aborted, validEnvelope } = require('./errors.cjs');
const { publicProject, publicFailure, setupState, UUID } = require('./project.cjs');
const catalog = require('./catalog.cjs');
const { prepareExport, exportIdentity } = require('./export.cjs');

const METHODS = new Set([...catalog.allowed, 'list_tools', 'call_tool']);
const success = data => ({ success: true, data });
const sameIdentity = (a, b) => a && b && a.nonce === b.nonce && a.pid === b.pid && a.endpoint === b.endpoint;
function contextFor(params) {
  if (!params || Object.keys(params).some(k => !['context', 'arguments'].includes(k)) || !params.context) throw new RuntimeError('invalid_arguments', '运行时参数无效。', '只能使用宿主任务上下文和声明的业务参数。');
  const c = params.context;
  if (Object.keys(c).some(k => !['session_id', 'workdir', 'workdir_is_local', 'workdir_is_read_only'].includes(k))
    || typeof c.session_id !== 'string' || !c.session_id || c.session_id.length > 200
    || typeof c.workdir !== 'string' || !path.isAbsolute(c.workdir) || c.workdir.startsWith('//') || c.workdir.length > 4096
    || c.workdir.includes('\0') || typeof c.workdir_is_read_only !== 'boolean') {
    throw new RuntimeError('context_missing', '任务上下文缺失或无效。', '在本地项目任务中重新调用。');
  }
  if (c.workdir_is_local !== true) throw new RuntimeError('context_not_local', '拒绝非本机目录。', '在执行插件的电脑上选择本地目录。');
  return c;
}

// Used only with fixed command arrays from this module, never tool arguments.
function runCLI(executable, args, { cwd, env, signal, timeoutMs = 5000 }) {
  aborted(signal);
  return new Promise((resolve, reject) => {
    const child = spawn(executable, args, { cwd, env, shell: false, stdio: ['ignore', 'pipe', 'pipe'] });
    let bytes = Buffer.alloc(0), error;
    const stop = e => { error ||= e; child.kill('SIGTERM'); };
    const cancel = () => stop(new RuntimeError('request_cancelled', '调用已取消。', '其他项目连接不受影响。'));
    const timer = setTimeout(() => stop(new RuntimeError('request_timeout', 'CLI 状态查询超时。', '稍后重新运行诊断。')), timeoutMs);
    const killTimer = setTimeout(() => child.kill('SIGKILL'), timeoutMs + 1000);
    signal?.addEventListener('abort', cancel, { once: true });
    child.stdout.on('data', part => {
      if (bytes.length + part.length > MAX_LINE) stop(new RuntimeError('result_too_large', 'CLI 状态结果过大。', '关闭不再使用的项目后重试。'));
      else bytes = Buffer.concat([bytes, part]);
    });
    child.stderr.on('data', () => {});
    child.on('error', () => { error = new RuntimeError('cli_unavailable', 'CLI 无法执行。', '检查插件包与 CPU 架构。'); });
    child.on('close', code => {
      clearTimeout(timer); clearTimeout(killTimer);
      signal?.removeEventListener('abort', cancel);
      if (error) return reject(error);
      let result;
      try { result = JSON.parse(bytes.toString('utf8')); } catch { return reject(new RuntimeError('protocol_error', 'CLI 状态返回无效 JSON。', '重新运行诊断。')); }
      if (!validEnvelope(result)) return reject(new RuntimeError('protocol_error', 'CLI 状态信封无效。', '检查随包 CLI 版本。'));
      if (!result.success || ![0, 3].includes(code)) return reject(new RuntimeError('instance_unavailable', '共享 CLI 实例暂时不可用。', '保留已有服务；待其恢复后重试诊断。'));
      resolve(result.data);
    });
  });
}

class Runtime {
  constructor({ executable, build, dataDir, environment = 'production', idleMs = 30000, bindingMs = 60000,
    connectionFactory = options => new MCPConnection(options), env, verifyBinary = true } = {}) {
    this.executable = executable;
    this.build = build;
    this.dataDir = dataDir;
    this.environment = environment;
    this.idleMs = idleMs;
    this.bindingMs = bindingMs;
    this.connectionFactory = connectionFactory;
    this.verifyBinary = verifyBinary;
    this.env = env || { HOME: os.homedir(), PATH: '/usr/bin:/bin', TMPDIR: os.tmpdir(), LANG: 'en_US.UTF-8' };
    this.sessions = new Map();
    this.connections = new Map();
    this.closing = new Set();
    this.closed = false;
  }

  async binary() {
    if (this.build.platform !== process.platform || this.build.arch !== process.arch) {
      throw new RuntimeError('unsupported_platform', '该插件包不支持此电脑架构。', '使用为当前平台构建的插件包。');
    }
    try {
      const info = await fs.stat(this.executable);
      if (!info.isFile() || !(info.mode & 0o111)) throw new Error('mode');
      const signature = `${info.dev}:${info.ino}:${info.size}:${info.mtimeMs}:${info.ctimeMs}`;
      if (signature !== this.binarySignature) {
        if (this.verifyBinary && crypto.createHash('sha256').update(await fs.readFile(this.executable)).digest('hex') !== this.build.cli_sha256) throw new Error('digest');
        this.binarySignature = signature;
      }
    } catch {
      throw new RuntimeError('cli_missing', '随包 CLI 缺失、已改变或不可执行。', '重新构建完整插件包；不要下载运行时代码。');
    }
  }

  async directory(root, writable = false) {
    try {
      const real = await fs.realpath(root), stat = await fs.stat(real);
      if (!stat.isDirectory()) throw new Error('directory');
      await fs.access(real, fs.constants.R_OK | fs.constants.X_OK | (writable ? fs.constants.W_OK : 0));
      return { root: real, stamp: `${stat.dev}:${stat.ino}` };
    } catch { throw new RuntimeError('workdir_unavailable', '任务目录不可用。', '核对当前任务的本地工作目录。'); }
  }

  async projectFile(root) {
    try {
      const stat = await fs.lstat(path.join(root, 'project.sqlite'));
      if (!stat.isFile()) throw new RuntimeError('project_identity_mismatch', '项目数据库不是普通文件。', '不要使用指向其他项目的数据库链接。');
      return `${stat.dev}:${stat.ino}`;
    } catch (error) { if (error.code === 'ENOENT') return null; throw error; }
  }

  async dataDirectory() {
    try { return await fs.realpath(this.dataDir); }
    catch (error) {
      if (error.code !== 'ENOENT') throw new RuntimeError('instance_unavailable', 'CLI 数据目录不可用。', '检查本机目录权限。');
      return path.join(await fs.realpath(path.dirname(this.dataDir)), path.basename(this.dataDir));
    }
  }

  async inspect(dataDir, signal, readOnly = false) {
    const value = await runCLI(this.executable,
      ['status', '--environment', this.environment, '--data-dir', dataDir, '--json', '--timeout', '4s', ...(readOnly ? ['--read-only'] : [])],
      { cwd: path.dirname(this.executable), env: this.env, signal });
    if (typeof value?.running !== 'boolean' || value.environment !== this.environment
      || (value.running && (!Number.isSafeInteger(value.pid) || value.pid <= 0
        || !/^http:\/\/127\.0\.0\.1:\d+$/.test(value.endpoint) || !/^[a-f0-9]{64}$/.test(value.nonce)))) {
      throw new RuntimeError('protocol_error', 'CLI 实例身份无效。', '检查服务发现信息；不会自动停止或删除未知实例。');
    }
    return value;
  }

  drop(record) {
    if (this.connections.get(record.key) === record) this.connections.delete(record.key);
    clearTimeout(record.timer);
    const closing = record.peer.close();
    this.closing.add(closing);
    closing.finally(() => this.closing.delete(closing));
  }

  idle(record) {
    clearTimeout(record.timer);
    if (record.active === 0) {
      record.timer = setTimeout(() => this.drop(record), this.idleMs);
      record.timer.unref();
    }
  }

  forget(session) {
    if (this.sessions.get(session.id) === session) this.sessions.delete(session.id);
    clearTimeout(session.timer);
    session.revoked = true;
    for (const controller of session.calls) controller.abort();
    for (const record of this.connections.values()) {
      if (record.sessions.get(session.id) === session) record.sessions.delete(session.id);
      if (!record.sessions.size && !record.active) this.drop(record);
    }
  }

  touch(session) {
    clearTimeout(session.timer);
    if (!session.calls.size && !session.revoked) {
      session.timer = setTimeout(() => this.forget(session), this.bindingMs);
      session.timer.unref();
    }
  }

  async execute(method, params, signal) {
    if (!METHODS.has(method)) throw new RuntimeError('method_not_found', '运行时方法不存在。', '使用插件声明的工具。');
    if (method === 'list_tools') return success(catalog.discover(params?.arguments ?? {}));
    if (method === 'call_tool') {
      const wrapper = catalog.validateCall(params?.arguments);
      method = wrapper.name;
      params = { ...params, arguments: wrapper.args ?? {} };
    }
    const input = catalog.validate(method, params?.arguments ?? {});
    // Reject the actual MCP parameter budget before acquiring a connection or
    // marking any mutation sent. Oversize input is a known non-write outcome.
    if (Buffer.byteLength(JSON.stringify({ name: method, arguments: input,
      _meta: { 'lumi/expected_project_uuid': '00000000-0000-7000-8000-000000000000' } })) > MAX_PARAMS) {
      const error = new RuntimeError('arguments_too_large', 'MCP 参数超过 240 KiB。', '缩小单次参数；完整正文不能静默截断，必要时调整作品拆分。');
      error.correction = catalog.repair(method); throw error;
    }
    const mutation = catalog.isWrite(method, input);
    const context = contextFor(params);
    aborted(signal);
    if (this.closed) throw new RuntimeError('runtime_closed', '运行时已释放。', '发起新的工具调用。');
    if (method === 'runtime_release') {
      const session = this.sessions.get(context.session_id);
      if (session) this.forget(session); // Revoke first; late completions cannot rebind.
      if (session) await Promise.all([...session.pending]);
      await Promise.all([...this.closing]);
      return success({ released: true, server_policy: 'shared_retained' });
    }
    let session = this.sessions.get(context.session_id);
    // A permission downgrade invalidates old in-flight writes as well as the
    // next call. Never use a prior session's writable flag as authorization.
    if (session && session.permission?.workdir_is_read_only !== context.workdir_is_read_only) {
      this.forget(session);
      session = null;
    }
    if (mutation && context.workdir_is_read_only) {
      throw new RuntimeError('read_only_context', '只读任务不能执行此写操作。', '可使用 list/get 读取；finalize、select_image、初始化与导出落盘均需要当前写权限。');
    }
    if (!session) {
      if (this.sessions.size >= 128) throw new RuntimeError('runtime_busy', '任务绑定数量已满。', '释放不再使用的连接或等待空闲回收。');
      session = { id: context.session_id, calls: new Set(), pending: new Set(), revoked: false };
      this.sessions.set(session.id, session);
    }
    session.permission = { ...context };
    clearTimeout(session.timer);
    const controller = new AbortController();
    const cancel = () => controller.abort();
    signal?.addEventListener('abort', cancel, { once: true });
    session.calls.add(controller);
    let finish;
    const done = new Promise(resolve => { finish = resolve; });
    session.pending.add(done);
    const callSignal = controller.signal;
    let record, acquired = false, finishInitialization, mutationSent = false;
    try {
      await this.binary();
      aborted(callSignal);
      const directory = await this.directory(context.workdir, !context.workdir_is_read_only && method !== 'runtime_status');
      const dataDir = await this.dataDirectory();
      aborted(callSignal);
      const key = JSON.stringify([directory.root, directory.stamp, this.environment, dataDir, this.build.cli_sha256, context.workdir_is_read_only]);
      if (session.key && session.key !== key) {
        this.forget(session);
        throw new RuntimeError('binding_changed', '任务目录或 CLI 绑定已改变。', '重新执行工具以确认新的绑定。');
      }
      if (session.revoked || this.sessions.get(session.id) !== session) aborted(AbortSignal.abort());
      session.key = key;
      session.root = directory.root;
      session.directoryStamp = directory.stamp;
      record = this.connections.get(key);
      if (!record || record.peer.closed) {
        if (this.connections.size >= 16) throw new RuntimeError('runtime_busy', '项目连接数量已满。', '释放不再使用的项目连接。');
        record = { key, active: 0, sessions: new Map(), identity: null };
        record.peer = this.connectionFactory({
          executable: this.executable,
          args: ['mcp', '--environment', this.environment, '--data-dir', dataDir, '--timeout', '25s', ...(context.workdir_is_read_only ? ['--read-only'] : [])],
          cwd: directory.root, env: this.env,
          onClose: () => { if (this.connections.get(key) === record) this.connections.delete(key); },
        });
        this.connections.set(key, record);
        record.ready = record.peer.initialize();
        record.ready.catch(() => this.drop(record));
      }
      clearTimeout(record.timer);
      record.active++;
      acquired = true;
      record.sessions.set(session.id, session);
      // Serialize initializations of one root, without blocking other projects
      // or concurrent reads. Cancellation cannot let later initializers jump
      // ahead of an earlier one still running.
      const priorInitialization = record.initializing || Promise.resolve();
      if (method === 'project_init') {
        const own = new Promise(resolve => { finishInitialization = resolve; });
        record.initializing = priorInitialization.then(() => own);
      }
      // Initialization belongs to the connection, not the first requesting task.
      await abortable(record.ready, callSignal);
      await abortable(priorInitialization, callSignal);
      aborted(callSignal);
      const again = await this.directory(context.workdir);
      if (again.root !== directory.root || again.stamp !== directory.stamp) {
        this.forget(session);
        throw new RuntimeError('binding_changed', '任务目录在调用期间发生变化。', '释放连接后重新执行工具。');
      }
      // CLI status may create its data-dir lock when no daemon is running. A
      // read-only workdir can contain that data-dir (e.g. HOME), so only perform
      // the side-effect-free MCP handshake/ping in read-only diagnostics.
      const before = context.workdir_is_read_only && method === 'runtime_status' ? null : await this.inspect(dataDir, callSignal, context.workdir_is_read_only);
      if (record.identity && before && (!before.running || !sameIdentity(record.identity, before))) {
        // MCP is a stateless CLI relay; rediscovery occurs on every call. Never
        // reuse a cached endpoint/nonce or infer permission from an old binding.
        record.identity = null;
      }
      if (method === 'runtime_status') {
        await record.peer.request('ping', {}, { signal: callSignal });
        record.identity = before?.running ? before : null;
        return success({ runtime: 'ready', ...record.peer.info, cli_commit: this.build.source_commit,
          platform: this.build.platform, arch: this.build.arch, read_only: context.workdir_is_read_only,
          server_running: before?.running ?? null, server_observation: before ? 'verified' : 'not_checked_read_only',
          server_policy: 'shared_retained' });
      }
      const verifyDirectory = async () => {
        const current = await this.directory(context.workdir, !context.workdir_is_read_only);
        if (current.root !== directory.root || current.stamp !== directory.stamp) {
          this.forget(session);
          throw new RuntimeError('binding_changed', '任务目录在调用期间发生变化。', '重新读取当前任务项目。');
        }
      };
      await verifyDirectory();
      const file = await this.projectFile(directory.root);
      if ((session.projectFile && session.projectFile !== file) || (record.projectFile && record.projectFile !== file)
        || (session.projectUUID && record.projectUUID && session.projectUUID !== record.projectUUID)) {
        this.forget(session);
        throw new RuntimeError('binding_changed', '项目文件或身份已改变。', '重新读取当前任务项目，不沿用旧绑定。');
      }
      let expectedUUID = session.projectUUID || record.projectUUID;
      const bind = result => {
        aborted(callSignal);
        const uuid = result.data?.project?.uuid;
        if (!UUID.test(uuid)) throw new RuntimeError('protocol_error', 'CLI 未返回有效项目 UUIDv7。', '检查 CLI 版本。');
        if (expectedUUID && expectedUUID !== uuid) {
          this.forget(session);
          throw new RuntimeError('binding_changed', '当前目录的项目身份已改变。', '重新读取当前任务项目。');
        }
        expectedUUID = uuid;
        session.projectUUID = record.projectUUID = uuid;
      };
      // Setup has no project identity in its arguments. First bind from a
      // trusted cwd read, then send that UUID as an additional CLI constraint.
      if (!['project_init', 'project_overview'].includes(method)) {
        const current = await record.peer.overview(callSignal, expectedUUID);
        if (!current.success) {
          if (current.error.code === 'project_identity_mismatch') this.forget(session);
          return publicFailure(current);
        }
        bind(current);
        await verifyDirectory();
        if (await this.projectFile(directory.root) !== file) {
          this.forget(session);
          throw new RuntimeError('binding_changed', '项目文件在调用期间发生变化。', '重新读取项目。');
        }
      }
      // The shared renderer filters context lists, so explicitly reject UUIDs
      // outside this project rather than silently dropping them in Cindy.
      if (method === 'render_prompt') {
        const chapterUUIDs = new Set([...(input.context_chapter_uuids || []), ...(input.chapter_uuid ? [input.chapter_uuid] : [])]);
        const settingUUIDs = new Set([...(input.context_setting_uuids || []), ...(input.setting_uuid ? [input.setting_uuid] : [])]);
        const checks = [...chapterUUIDs].map(chapter_uuid => ['chapters', { action: 'get', chapter_uuid }]);
        checks.push(...[...settingUUIDs].map(setting_uuid => ['settings', { action: 'get', setting_uuid }]));
        if (input.page_uuid) {
          if (!input.chapter_uuid) {
            const e = new RuntimeError('invalid_arguments', 'page_uuid 必须同时提供 chapter_uuid。', '按目标资源读取并重新渲染任务。');
            e.correction = catalog.repair(method); throw e;
          }
          checks.push(['pages', { action: 'get', chapter_uuid: input.chapter_uuid, page_uuid: input.page_uuid }]);
        }
        for (const [tool, args] of checks) {
          const checked = await record.peer.call(tool, args, { signal: callSignal, expectedProjectUUID: expectedUUID });
          if (!checked.success) return { ...checked, correction: catalog.repair(method) };
        }
      }
      mutationSent = mutation;
      const result = method === 'project_overview'
        ? await record.peer.overview(callSignal, expectedUUID)
        : await record.peer.call(method, input, { signal: callSignal, expectedProjectUUID: expectedUUID, mutation });
      aborted(callSignal);
      if (!result.success) {
        mutationSent = false;
        if (result.error.code === 'project_identity_mismatch') this.forget(session);
        if (mutation && ['instance_unavailable', 'result_too_large'].includes(result.error.code)) {
          throw new RuntimeError('result_unknown', '写入结果尚未确认。', '先重读目标与作品内容，再决定是否重试；禁止自动重放。');
        }
        return { ...(['project_init', 'project_overview', 'project_setup'].includes(method) ? publicFailure(result) : result), correction: catalog.repair(method) };
      }
      await verifyDirectory();
      const afterFile = await this.projectFile(directory.root);
      if (file && file !== afterFile) {
        this.forget(session);
        throw new RuntimeError('binding_changed', '项目文件在调用期间发生变化。', '重新读取项目。');
      }
      const after = await this.inspect(dataDir, callSignal, context.workdir_is_read_only);
      aborted(callSignal);
      if (!after.running || (before.running && !sameIdentity(before, after))) {
        throw new RuntimeError('instance_changed', '调用期间共享 CLI 实例已更换。', '重新读取项目状态，不自动重放写操作。');
      }
      record.identity = after;
      session.instance = { ...after };
      record.provenance = before.running ? 'observed_existing_shared' : 'first_observed_shared';
      session.projectFile = record.projectFile = afterFile;
      if (method === 'project_setup') {
        return success({ ...setupState(result.data, expectedUUID), limitations: { read_only: context.workdir_is_read_only, open_may_write: !context.workdir_is_read_only } });
      }
      if (method === 'export_book') {
        // Validate the original receipt before reporting any generated file.
        const identity = exportIdentity(directory.root, result.data);
        try {
          const prepared = await prepareExport(directory.root, result.data, callSignal);
          await verifyDirectory();
          return success(prepared);
        } catch (error) {
          aborted(callSignal);
          mutationSent = false; // Generation is known; delivery preparation failed.
          const failed = new RuntimeError('export_delivery_failed', '作品文件已生成，但任务文件交付未准备完成。', '保留生成文件；不要为交付失败自动重新导出。核对当前任务文件列表与目录权限。');
          failed.correction = { ...catalog.repair(method), generated_export: {
            uuid: result.data.uuid, format: result.data.format, byte_size: result.data.byte_size,
            file_generated: true, user_received: false,
            project_relative_path: `.lumi/exports/text-${result.data.uuid}.${identity.ext}`,
          }, delivery: { status: 'failed', receipt: null } };
          throw failed;
        }
      }
      if (!['project_init', 'project_overview'].includes(method)) return result;
      bind(result);
      const data = publicProject(result.data);
      data.limitations = { ...data.limitations, read_only: context.workdir_is_read_only, open_may_write: !context.workdir_is_read_only };
      return success(data);
    } catch (error) {
      if (mutationSent) throw new RuntimeError('result_unknown', '项目操作结果尚未确认。', '先调用 project_overview 重读状态，再决定是否重试；不会自动重放写操作。');
      throw error;
    } finally {
      finishInitialization?.();
      signal?.removeEventListener('abort', cancel);
      session.calls.delete(controller);
      this.touch(session);
      if (record && acquired) {
        record.active--;
        if (!record.active && !record.sessions.size) this.drop(record);
        else this.idle(record);
      }
      session.pending.delete(done);
      finish();
    }
  }

  async close() {
    this.closed = true;
    const sessions = [...this.sessions.values()];
    for (const session of sessions) this.forget(session);
    await Promise.all(sessions.flatMap(session => [...session.pending]));
    for (const record of this.connections.values()) this.drop(record);
    await Promise.all([...this.closing]);
  }
}

function abortable(promise, signal) {
  aborted(signal);
  return new Promise((resolve, reject) => {
    const cancel = () => reject(new RuntimeError('request_cancelled', '调用已取消。', '其他任务的初始化不受影响。'));
    signal.addEventListener('abort', cancel, { once: true });
    promise.then(resolve, reject).finally(() => signal.removeEventListener('abort', cancel));
  });
}
module.exports = { Runtime, contextFor, runCLI, METHODS };
