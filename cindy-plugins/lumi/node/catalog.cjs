'use strict';
const snapshot = require('./business-specs.json');
const { RuntimeError } = require('./errors.cjs');
const { receiveMediaSchema } = require('./media-contract.cjs');
const { launchTools } = require('./workspace-contract.cjs');
const productionSchemas = require('./production-contract.json');
// This is the authorization boundary. New MCP tools never enter implicitly.
const categories = {
  project: ['project_init', 'project_overview', 'project_setup', 'runtime_status', 'runtime_release'],
  story: ['story_profile', 'chapters', 'book_outline'], settings: ['settings', 'setting_boards', 'apply_setting_breakdown'], pages: ['pages'],
  prompts: ['prompt_catalog', 'render_prompt'], media: ['image_target', 'prepare_page_image', 'read_image', 'receive_media'], review: launchTools, production: Object.keys(productionSchemas), export: ['export_book', 'export_website'],
};
const rules = {
  production: [
    '用户以 create_thread 表达新建独立 Cindy 会话时，使用宿主 send_to_session 新建模式，不传 target_session_id；通过 cindy_helper.list_tools(category="handoff") 发现并由 cindy_helper.call_tool 调用，不经 Lumi call_tool。保留指定模型、项目、执行目录与父任务职责；核实新 session 回执并提供链接，不因缺少同名工具改用 spawn_agent。按意图选择入口，不因正文仅提及工具名就创建任务。详细流程见 authoring/references/serial-production.md。',
    '派发前按用户要求发现真实任务类型与等待/回报渠道；原生 thread、subagent、Orca Worker、普通 session 不静默互换。send_to_session 新建会话不自带父子等待或自动完成回报。production_tasks 仅覆盖本插件普通任务，不实现原生等待或完成订阅；get_run/read_messages 不是 wait。禁止 sleep→查状态→汇报循环，也不改用 get_task_digest/get_session_runtime 轮询。核实完成回报后保存 waiting 并结束当前轮；无渠道则记录能力限制，不承诺自动续接。只处理匹配批次的新完成/阻塞/依赖回报，重复或过期进度不重复展开。',
    '直接使用同名顶层 production_records / production_tasks，不经 call_tool。开始多章制作前读取 authoring/references/serial-production.md。默认逐章、逐场串行；用户明确选择后最多两场无依赖并行。使用 production_tasks 时已建角色任务自动复用，未知创建/发送先 request_status，再用 resume_request 原键核对；其子任务在 Host 分配目录中只写草稿，当前 Lumi 任务串行写回，不能声称子任务已绑定同一项目。这些适配器限制不套用于宿主 send_to_session，后者按实际项目、目录和权限回执核实。',
    '记录保存在当前项目 user-data/lumi-production/，read 返回 revision=0 表示不存在；write 需要最新 expected_revision 与稳定 request_key。逐页保存草稿、写入回执、完整请求参数及未决状态。恢复先 project_overview → project_setup get → book_outline，核对当前采用图/review_status；本地记录不是作品事实。get_run 的 reconciling 或超时不等于完成，read_messages 沿 after 游标续读。PERMISSION_DENIED 停止，不重复请求授权；只有用户修复授权并明确要求后，才用 retry_create 与不同 new_request_key 重建已知权限拒绝的创建；未知、取消及成功请求不可替换。任务能力不可用时，仅在用户允许当前任务承担制作的范围内按同一记录串行完成；父任务只创建和等待时不接手制作。',
  ],
  project: ['使用 Host 当前任务绑定；不接收路径或项目标识。project_init 会写入；project_setup 仅 get 只读，update/finalize 需最新 revision。只读业务读取要求项目已由可写任务打开。', 'picture_book.aspect_ratio 的 landscape/square/portrait 是预设，只传 mode，不回传读取结果中的 width/height；只有 custom 才同时传 width 和 height。update 成功后才用返回的新 revision finalize，不预排依赖写入。'],
  story: ['story_profile get、chapters list/get、book_outline 只读。create 使用 project.revision，其他修改使用目标 revision。正文 write 必须提交完整文本；历史不可变，delete 为软删除。'],
  settings: ['list/get 只读。create 使用 list 的 premise.revision；update/delete/select_image 使用目标 revision。select_image 是写入。setting_boards 仅 list/get；总览图由 receive_media(slot=setting_board) 导入。apply_setting_breakdown 是原子写入，使用批次与 premise 两级 revision；同键原参数重放返回原结果。'],
  pages: ['list/get 只读。create 使用 list 的 state.revision，逐页重读；update/delete/select_image 使用 page.revision。description_md 为正文，storyboard_md 为完整画面描述，不能省略或截断。', 'record_image_review 是已有的写操作。页面规划、实际看图记录与按 review_status 续做的完整流程见 ghost_manual({ghost_id:"<当前插件 id>",path:"authoring/MANUAL.md"}) 的「设定、章节与页面」及「页面成图核对与续做」；替换为当前发现的精确身份与 namespace。'],
  prompts: ['render_prompt 提供文本任务，prepare_page_image 提供页面完整生图提示词和签名准备凭据；当前 Agent 执行后调用业务工具保存。page / setting 可渲染图片提示词；生图由当前 Agent 执行，不启动 Lumi pipeline。setting_board / setting_breakdown 分别用于批量总览图和已保存原图的识图拆分。', '所有传入的目标与 context UUID 必须属于当前项目；page_uuid 同时需要 chapter_uuid。不存在或跨项目的上下文 UUID 会拒绝，不默默忽略。', '页面使用 prepare_page_image(page_uuid, setting_uuids, setting_reviews, additional_direction?)，服务端组装完整 generation_prompt、临时合图路径、image_write_back 与含摘要签名的 preparation。原生引擎查看并引用同一 reference_image_path；Core 先核对 reference_delivery.kind=full_collage 且 status=ready，再按当前 Guide 将 reference_media_url 与完整 generation_prompt 传入请求。明确 [] / none 时不传参考图。receive_media 原样带回 prompt/preparation；校验缺参且未执行副作用时可按 schema 补齐，权限拒绝或未知写入/付费提交不得重放。'],
  media: ['设定先 image_target；页面先 prepare_page_image 获取完整提示词、同一完整合图与 preparation。原生引擎显式使用正式参考参数及 reference_image_path；Core 使用 status=ready 的 full_collage reference_media_url，先实际查看合图，再按本次 Guide 明确传图。status=failed 表示 Core 交接失败，未提交生图；不得用预览图/路径冒充，也不切付费 pipeline。none 模式不传参考。总览图生成后用 receive_media(slot=setting_board)，单项设定/页面用对应 slot；均在 ghost_call.attachments 交接恰好一张图，不在 args 中自报路径、指纹、字节或 session。call_tool 不能代替附件入口。',
    '历史矩形改图先 pages get(include_versions=true)，按 version_no 取得本页 base_variant_uuid 并看该版完整页面。prepare_page_image 传 base_variant_uuid、整页归一化 selection 和 additional_direction，setting_uuids 省略或 []，不传 setting_reviews。reference_mode=local_edit 返回原尺寸上下文裁图，Core 只用 reference_delivery.kind=local_edit_crop、status=ready 的 reference_media_url；原生通道引用同一 reference_image_path。唯一裁图与 generation_prompt 原样交给模型，按裁图比例输出完整裁图。receive_media 带原签名 preparation、同一 prompt、fit_mode=keep；只新增候选 adopted=false，当前采用版不变。看候选并经用户明确采用后才 pages select_image。旧 revision 或结果未知时先查当前版本和写入回执，不重复生图或导入。',
    '单图最多 20 MiB；PNG/JPEG/WebP，完整解码最多 4000 万像素。默认 keep，可选 crop/pad 并保留原图历史。冲突或结果未知先重读目标，不自动重新生成或覆盖。',
    'read_image 返回最长边不超过 1024、JPEG 不超过 120 KiB 的受控参考图。Core 使用返回的 reference_media_url；引擎原生生图使用刚读到的对话参考图。寄存预览成功不代表编辑成功，也不能证明订阅额度来源。'],
  review: ['工作区使用 Cindy 标准 MCP Apps 工具结果页。直接以 ghost_call.tool=review_book / review_chapters / review_chapter / review_page / review_settings / review_candidates 开页，业务参数直接放 args，不经 call_tool。开页只读，保存和采用由用户在界面明确提交；点选引用只更新当前对话上下文，不发送消息或写入作品。', '页面工具仅由 app.callServerTool 调用，不向模型开放；每次使用 Host 当前任务上下文，卡片项目 UUID 仅作身份核对。旧卡片遇到项目切换时拒绝，回当前任务重新开页。正文事实仍由 book_outline、chapters/settings/pages 的 list/get 获取；不能把开页摘要当完整审阅。'],
  export: ['明确要求生成或更新网站时直接 export_website，默认不跳过缺图；只问能力、普通绘本导出及 PDF/ZIP/Markdown 不触发。网站只在本地生成。收到 done=true 后，告诉用户网站已生成在 output_directory 对应的本地目录。同一条回复中将完整 publish_prompt 原样放入独立的文本代码块，供用户复制到 Codex 后修改和发送。发布需要用户另行提出。等待或断线时通过本工具的幂等重放核对原任务，不需要额外状态工具；有回执时使用其中的 idempotency_key。', 'export_book 会落盘，是写操作。成功后将固定导出文件准备到当前任务 lumi-exports/，path 为相对路径，delivery.status=prepared、file_generated=true、user_received=false。在 Cindy 任务文件列表打开；只有实际读取回执或用户确认才能报告收到，本地绝对路径不代表手机可下载。', 'export_delivery_failed 表示原文件已生成但可见副本准备失败；按 correction.generated_export 重读核对，不自动再次导出。'],
};
const common = ['Lumi 业务只使用本目录工具与 JSON 对象参数；宿主会话操作经 cindy_helper 发现和调用，不经 Lumi call_tool。UUID 必须来自当前项目读取，不能使用内部 id。',
  '版本冲突时保留已有内容，重读目标后再判断；取消、超时或 result_unknown 后先核对状态，禁止自动重写。',
  '列表使用 cursor_pagination；result_too_large 时减小 limit 或上下文范围，不截断正文。完整流程见当前插件 authoring/MANUAL.md。'];
