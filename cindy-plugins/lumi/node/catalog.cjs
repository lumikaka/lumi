'use strict';
const snapshot = require('./business-specs.json');
const { RuntimeError } = require('./errors.cjs');
// This is the authorization boundary. New MCP tools never enter implicitly.
const categories = {
  project: ['project_init', 'project_overview', 'project_setup', 'runtime_status', 'runtime_release'],
  story: ['story_profile', 'chapters', 'book_outline'], settings: ['settings'], pages: ['pages'],
  prompts: ['prompt_catalog', 'render_prompt'], media: ['image_target', 'read_image', 'receive_media'], review: [], export: ['export_book'],
};
const rules = {
  project: ['使用 Host 当前任务绑定；不接收路径或项目标识。project_init 会写入；project_setup 仅 get 只读，update/finalize 需最新 revision。只读业务读取要求项目已由可写任务打开。'],
  story: ['story_profile get、chapters list/get、book_outline 只读。create 使用 project.revision，其他修改使用目标 revision。正文 write 必须提交完整文本；历史不可变，delete 为软删除。'],
  settings: ['list/get 只读。create 使用 list 的 premise.revision；update/delete/select_image 使用目标 revision。select_image 是写入，receive_media 导入授权图片。'],
  pages: ['list/get 只读。create 使用 list 的 state.revision，逐页重读；update/delete/select_image 使用 page.revision。description_md 为正文，storyboard_md 为完整画面描述，不能省略或截断。'],
  prompts: ['render_prompt 是业务提示词唯一来源，返回完整任务、目标 UUID、expected_revision 和写回契约；当前 Agent 执行后调用业务工具保存。page / setting 可渲染图片提示词；生图由当前 Agent 执行，不启动 Lumi pipeline。setting_board / setting_breakdown 尚未开放。', '所有传入的目标与 context UUID 必须属于当前项目；page_uuid 同时需要 chapter_uuid。不存在或跨项目的上下文 UUID 会拒绝，不默默忽略。', '页面 section_image 先用 render_prompt 取得 before_image 和真实参考状态片段的 fragment_prompt，补入 before_image_prompt/reference_usage_text；section_id 由页面读取。图片模型使用 generation_prompt，导入使用 image_write_back；校验缺参且未执行副作用时可按 schema 补齐，权限拒绝或未知写入/付费提交不得重放。'],
  media: ['先 image_target 获取比例与最新 revision，再 render_prompt；生图由当前 Agent 使用已可用能力执行。完成后立即调用顶层 receive_media，并在 ghost_call.attachments 交接恰好一张图，不在 args 中自报路径、指纹、字节或 session。call_tool 不能代替附件入口。',
    '单图最多 20 MiB；PNG/JPEG/WebP，完整解码最多 4000 万像素。默认 keep，可选 crop/pad 并保留原图历史。冲突或结果未知先重读目标，不自动重新生成或覆盖。',
    'read_image 返回最长边不超过 1024、JPEG 不超过 120 KiB 的受控参考图。Core 使用返回的 reference_media_url；引擎原生生图使用刚读到的对话参考图。寄存预览成功不代表编辑成功，也不能证明订阅额度来源。'],
  review: ['尚未实现 Cindy 审阅界面；以 book_outline、chapters/settings/pages 的 list/get 逐页读取完整文本进行审阅。'],
  export: ['export_book 会落盘，是写操作。成功后将固定导出文件准备到当前任务 lumi-exports/，path 为相对路径，delivery.status=prepared、file_generated=true、user_received=false。在 Cindy 任务文件列表打开；只有实际读取回执或用户确认才能报告收到，本地绝对路径不代表手机可下载。', 'export_delivery_failed 表示原文件已生成但可见副本准备失败；按 correction.generated_export 重读核对，不自动再次导出。'],
};
const common = ['只使用本目录工具与 JSON 对象参数。UUID 必须来自当前项目读取，不能使用内部 id。',
  '版本冲突时保留已有内容，重读目标后再判断；取消、超时或 result_unknown 后先核对状态，禁止自动重写。',
  '列表使用 cursor_pagination；result_too_large 时减小 limit 或上下文范围，不截断正文。完整流程见当前插件 authoring/MANUAL.md。'];
