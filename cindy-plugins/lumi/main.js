/* Browser logic only. Host alone mints args.session_context and callId. */
(() => {
  'use strict';
  const methods = new Set(['runtime_status', 'project_overview', 'runtime_release']);
  const fail = (code, message, details = '') => ({ success: false, data: null, error: { code, message, details } });
  cindy.onHostMessage(async msg => {
    if (msg.type !== 'tool-call') return; // No page/focus/event may reopen a binding.
    let result;
    try {
      const args = msg.args;
      const context = args?.session_context;
      if (!methods.has(msg.tool) || !args || Object.keys(args).some(key => key !== 'session_context')) {
        result = fail('invalid_arguments', '工具不接受自定义路径或业务参数。', '使用当前任务的宿主上下文。');
      } else if (typeof msg.callId !== 'string' || !msg.callId || typeof context?.session_id !== 'string'
        || !context.session_id || typeof context.workdir !== 'string' || !context.workdir
        || typeof context.workdir_is_read_only !== 'boolean') {
        result = fail('context_missing', '缺少可信任务上下文。', '在有本地工作目录的 Cindy 任务中调用。');
      } else if (context.workdir_is_local !== true) {
        result = fail('context_not_local', '当前任务不是本机目录。', '在执行插件的电脑上选择本地项目任务。');
      } else if (msg.tool === 'project_overview' && context.workdir_is_read_only) {
        result = fail('read_only_context', '只读任务不能打开 Lumi 项目。', '现有 CLI 打开项目可能迁移数据库或写入运行文件；可使用 runtime_status。');
      } else {
        const response = await cindy.node.request({
          method: msg.tool,
          params: { context: {
            session_id: context.session_id, workdir: context.workdir,
            workdir_is_local: true, workdir_is_read_only: context.workdir_is_read_only,
          } },
          callId: msg.callId, cancelWithCall: true, timeoutMs: 60000,
        });
        if (!response?.ok) {
          result = fail('host_node_failed', '宿主未完成 Node 调用。', '检查插件权限或重新执行诊断；写操作结果未知时先重读，不自动重试。');
        } else if (typeof response.result?.success !== 'boolean' || !Object.hasOwn(response.result, 'data')
          || (!response.result.success && (response.result.data !== null
            || typeof response.result.error?.code !== 'string'
            || typeof response.result.error?.message !== 'string'
            || typeof response.result.error?.details !== 'string'))) {
          result = fail('protocol_error', 'Node 返回了无效结果。', '重新执行运行诊断。');
        } else result = response.result;
      }
    } catch {
      result = fail('host_node_failed', '宿主运行时连接失败。', '检查插件状态后重新执行诊断；不要自动重放写操作。');
    }
    // Business failures remain complete envelopes; transport failures never
    // become success:true. Host's ok is the transport receipt, not Lumi status.
    await cindy.send({ type: 'tool-result', callId: msg.callId, ok: true, result });
  });
})();
