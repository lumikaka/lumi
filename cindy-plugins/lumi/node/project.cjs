'use strict';
const { RuntimeError, failure } = require('./errors.cjs');
const UUID = /^[a-f0-9]{8}-[a-f0-9]{4}-7[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/;
const fields = {
  project_init: ['idea', 'name', 'generation_language'],
  project_setup: ['action', 'expected_revision', 'project_name', 'generation_language', 'overall_style', 'generation_brief', 'picture_book'],
};

function argumentsFor(method, params) {
  const input = params.arguments ?? {};
  const allowed = fields[method] || [];
  const bad = () => { throw new RuntimeError('invalid_arguments', '项目工具参数无效。', '按工具参数声明调用；不接受路径、任务或项目绑定。'); };
  if (!input || Array.isArray(input) || typeof input !== 'object' || Object.keys(input).some(k => !allowed.includes(k))) bad();
  for (const [key, value] of Object.entries(input)) {
    if (!['expected_revision', 'picture_book'].includes(key) && (typeof value !== 'string' || value.length > 120000)) bad();
  }
  if (method === 'project_init' && (typeof input.idea !== 'string' || !input.idea.trim()
    || (input.name !== undefined && [...input.name].length > 120))) bad();
  if (input.generation_language !== undefined && !['zh-Hans', 'en'].includes(input.generation_language)) bad();
  if (method === 'project_setup') {
    if (!['get', 'update', 'finalize'].includes(input.action)) bad();
    if (input.action !== 'get' && (!Number.isSafeInteger(input.expected_revision) || input.expected_revision < 0)) bad();
    if (input.picture_book !== undefined && (!input.picture_book || Array.isArray(input.picture_book) || typeof input.picture_book !== 'object')) bad();
  }
  return input; // CLI validates the complete business schema, including nested fields.
}

const pick = (object, keys) => Object.fromEntries(keys.filter(k => object?.[k] !== undefined).map(k => [k, object[k]]));
function pictureBook(value) {
  if (!value) return value;
  return { ...pick(value, ['format', 'large_image_minimal_text', 'interaction_mode', 'comic_layout']),
    ...(value.aspect_ratio ? { aspect_ratio: pick(value.aspect_ratio, ['mode', 'width', 'height']) } : {}) };
}
function setupState(value, uuid) {
  if (!value || value.project_uuid !== uuid || !Number.isSafeInteger(value.revision)
    || (value.uuid && !UUID.test(value.uuid)) || !['draft', 'ready'].includes(value.setup_status)) {
    throw new RuntimeError('protocol_error', 'CLI 未返回有效项目设置。', '检查随包 CLI 版本。');
  }
  const state = pick(value, ['uuid', 'project_uuid', 'revision', 'setup_status', 'status', 'original_input', 'missing_information', 'field_sources']);
  state.draft_values = pick(value.draft_values, ['project_name', 'generation_language', 'overall_style', 'generation_brief', 'generation_scope']);
  if (value.draft_values?.picture_book) state.draft_values.picture_book = pictureBook(value.draft_values.picture_book);
  if (value.final_picture_book) state.final_picture_book = pictureBook(value.final_picture_book);
  return state;
}
function publicProject(data) {
  const p = data?.project;
  if (!UUID.test(p?.uuid) || typeof p.name !== 'string' || !Number.isSafeInteger(p.revision)) {
    throw new RuntimeError('protocol_error', 'CLI 未返回有效项目身份。', '检查随包 CLI 版本。');
  }
  return {
    project: { ...pick(p, ['uuid', 'name', 'revision', 'setup_status', 'generation_language', 'chapter_count']),
      ...(p.picture_book ? { picture_book: pictureBook(p.picture_book) } : {}) },
    setup: setupState(data.setup, p.uuid),
    ...pick(data, ['setting_count', 'page_count']),
    limitations: { read_only: false, open_may_write: true, guidance: 'plugin_manual', server_policy: 'shared_retained' },
  };
}

// CLI diagnostics can include OS paths. Preserve codes, but expose only local
// adapter messages; endpoints, lock metadata, roots and nonces stay in Node.
function publicFailure(result) {
  const code = /^[a-z][a-z0-9_]{0,79}$/.test(result.error?.code) ? result.error.code : 'project_operation_failed';
  const guidance = {
    project_locked: ['项目正被其他 Lumi 实例占用。', '在 Lumi 桌面版关闭此项目或退出占用它的实例后重试；不会抢锁。'],
    project_not_initialized: ['当前目录尚未初始化为 Lumi 项目。', '确认 Cindy 已选择项目根目录，再用原始创意调用 project_init。'],
    project_initialization_conflict: ['目录中存在与 Lumi 冲突的受管文件。', '检查 README.md、STORY.md、project.sqlite 及其侧文件、.lumi、assets；请移动冲突文件或在 Cindy 选择其他目录，已有文件未被覆盖。'],
    project_identity_mismatch: ['项目目录与绑定身份不一致。', '核对 Cindy 当前目录；若目录或文件被替换，先恢复原路径或协调持锁实例释放，再重新读取。复制项目不能凭相同 UUID 合并。'],
    cli_upgrade_required: ['共享 CLI 版本不兼容。', '协调其他任务及客户端释放服务后再升级；插件不会自动停止共享服务。'],
  }[code];
  return failure(code, guidance?.[0] || 'Lumi 未完成项目操作。', guidance?.[1] || '重新读取项目概况与 setup 状态，核对参数和 revision；不要自动重放写操作。');
}
module.exports = { argumentsFor, publicProject, publicFailure, setupState, UUID };
