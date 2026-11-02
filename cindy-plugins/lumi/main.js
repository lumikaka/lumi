/* Browser logic only. Host alone mints args.session_context and callId. */
(() => {
  'use strict';
  const mediaFields = ['target_uuid', 'slot', 'expected_revision', 'fit_mode', 'description', 'prompt'];
  const argumentsByMethod = { list_tools: ['category'], call_tool: ['name', 'args'], receive_media: mediaFields };
  const schemas = {
    list_tools: { type: 'object', properties: { category: { type: 'string' } }, additionalProperties: false },
    call_tool: { type: 'object', properties: { name: { type: 'string' }, args: { type: 'object' } }, required: ['name'], additionalProperties: false },
  };
  schemas.receive_media = {"additionalProperties":false,"properties":{"description":{"maxLength":12000,"type":"string"},"expected_revision":{"description":"Exact revision from the last read or render_prompt; required for every mutation.","minimum":0,"type":"integer"},"fit_mode":{"enum":["keep","crop","pad"],"type":"string"},"prompt":{"maxLength":12000,"type":"string"},"slot":{"enum":["page_image","setting_image"],"type":"string"},"target_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"}},"required":["target_uuid","slot","expected_revision"],"type":"object"};
  const fail = (code, message, details = '') => ({ success: false, data: null, error: { code, message, details } });
  const mediaError = (code, message) => { throw { business: fail(code, message, '保留作品并重读目标；使用顶层 attachments 交接原图，不自动重新生成。') }; };
  let receiving = 0;
  async function receiveMedia(msg, context, input) {
    if (context.workdir_is_read_only) mediaError('read_only_context', '只读任务不能接收图片。');
    if (!Array.isArray(msg.args.attachments) || msg.args.attachments.length !== 1) mediaError('media_attachment_required', '请在 ghost_call.attachments 交接恰好一张图片。');
    const value = msg.args.attachments[0];
    const token = typeof value === 'string' && /^(?:cindy-media:\/\/blobs\/)?([a-f0-9]{64})(\.(?:png|jpg|jpeg|webp))?$/.exec(value);
    if (!token) mediaError('media_attachment_invalid', '宿主未提供有效的图片附件指纹。');
    if (receiving >= 2) mediaError('media_busy', '最多同时导入两张图片。');
    receiving++;
    const controller = new AbortController(), deadline = setTimeout(() => controller.abort(), 90000);
    let transfer;
    const node = async (method, extra = {}) => {
      const response = await cindy.node.request({ method, params: { context, invocation: msg.callId, ...extra },
        callId: msg.callId, cancelWithCall: true, timeoutMs: 60000 });
      if (!response?.ok) mediaError('host_node_failed', '宿主未完成图片传输；写入结果未知时先重读目标。');
      if (response.result?.success !== true) throw { business: response.result || fail('protocol_error', '媒体响应无效。') };
      return response.result;
    };
    try {
      // The Host grants only these exact attachment hashes. Bare hashes have no
      // extension; try the finite documented image suffixes on the owned route.
      let response;
      for (const suffix of token[2] ? [token[2]] : ['.png', '.jpg', '.jpeg', '.webp']) {
        response = await fetch(`/media/${token[1]}${suffix}`, { signal: controller.signal });
        if (response.ok) break;
        if (response.status !== 404) mediaError('media_access_denied', '宿主拒绝读取授权图片。');
      }
      if (!response?.ok) mediaError('media_unavailable', '授权图片不存在、已过期或格式不支持。');
      const declared = Number(response.headers.get('content-length'));
      if (declared > 20 * 1024 * 1024) mediaError('media_too_large', '单张图片最多 20 MiB。');
      const reader = response.body.getReader(), chunks = [];
      let length = 0;
      try {
        for (;;) {
          const { done, value: bytes } = await reader.read(); if (done) break;
          length += bytes.length;
          if (length > 20 * 1024 * 1024) mediaError('media_too_large', '单张图片最多 20 MiB。');
          chunks.push(bytes);
        }
      } finally { await reader.cancel().catch(() => {}); }
      if (!length || (declared > 0 && declared !== length)) mediaError('media_incomplete', '宿主返回的原图字节不完整。');
      const bytes = new Uint8Array(length); let offset = 0;
      for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.length; }
      const ascii = (from, to) => String.fromCharCode(...bytes.subarray(from, to));
      const mime = [137,80,78,71,13,10,26,10].every((b, i) => bytes[i] === b) ? 'image/png'
        : bytes[0] === 255 && bytes[1] === 216 && bytes[2] === 255 ? 'image/jpeg'
        : ascii(0, 4) === 'RIFF' && ascii(8, 12) === 'WEBP' ? 'image/webp' : null;
      const declaredMime = response.headers.get('content-type')?.split(';')[0].trim().toLowerCase();
      if (!mime || (declaredMime && declaredMime !== 'application/octet-stream' && declaredMime !== mime)) mediaError('media_format_mismatch', '图片真实格式与宿主 MIME 不一致。');
      const sha256 = [...new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))].map(b => b.toString(16).padStart(2, '0')).join('');
      if (sha256 !== token[1]) mediaError('media_checksum_mismatch', '图片原件摘要与授权指纹不一致。');
      const begin = await node('media_begin', { input, bytes: length, sha256, mime });
      transfer = begin.data.transfer;
      for (let pos = 0, index = 0; pos < length; pos += 48 * 1024, index++) {
        if (controller.signal.aborted) mediaError('media_timeout', '图片传输已超时。');
        let binary = '';
        for (const byte of bytes.subarray(pos, pos + 48 * 1024)) binary += String.fromCharCode(byte);
        await node('media_chunk', { transfer, index, base64: btoa(binary) });
      }
      if (controller.signal.aborted) mediaError('media_timeout', '图片传输已超时。');
      return await node('media_commit', { transfer });
    } finally {
      clearTimeout(deadline); controller.abort(); receiving--;
      // Cancellation may already revoke Host access. Worker binding/transfer
      // deadlines also discard unfinished buffers when this cleanup is refused.
      if (transfer) await node('media_abort', { transfer }).catch(() => {});
    }
  }
  cindy.onHostMessage(async msg => {
    if (msg.type !== 'tool-call') return; // No page/focus/event may reopen a binding.
    let result, images;
    try {
      const args = msg.args;
      const context = args?.session_context;
      const allowed = Object.hasOwn(argumentsByMethod, msg.tool) ? argumentsByMethod[msg.tool] : null;
      if (!allowed || !args || typeof args !== 'object' || Array.isArray(args)
        || Object.keys(args).some(key => key !== 'session_context' && !(msg.tool === 'receive_media' && key === 'attachments') && !allowed.includes(key))) {
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
        const trustedContext = {
          session_id: context.session_id, workdir: context.workdir,
          workdir_is_local: true, workdir_is_read_only: context.workdir_is_read_only,
        };
        if (msg.tool === 'receive_media') {
          result = await receiveMedia(msg, trustedContext, input);
        } else {
        const response = await cindy.node.request({
          method: msg.tool,
          params: { context: trustedContext, ...(allowed.length ? { arguments: input } : {}) },
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
        if (result?.success && result.transport_image) {
          const preview = result.transport_image;
          delete result.transport_image;
          images = [{ data: preview.data }];
          // Preview bytes, not the original. Core image edit needs an owned
          // media URL; native engines can use the attached conversation image.
          let deposited;
          try { deposited = await cindy.send({ type: 'cindy-request', kind: 'deposit_media', data: preview.data, label: 'Lumi project reference preview', callId: msg.callId }); } catch {}
          result.data.reference_delivery = { kind: 'bounded_preview', deposited: deposited?.ok === true };
          if (deposited?.ok && typeof deposited.url === 'string' && deposited.url.startsWith('cindy-media://')) result.data.reference_media_url = deposited.url;
        }
      }
    } catch (error) {
      result = error?.business || fail('host_node_failed', '宿主运行时连接失败。', '检查插件状态后重新执行诊断；不要自动重放写操作。');
    }
    // A Lumi business failure must also be a failed Host tool receipt.
    if (result?.success === true) {
      await cindy.send({ type: 'tool-result', callId: msg.callId, ok: true, result, ...(images ? { images } : {}) });
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
