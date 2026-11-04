'use strict';
const { RuntimeError, aborted, validEnvelope } = require('./errors.cjs');
const { UUID } = require('./project.cjs');

const methods = new Set(['read_app_resource', 'write_app_resource', 'read_app_image', 'watch_app_changes']);
function workspaceResult(raw, name) {
  const result = name === 'watch_app_changes' ? raw?.structuredContent : raw?._meta?.['lumi/app_response'];
  if (!validEnvelope(result) || (raw.isError === true) !== !result.success) {
    throw new RuntimeError('protocol_error', '工作区响应缺少有效的界面专用数据。', '未改为读取模型文本或返回虚假的成功。');
  }
  return result;
}

// Internal adapter, NOT an RPC entry or a Host API. No production authority is
// installed. A future verified Host connector must resolve its observed sender
// to the SAME live grant object on every request; page JSON cannot mint it.
class WorkspaceGateway {
  constructor({ runtime, authority, sender } = {}) {
    Object.assign(this, { runtime, authority, sender });
    this.controller = new AbortController();
    this.subscriptions = new Set();
  }
  async principal() {
    aborted(this.controller.signal);
    if (!this.authority?.resolve) throw new RuntimeError('workspace_host_identity_unavailable', '当前 Cindy 尚无已核验的页面任务授权通道。', '工作区生产桥保持关闭；可继续使用对话工具和任务文件。');
    const grant = await this.authority.resolve(this.sender);
    if (!grant || !UUID.test(grant.project_uuid) || !grant.signal || grant.signal.aborted) throw new RuntimeError('workspace_revoked', '页面任务授权已失效。', '关闭旧页面并在当前任务重新打开。');
    return grant;
  }
  async call(name, args = {}, { signal, project } = {}) {
    if (!methods.has(name)) throw new RuntimeError('workspace_method_forbidden', '页面不能调用此工具。', '仅允许固定工作区读写、预览和实时订阅。');
    const grant = await this.principal();
    if (project && project !== grant.project_uuid) throw new RuntimeError('app_project_mismatch', '请求不属于当前页面项目。');
    const combined = AbortSignal.any([this.controller.signal, grant.signal, ...(signal ? [signal] : [])]);
    const result = await this.runtime.workspace(name, args, grant.context, grant.project_uuid, combined);
    if (grant !== await this.principal() || combined.aborted) throw new RuntimeError(name === 'write_app_resource' ? 'result_unknown' : 'workspace_revoked', '请求期间页面授权已改变。', '先在当前任务重读事实，不自动重放写操作。');
    if (name === 'watch_app_changes' && result.success) {
      if (args.action === 'open') this.subscriptions.add({ uuid: result.data.subscription_uuid, grant });
      if (args.action === 'close') for (const item of this.subscriptions) if (item.uuid === args.subscription_uuid) this.subscriptions.delete(item);
    }
    return result;
  }
  async close() {
    this.controller.abort();
    await Promise.allSettled([...this.subscriptions].map(item => this.runtime.workspace('watch_app_changes', {
      action: 'close', subscription_uuid: item.uuid,
    }, item.grant.context, item.grant.project_uuid)));
    this.subscriptions.clear();
  }
}

module.exports = { WorkspaceGateway, workspaceResult, methods };