const empty = { type: 'object', properties: {}, required: [], additionalProperties: false };
const callSchema = { type: 'object', properties: { name: { type: 'string' }, args: { type: 'object' } }, required: ['name'], additionalProperties: false };
const specs = Object.fromEntries(snapshot.map(s => [s.Name, { name: s.Name, description: s.Description, input_schema: s.Schema }]));
for (const [name, schema] of Object.entries(productionSchemas)) specs[name] = { name, description: name === 'production_records' ? '当前项目制作交接、逐页草稿与回执的有界持久记录。' : 'Cindy 自有普通任务与可恢复请求回执；必须调用同名顶层工具。', input_schema: schema };
specs.runtime_status = { name: 'runtime_status', description: '检查随包 CLI 与当前任务连接；只读诊断不打开作品。', input_schema: empty };
specs.runtime_release = { name: 'runtime_release', description: '释放当前任务连接，保留共享 CLI 与作品。', input_schema: empty };
specs.receive_media = { name: 'receive_media', description: '仅通过顶层同名工具接收 ghost_call.attachments 授权的单张原图，导入页面、单项设定或设定总览图；不能通过 call_tool 调用。', input_schema: receiveMediaSchema(snapshot) };
// Models may inspect boards and submit visual crops, but only the attachment
// adapter may turn an authorized file into a board import.
specs.setting_boards = structuredClone(specs.setting_boards);
specs.setting_boards.input_schema.properties.action.enum = ['list', 'get'];
for (const key of ['path', 'source_text', 'style_snapshot', 'prompt', 'expected_revision', 'idempotency_key']) delete specs.setting_boards.input_schema.properties[key];
specs.setting_boards.input_schema.allOf = specs.setting_boards.input_schema.allOf.filter(rule => rule.if.properties.action.const !== 'import');
specs.setting_boards.description = '查询当前项目已保存的设定总览图、原图身份、批次 revision 和 latest_breakdown。导入必须用顶层 receive_media(slot=setting_board) 与授权 attachments，不接收自报路径。';
const allowed = new Set(Object.values(categories).flat());
// Host instructions are explicit adapters; business input schemas still come
// unchanged from Go. Never advertise a native-engine command as a Core input.
specs.prepare_page_image = { ...specs.prepare_page_image, description: '准备页面完整图片提示词及当前设定的单张带标签合图。setting_uuids 必须显式提供（最多 12 项，[] 表示无参考）；各项需要绑定当前 variant/revision/style_digest 的 passed setting_reviews，包含实际看图依据及具体比较。返回原始 generation_prompt、image_write_back、签名 preparation 和 reference_mode。collage 模式：原生引擎仅在正式参数支持本机文件时查看并引用 reference_image_path；Cindy Core 使用 reference_delivery.kind=full_collage、status=ready 的 reference_media_url，先看同一合图，再按当前模型 Guide 明确传入完整提示词和该图片。status=failed 不提交 Core 请求；none 不传图片。不得用预览、UUID 或路径代替 Core 媒体输入。导入必须带同一 prompt/preparation；页面、设定、规则变化或服务重启后重新准备、看图和生成。' };
specs.prepare_page_image.description += ' 局部改图改用 base_variant_uuid、selection 和 additional_direction，setting_uuids 省略或 []，不传 setting_reviews。base_variant_uuid 可来自本页任一未采用历史版；selection 以整页为坐标。local_edit 使用唯一原尺寸上下文裁图，Core 对应 local_edit_crop/status=ready；原样传提示词与签名 preparation，fit_mode=keep。导入 adopted=false，不改变当前采用版；用户明确采用后才 select_image。';
specs.read_image = { ...specs.read_image, description: '按当前项目 Asset UUID 读取受控图片用于设定视觉核对或单项参考，最长边 1024、JPEG 最多 120 KiB；返回 Host 图片和寄存成功时的 reference_media_url。这是预览，不能替代页面完整合图；页面应使用 prepare_page_image 的 full_collage 回执。' };
specs.render_prompt = structuredClone(specs.render_prompt);
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
const supported = new Set(['type', 'properties', 'required', 'additionalProperties', 'description', 'enum', 'const', 'allOf', 'if', 'then', 'pattern', 'minimum', 'maximum', 'minLength', 'maxLength', 'minItems', 'maxItems', 'uniqueItems', 'items']);
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
    if (schema.uniqueItems && new Set(value.map(v => JSON.stringify(v))).size !== value.length) return `${at}: duplicate items`;
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
const reads = { project_setup: ['get'], story_profile: ['get'], chapters: ['list', 'get'], settings: ['list', 'get'], pages: ['list', 'get'], setting_boards: ['list', 'get'] };
const readTools = new Set(['project_overview', 'book_outline', 'prompt_catalog', 'render_prompt', 'runtime_status', 'runtime_release', 'image_target', 'prepare_page_image', 'read_image', ...launchTools]);
function isWrite(name, input) {
  validate(name, input);
  if (name === 'production_records') return input.action === 'write';
  if (name === 'production_tasks') return ['create', 'send', 'resume_request', 'retry_create'].includes(input.action);
  return !readTools.has(name) && !(reads[name]?.includes(input.action));
}
module.exports = { categories, allowed, specs, discover, repair, validate, validateCall, isWrite, check };
