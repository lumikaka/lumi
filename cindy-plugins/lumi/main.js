/* Browser logic only. Host alone mints args.session_context and callId. */
(() => {
  'use strict';
  const argumentsByMethod = { list_tools: ['category'], call_tool: ['name', 'args'] };
  const schemas = {
    list_tools: { type: 'object', properties: { category: { type: 'string' } }, additionalProperties: false },
    call_tool: { type: 'object', properties: { name: { type: 'string' }, args: { type: 'object' } }, required: ['name'], additionalProperties: false },
  };
  const fail = (code, message, details = '') => ({ success: false, data: null, error: { code, message, details } });
  cindy.onHostMessage(async msg => {
    if (msg.type !== 'tool-call') return; // No page/focus/event may reopen a binding.
    let result;
    try {
      const args = msg.args;
      const context = args?.session_context;
      const allowed = Object.hasOwn(argumentsByMethod, msg.tool) ? argumentsByMethod[msg.tool] : null;
      if (!allowed || !args || typeof args !== 'object' || Array.isArray(args)
        || Object.keys(args).some(key => key !== 'session_context' && !allowed.includes(key))) {
        result = fail('invalid_arguments', '工具参数无效。', '不能指定路径、任务或项目绑定；使用当前任务的宿主上下文。');
        result.correction = { input_schema: allowed ? schemas[msg.tool] : schemas.call_tool,
          rules: ['顶层仅 list_tools / call_tool；先发现类别，再以 name 和 args 调用允许工具。', 'session_context 由 Host 注入，不能用业务参数选择目录或项目。'],
          discovery: { tool: 'list_tools', args: {} } };
      } else if (msg.tool === 'list_tools' && typeof msg.callId === 'string' && msg.callId) {
        const response = await cindy.node.request({ method: 'list_tools', params: { arguments: Object.fromEntries(allowed.filter(k => Object.hasOwn(args, k)).map(k => [k, args[k]])) }, callId: msg.callId, cancelWithCall: true, timeoutMs: 60000 });
        result = response?.ok ? response.result : fail('host_node_failed', '目录调用失败。', '检查插件运行状态。');
      } else if (typeof msg.callId !== 'string' || !msg.callId || typeof context?.session_id !== 'string'
        || !context.session_id
        || typeof context.workdir_is_read_only !== 'boolean') {
        result = fail('context_missing', '缺少可信任务上下文。', '在有本地工作目录的 Cindy 任务中调用。');
      } else if (context.workdir_is_local !== true) {
        result = fail('context_not_local', '当前任务不是本机目录。', '在执行插件的电脑上选择本地项目任务。');
      } else if (typeof context.workdir !== 'string' || !context.workdir) {
        result = fail('workdir_required', '当前任务尚未选择项目目录。', '在 Cindy 为当前任务选择本机项目根目录，再调用 Lumi；不会在插件安装目录创建作品。');
      } else if (!context.workdir.startsWith('/') || context.workdir.startsWith('//') || context.workdir.includes('\u0000')) {
        result = fail('workdir_invalid', '当前任务目录不是本机绝对路径。', '在 Cindy 重新选择本机项目根目录，不接受远程地址或 URI。');
      } else {
        // Host provides the permission snapshot for this call. No page, KV or
        // model field may select it. Realpath/access checks run inside Node.
        const input = Object.fromEntries(allowed.filter(key => Object.hasOwn(args, key)).map(key => [key, args[key]]));
        const response = await cindy.node.request({
          method: msg.tool,
          params: { context: {
            session_id: context.session_id, workdir: context.workdir,
            workdir_is_local: true, workdir_is_read_only: context.workdir_is_read_only,
          }, ...(allowed.length ? { arguments: input } : {}) },
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
    // A Lumi business failure must also be a failed Host tool receipt.
    if (result?.success === true) {
      await cindy.send({ type: 'tool-result', callId: msg.callId, ok: true, result });
    } else {
      result ||= fail('protocol_error', '运行时结果无效。', '重新发现工具目录。');
      const correction = result.correction ? '\n调用修正规则与 schema：\n' + JSON.stringify(result.correction) : '';
      const code = result.error?.code || 'protocol_error';
      await cindy.send({ type: 'tool-result', callId: msg.callId, ok: false,
        errorCode: 'LUMI_' + code.toUpperCase().replace(/[^A-Z0-9_]/g, '_').slice(0, 59),
        message: `Lumi ${code}：` + [result.error?.message, result.error?.details].filter(Boolean).join(' ') + correction, result });
    }
  });
})();
