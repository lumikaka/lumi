'use strict';
const { RuntimeError } = require('./errors.cjs');

function adaptAuthoringResult(method, input, result) {
  if (!result.success || method !== 'render_prompt' || input.target !== 'setting_board') return result;
  const data = result.data;
  if (data?.write_back?.tool !== 'setting_boards' || data.write_back.action !== 'import' || typeof data.prompt !== 'string') {
    throw new RuntimeError('protocol_error', '总览图任务写回契约不兼容。', '检查随包 CLI；未执行生图或导入。');
  }
  const write = { tool: 'receive_media', slot: 'setting_board', expected_revision: data.write_back.expected_revision,
    source_text: data.write_back.source_text, style_snapshot: data.write_back.style_snapshot,
    required_from_host: ['prompt', 'idempotency_key'],
    attachment_delivery: 'ghost_call top-level attachments: exactly one authorized generated image; not call_tool, no path in args',
  };
  const marker = '\n\nWrite-back contract (JSON):\n';
  const start = data.prompt.lastIndexOf(marker);
  const end = data.prompt.indexOf('\nExecute the catalog task above', start + marker.length);
  if (start < 0 || end < 0) throw new RuntimeError('protocol_error', '总览图任务缺少可识别的写回契约。', '检查 CLI 契约；不把路径导入交给普通工具调用。');
  // Only the orchestration contract changes. The exact visual prompt sent to
  // the model, all project facts and business revisions remain untouched.
  return { ...result, data: { ...data, write_back: write,
    prompt: data.prompt.slice(0, start + marker.length) + JSON.stringify(write, null, 2) + data.prompt.slice(end),
  } };
}

module.exports = { adaptAuthoringResult };
