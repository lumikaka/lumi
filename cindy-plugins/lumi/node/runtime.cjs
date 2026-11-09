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
const { MediaTransfers, prepareRecovery } = require('./media.cjs');
const { ReferenceTransfers } = require('./reference.cjs');
const { adaptAuthoringResult } = require('./authoring.cjs');
const { workspaceResult, methods: workspaceMethods } = require('./workspace.cjs');

const METHODS = new Set([...catalog.allowed, 'list_tools', 'call_tool']);
const MEDIA_METHODS = new Set(['media_begin', 'media_chunk', 'media_commit', 'media_abort']);
const REFERENCE_METHODS = new Set(['reference_prepare', 'reference_chunk', 'reference_release']);
const MEDIA_IMPORT = Symbol('trusted media import');
const WORKSPACE_ACCESS = Symbol('internal workspace adapter');
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
    connectionFactory = options => new MCPConnection(options), env, verifyBinary = true, statusRunner = runCLI } = {}) {
    this.executable = executable;
    this.build = build;
    this.dataDir = dataDir;
    this.environment = environment;
    this.idleMs = idleMs;
    this.bindingMs = bindingMs;
    this.connectionFactory = connectionFactory;
    this.verifyBinary = verifyBinary;
    this.statusRunner = statusRunner;
    this.env = env || { HOME: os.homedir(), PATH: '/usr/bin:/bin', TMPDIR: os.tmpdir(), LANG: 'en_US.UTF-8' };
    this.sessions = new Map();
    this.connections = new Map();
    this.closing = new Set();
    this.closed = false;
    this.media = new MediaTransfers({ namespace: JSON.stringify([executable, dataDir, environment]) });
    this.references = new ReferenceTransfers({ tempDir: this.env.TMPDIR || os.tmpdir() });
    this.mediaStarting = 0;
    this.mediaPending = new Set();
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
    let value;
    for (let attempt = 0; ; attempt++) {
      try {
        value = await this.statusRunner(this.executable,
          ['status', '--environment', this.environment, '--data-dir', dataDir, '--json', '--timeout', '4s', ...(readOnly ? ['--read-only'] : [])],
          { cwd: path.dirname(this.executable), env: this.env, signal });
        break;
      } catch (error) {
        if (error.code !== 'instance_unavailable' || attempt >= 3 || signal?.aborted) throw error;
        // Two startup status probes can briefly contend for the lifetime lock
        // before an instance exists. Retry only this bounded identity read,
        // never a business request, mutation or service start/stop.
        await abortable(new Promise(resolve => setTimeout(resolve, 25 * 2 ** attempt)), signal || new AbortController().signal);
      }
    }
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
    for (const owner of session.mediaOwners?.values() || []) this.media.revoke(owner);
    session.mediaOwners?.clear();
    for (const owner of session.referenceOwners?.values() || []) this.references.revoke(owner);
    session.referenceOwners?.clear();
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

  // Not dispatched by worker-transport/main.js. Only a verified sender adapter
  // may use this internal interface; production currently has no such adapter.
  workspace(method, args, context, project, signal) {
    if (!workspaceMethods.has(method) || !UUID.test(project)) throw new RuntimeError('workspace_method_forbidden', '工作区目标或操作无效。');
    if (method === 'write_app_resource' && /^(?:asset-uploads|comic-exports)(?:\/|$)|generations|generation-batches|reconstructions|\/retries$/.test(args?.resource || '')) throw new RuntimeError('workspace_capability_unavailable', '此 Cindy 页面尚未接入文件、生成或导出操作。', '请通过当前任务对话及授权附件继续，未提交此操作。');
    const error = catalog.check(catalog.specs[method].input_schema, { ...args, ...(method === 'watch_app_changes' ? { project_uuid: project } : {}) });
    if (error) throw new RuntimeError('invalid_arguments', error);
    return this.execute(method, { context, arguments: { ...args, ...(method === 'watch_app_changes' ? { project_uuid: project } : {}) } }, signal, WORKSPACE_ACCESS, project);
  }

  async execute(method, params, signal, token, pageProject) {
    if (MEDIA_METHODS.has(method) || REFERENCE_METHODS.has(method)) {
      const pending = REFERENCE_METHODS.has(method) ? this.executeReference(method, params, signal) : this.executeMedia(method, params, signal);
      this.mediaPending.add(pending);
      try { return await pending; } finally { this.mediaPending.delete(pending); }
    }
    const importingMedia = token === MEDIA_IMPORT && (method === 'import_image' || (method === 'setting_boards' && params?.arguments?.action === 'import'));
    const workspace = token === WORKSPACE_ACCESS && workspaceMethods.has(method);
    if (!workspace && !importingMedia && !METHODS.has(method)) throw new RuntimeError('method_not_found', '运行时方法不存在。', '使用插件声明的工具。');
    if (method === 'list_tools') return success(catalog.discover(params?.arguments ?? {}));
    if (method === 'call_tool') {
      const wrapper = catalog.validateCall(params?.arguments);
      method = wrapper.name;
      params = { ...params, arguments: wrapper.args ?? {} };
    }
    if (method === 'receive_media') throw new RuntimeError('media_handoff_required', '图片必须通过顶层 receive_media 交接。', '使用 ghost_call.tool=receive_media，并在顶层 attachments 传恰好一个授权图片引用。');
    const input = importingMedia || workspace ? params.arguments : catalog.validate(method, params?.arguments ?? {});
    // Reject the actual MCP parameter budget before acquiring a connection or
    // marking any mutation sent. Oversize input is a known non-write outcome.
    if (Buffer.byteLength(JSON.stringify({ name: method, arguments: input,
      _meta: { 'lumi/expected_project_uuid': '00000000-0000-7000-8000-000000000000' } })) > MAX_PARAMS) {
      const error = new RuntimeError('arguments_too_large', 'MCP 参数超过 240 KiB。', '缩小单次参数；完整正文不能静默截断，必要时调整作品拆分。');
      error.correction = catalog.repair(method); throw error;
    }
    const mutation = importingMedia || (workspace ? method === 'write_app_resource' : catalog.isWrite(method, input));
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
          args: ['mcp', '--host', 'cindy', '--environment', this.environment, '--data-dir', dataDir, '--timeout', '25s', ...(context.workdir_is_read_only ? ['--read-only'] : [])],
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
      const serverCompatible = before?.running ? Number.isSafeInteger(before.creation_protocol_version) && before.creation_protocol_version >= 1 : null;
      if (record.identity && before && (!before.running || !sameIdentity(record.identity, before))) {
        // MCP is a stateless CLI relay; rediscovery occurs on every call. Never
        // reuse a cached endpoint/nonce or infer permission from an old binding.
        record.identity = null;
      }
      if (method === 'runtime_status') {
        await record.peer.request('ping', {}, { signal: callSignal });
        record.identity = before?.running ? before : null;
        return success({ runtime: serverCompatible === false ? 'upgrade_required' : 'ready', ...record.peer.info, cli_commit: this.build.source_commit,
          channel: this.build.channel || this.environment, build_id: this.build.build_id || null,
          cli_sha256: this.build.cli_sha256, environment: this.environment,
          platform: this.build.platform, arch: this.build.arch, read_only: context.workdir_is_read_only,
          server_running: before?.running ?? null, server_observation: before ? 'verified' : 'not_checked_read_only',
          server_compatible: serverCompatible, server_creation_protocol_version: before?.creation_protocol_version ?? null,
          server_policy: 'shared_retained' });
      }
      if (serverCompatible === false) {
        throw new RuntimeError('cli_upgrade_required', '正在运行的共享 CLI 服务协议过旧。',
          '未派发创作请求。先协调同一 environment/data-dir 的使用者，再明确停止旧服务并用新包重连；保留现有 draft，先重读事实，不自动重放写入。');
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
      if (workspace && expectedUUID !== pageProject) throw new RuntimeError('app_project_mismatch', '页面授权项目与当前任务绑定不一致。');
      mutationSent = mutation;
      const result = method === 'project_overview'
        ? await record.peer.overview(callSignal, expectedUUID)
        : workspace ? workspaceResult(await record.peer.request('tools/call', { name: method, arguments: input, _meta: { 'lumi/expected_project_uuid': expectedUUID } }, {
          signal: callSignal, mutation, ...(method === 'watch_app_changes' && input.action === 'next' ? { timeoutMs: 45000 } : {}),
        }), method)
        : await record.peer.call(method, input, { signal: callSignal, expectedProjectUUID: expectedUUID, mutation });
      aborted(callSignal);
      if (!result.success) {
        mutationSent = false;
        if (result.error.code === 'project_identity_mismatch') this.forget(session);
        if (mutation && ['instance_unavailable', 'result_too_large'].includes(result.error.code)) {
          throw new RuntimeError('result_unknown', '写入结果尚未确认。', '先重读目标与作品内容，再决定是否重试；禁止自动重放。');
        }
        return { ...(['project_init', 'project_overview', 'project_setup'].includes(method) ? publicFailure(result) : result), correction: catalog.repair(importingMedia ? 'receive_media' : method) };
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
      if (!['project_init', 'project_overview'].includes(method)) return adaptAuthoringResult(method, input, result);
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

  async executeReference(method, params, signal) {
    const fields = method === 'reference_prepare' ? ['context', 'invocation', 'input']
      : method === 'reference_chunk' ? ['context', 'invocation', 'transfer', 'index'] : ['context', 'invocation', 'transfer'];
    if (!params || Object.keys(params).some(k => !fields.includes(k)) || typeof params.invocation !== 'string'
      || !params.invocation || params.invocation.length > 256) throw new RuntimeError('invalid_arguments', '参考交接参数无效。');
    const context = contextFor({ context: params.context });
    const invocation = crypto.createHash('sha256').update(params.invocation).digest('hex');
    let session = this.sessions.get(context.session_id);
    if (session && Object.keys(context).some(k => context[k] !== session.permission[k])) {
      this.forget(session);
      throw new RuntimeError('binding_changed', '参考交接期间任务或权限已改变。', '重新读取并准备当前项目。');
    }
    if (this.closed) throw new RuntimeError('runtime_closed', '运行时已释放。');
    let owner;
    const validate = async () => {
      aborted(signal);
      if (!session || session.revoked || this.sessions.get(session.id) !== session
        || Object.keys(context).some(k => context[k] !== session.permission[k])) throw new RuntimeError('reference_transfer_invalid', '当前任务参考交接已撤销。');
      const directory = await this.directory(context.workdir);
      if (directory.root !== session.root || directory.stamp !== session.directoryStamp
        || await this.projectFile(directory.root) !== session.projectFile) {
        this.forget(session);
        throw new RuntimeError('binding_changed', '参考交接期间项目发生变化。');
      }
      aborted(signal);
      if (session.revoked || this.sessions.get(session.id) !== session) throw new RuntimeError('reference_transfer_invalid', '当前任务参考交接已撤销。');
    };
    if (method === 'reference_prepare') {
      // This wrapper is an internal main→worker transport operation, absent
      // from the public tool catalog. Only Go chooses the source image path.
      const result = await this.execute('prepare_page_image', { context, arguments: params.input }, signal);
      if (!result.success || result.data?.reference_mode === 'none') return result;
      session = this.sessions.get(context.session_id);
      await validate();
      session.referenceOwners ||= new Map();
      for (const [key, value] of session.referenceOwners) if (![...this.references.items.values()].some(item => item.owner === value)) session.referenceOwners.delete(key);
      if (session.referenceOwners.has(invocation)) throw new RuntimeError('reference_transfer_invalid', '同一调用已准备参考交接。');
      owner = { session, invocation };
      const transport = await this.references.begin(owner, result.data, signal, validate);
      try {
        aborted(signal);
        if (session.revoked || this.sessions.get(session.id) !== session) throw new RuntimeError('reference_transfer_invalid', '当前任务参考交接已撤销。');
      } catch (error) { this.references.revoke(owner); throw error; }
      session.referenceOwners.set(invocation, owner);
      this.touch(session);
      return { ...result, transport_reference: transport };
    }
    owner = session?.referenceOwners?.get(invocation);
    if (!owner) throw new RuntimeError('reference_transfer_invalid', '参考交接不属于当前任务调用。');
    try {
      await validate(); this.touch(session);
      return success(method === 'reference_chunk' ? this.references.chunk(owner, params) : this.references.release(owner, params.transfer));
    } catch (error) {
      this.references.revoke(owner); session.referenceOwners.delete(invocation); throw error;
    } finally {
      if (method === 'reference_release') session.referenceOwners.delete(invocation);
    }
  }

  async executeMedia(method, params, signal) {
    const fields = {
      media_begin: ['context', 'invocation', 'input', 'bytes', 'sha256', 'mime'],
      media_chunk: ['context', 'invocation', 'transfer', 'index', 'base64'],
      media_commit: ['context', 'invocation', 'transfer'],
      media_abort: ['context', 'invocation', 'transfer'],
    };
    if (!params || Object.keys(params).some(k => !fields[method].includes(k))
      || typeof params.invocation !== 'string' || !params.invocation || params.invocation.length > 256) {
      throw new RuntimeError('invalid_arguments', '媒体桥接参数无效。', '由顶层 receive_media 发起授权附件交接。');
    }
    const context = contextFor({ context: params.context });
    if (this.closed) throw new RuntimeError('runtime_closed', '运行时已释放。', '重新建立任务绑定。');
    if (context.workdir_is_read_only) {
      const prior = this.sessions.get(context.session_id);
      if (prior) this.forget(prior);
      throw new RuntimeError('read_only_context', '只读任务不能导入图片。', '在当前项目的可写任务中交接图片。');
    }
    const invocation = crypto.createHash('sha256').update(params.invocation).digest('hex');
    let session, owner;
    if (method === 'media_begin') {
      catalog.validate('receive_media', params.input);
      if (this.mediaStarting + this.media.items.size >= 2) throw new RuntimeError('media_busy', '媒体接收并发已满。', '等待正在导入的图片完成。');
      this.mediaStarting++;
      try {
        const board = params.input.slot === 'setting_board';
        const target = await this.execute(board ? 'settings' : 'image_target', { context, arguments: board
          ? { action: 'list', limit: 1 }
          : { target_uuid: params.input.target_uuid, slot: params.input.slot } }, signal);
        if (!target.success) return target;
        // Let the atomic business import check revision after transfer. A stale
        // target still gets a durable recovery original instead of losing it.
        session = this.sessions.get(context.session_id);
        if (!session || session.revoked) throw new RuntimeError('binding_changed', '当前任务绑定已释放。', '重新读取当前项目。');
        session.mediaOwners ||= new Map();
        for (const [key, value] of session.mediaOwners) if (![...this.media.items.values()].some(item => item.owner === value)) session.mediaOwners.delete(key);
        if (session.mediaOwners.has(invocation)) throw new RuntimeError('media_transfer_invalid', '同一调用已开始媒体传输。', '不重放 begin；先取消原传输。');
        owner = { session, invocation };
        const result = this.media.begin(owner, params.input, params);
        session.mediaOwners.set(invocation, owner);
        this.touch(session);
        return success(result);
      } finally { this.mediaStarting--; }
    }
    session = this.sessions.get(context.session_id);
    owner = session?.mediaOwners?.get(invocation);
    if (!session || session.revoked || !owner || Object.keys(context).some(k => context[k] !== session.permission[k])) {
      throw new RuntimeError('media_transfer_invalid', '媒体传输不属于当前任务调用。', '只能由原始 receive_media 调用完成，不支持跨任务或 worker 接管。');
    }
    try {
      aborted(signal);
      const directory = await this.directory(context.workdir, true);
      if (directory.root !== session.root || directory.stamp !== session.directoryStamp
        || await this.projectFile(directory.root) !== session.projectFile) {
        this.forget(session);
        throw new RuntimeError('binding_changed', '媒体接收期间项目发生变化。', '重新读取当前任务项目。');
      }
      this.touch(session);
      if (method === 'media_chunk') return success(this.media.chunk(owner, params));
      if (method === 'media_abort') return success(this.media.abort(owner, params.transfer));
      return await this.media.commit(owner, params.transfer, async (file, input) => {
        if (session.revoked || this.sessions.get(session.id) !== session) throw new RuntimeError('binding_changed', '媒体调用绑定已撤销。', '重新读取当前项目。');
        const board = input.slot === 'setting_board';
        const { slot, ...boardInput } = input;
        // Go's existing board replay contract includes the original path.
        // Keep the authorized source at an immutable task-visible hash path so
        // retrying after a worker restart never substitutes a new staging path.
        const source = board ? await prepareRecovery(session.root, file, input) : null;
        const importPath = source ? path.join(session.root, source.relative_path) : file;
        const result = await this.execute(board ? 'setting_boards' : 'import_image', { context, arguments: board
          ? { ...boardInput, action: 'import', path: importPath }
          : { ...input, path: file } }, signal, MEDIA_IMPORT);
        if (!result.success && result.error.code === 'production_conflict') {
          try {
            const current = await this.directory(context.workdir, true);
            if (current.root !== session.root || current.stamp !== session.directoryStamp || session.revoked) throw new Error('binding');
            result.correction = { ...result.correction, recovery_file: await prepareRecovery(session.root, file, input) };
          } catch { result.correction = { ...result.correction, recovery_file: { status: 'failed', instructions: '原图未能在任务目录保留。请保留宿主原附件，先核对目录权限，不自动重新生图。' } }; }
        }
        return result;
      }, signal);
    } catch (error) {
      if (signal?.aborted) this.media.revoke(owner);
      throw error;
    } finally {
      if (method !== 'media_chunk') session.mediaOwners.delete(invocation);
    }
  }

  async close() {
    this.closed = true;
    this.references.close();
    const sessions = [...this.sessions.values()];
    for (const session of sessions) this.forget(session);
    await Promise.all(sessions.flatMap(session => [...session.pending]));
    for (const record of this.connections.values()) this.drop(record);
    await Promise.all([...this.closing]);
    // Media commit also has a short file-staging phase before acquiring an MCP
    // call. Drain it so graceful worker shutdown cannot strand those bytes.
    await Promise.allSettled([...this.mediaPending]);
    this.media.close();
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