const empty = { type: 'object', properties: {}, required: [], additionalProperties: false };
const callSchema = { type: 'object', properties: { name: { type: 'string' }, args: { type: 'object' } }, required: ['name'], additionalProperties: false };
const specs = Object.fromEntries(snapshot.map(s => [s.Name, { name: s.Name, description: s.Description, input_schema: s.Schema }]));
specs.runtime_status = { name: 'runtime_status', description: '检查随包 CLI 与当前任务连接；只读诊断不打开作品。', input_schema: empty };
specs.runtime_release = { name: 'runtime_release', description: '释放当前任务连接，保留共享 CLI 与作品。', input_schema: empty };
specs.receive_media = { name: 'receive_media', description: '仅通过顶层同名工具接收 ghost_call.attachments 授权的单张原图并导入当前项目；不能通过 call_tool 调用。', input_schema: structuredClone(specs.import_image.input_schema) };
delete specs.receive_media.input_schema.properties.path;
specs.receive_media.input_schema.required = specs.receive_media.input_schema.required.filter(k => k !== 'path');
const allowed = new Set(Object.values(categories).flat());
// Intentional Cindy-only restriction: no media-only render/write-back route.
specs.render_prompt = structuredClone(specs.render_prompt);
specs.render_prompt.input_schema.properties.target.enum = ['setup', 'story_profile', 'chapter', 'pages', 'page', 'setting', 'premise'];
delete specs.render_prompt.input_schema.properties.board_uuid;
specs.render_prompt.input_schema.allOf = specs.render_prompt.input_schema.allOf.filter(rule => !rule.if.properties.target.enum.includes('setting_breakdown'));
specs.render_prompt.description = specs.render_prompt.description.replace('Codex', 'the current Cindy Agent');
const categoryFor = name => Object.keys(categories).find(c => categories[c].includes(name));
function detail(category) {
  return { category, status: categories[category].length ? 'available' : 'not_implemented',
    tools: categories[category].map(name => specs[name]), rules: [...common, ...rules[category]] };
}
function repair(name) {
  const category = categoryFor(name);
  return category ? { tool: specs[name], rules: [...common, ...rules[category]], discovery: { tool: 'list_tools', args: { category } } }
    : { available_tools: [...allowed], rules: common, discovery: { tool: 'list_tools', args: {} } };
}
function invalid(name, message) {
  const error = new RuntimeError(allowed.has(name) ? 'invalid_arguments' : 'unknown_tool', message, '使用返回的 schema 与规则自纠后重试。');
  error.correction = repair(name); throw error;
}
// The generated schema vocabulary is intentionally small. Fail closed on drift.
const supported = new Set(['type', 'properties', 'required', 'additionalProperties', 'description', 'enum', 'const', 'allOf', 'if', 'then', 'pattern', 'minimum', 'maximum', 'minLength', 'maxLength', 'minItems', 'maxItems', 'items']);
function check(schema, value, at = '$') {
  for (const k of Object.keys(schema)) if (!supported.has(k)) throw new Error(`Unsupported schema keyword ${k}`);
  if (schema.type) {
    const valid = { object: value !== null && typeof value === 'object' && !Array.isArray(value), array: Array.isArray(value),
      string: typeof value === 'string', boolean: typeof value === 'boolean', integer: Number.isSafeInteger(value), number: typeof value === 'number' && Number.isFinite(value) };
    if (!valid[schema.type]) return `${at}: expected ${schema.type}`;
  }
  if (schema.enum && !schema.enum.includes(value)) return `${at}: enum ${schema.enum.join(', ')}`;
  if (Object.hasOwn(schema, 'const') && value !== schema.const) return `${at}: expected ${schema.const}`;
  if (typeof value === 'string') {
    if (schema.pattern && !new RegExp(schema.pattern).test(value)) return `${at}: invalid format`;
    if (schema.minLength !== undefined && [...value].length < schema.minLength) return `${at}: too short`;
    if (schema.maxLength !== undefined && [...value].length > schema.maxLength) return `${at}: too long`;
  }
  if (typeof value === 'number' && ((schema.minimum !== undefined && value < schema.minimum) || (schema.maximum !== undefined && value > schema.maximum))) return `${at}: out of bounds`;
  if (Array.isArray(value)) {
    if ((schema.minItems !== undefined && value.length < schema.minItems) || (schema.maxItems !== undefined && value.length > schema.maxItems)) return `${at}: array size`;
    if (schema.items) for (let i = 0; i < value.length; i++) { const e = check(schema.items, value[i], `${at}[${i}]`); if (e) return e; }
  } else if (value !== null && typeof value === 'object') {
    for (const k of schema.required || []) if (!Object.hasOwn(value, k)) return `${at}.${k}: required`;
    for (const [k, v] of Object.entries(value)) {
      const s = schema.properties && Object.hasOwn(schema.properties, k) ? schema.properties[k] : schema.additionalProperties;
      if (s === false) return `${at}.${k}: unknown field`;
      if (s && typeof s === 'object') { const e = check(s, v, `${at}.${k}`); if (e) return e; }
    }
  }
  for (const s of schema.allOf || []) { const e = check(s, value, at); if (e) return e; }
  if (schema.if && !check(schema.if, value, at) && schema.then) return check(schema.then, value, at);
  return null;
}
function validate(name, input) {
  if (!allowed.has(name)) invalid(name, '工具不在 Cindy 允许列表中。');
  const error = check(specs[name].input_schema, input);
  if (error) invalid(name, error);
  if (input.before !== undefined && input.after !== undefined) invalid(name, 'before 与 after 不能同时提供。');
  return input;
}
function discover(input) {
  const schema = { type: 'object', properties: { category: { type: 'string', enum: Object.keys(categories) } }, additionalProperties: false };
  const error = check(schema, input);
  if (error) { const e = new RuntimeError('invalid_arguments', error, '选择概览中的 category。'); e.correction = { input_schema: schema, rules: common }; throw e; }
  return input.category ? detail(input.category) : { categories: Object.keys(categories).map(category => ({ name: category, count: categories[category].length, status: categories[category].length ? 'available' : 'not_implemented' })) };
}
function validateCall(wrapper) {
  const error = check(callSchema, wrapper);
  if (error) {
    const e = new RuntimeError('invalid_arguments', error, 'call_tool 使用 name 与 JSON 对象 args；先用 list_tools 获取业务工具。');
    e.correction = { input_schema: callSchema, rules: common }; throw e;
  }
  return wrapper;
}
const reads = { project_setup: ['get'], story_profile: ['get'], chapters: ['list', 'get'], settings: ['list', 'get'], pages: ['list', 'get'] };
const readTools = new Set(['project_overview', 'book_outline', 'prompt_catalog', 'render_prompt', 'runtime_status', 'runtime_release', 'image_target', 'read_image']);
function isWrite(name, input) { validate(name, input); return !readTools.has(name) && !(reads[name]?.includes(input.action)); }
module.exports = { categories, allowed, specs, discover, repair, validate, validateCall, isWrite, check };
