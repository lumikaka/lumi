/* Browser logic only. Host alone mints args.session_context and callId. */
(() => {
  'use strict';
  const schemas = {
    list_tools: { type: 'object', properties: { category: { type: 'string' } }, additionalProperties: false },
    call_tool: { type: 'object', properties: { name: { type: 'string' }, args: { type: 'object' } }, required: ['name'], additionalProperties: false },
  };
  // BEGIN GENERATED MEDIA CONTRACT
  schemas.receive_media = {"type":"object","additionalProperties":false,"properties":{"description":{"maxLength":12000,"type":"string"},"expected_revision":{"description":"Exact revision from the last read or render_prompt; required for every mutation.","minimum":0,"type":"integer"},"fit_mode":{"enum":["keep","crop","pad"],"type":"string"},"preparation":{"additionalProperties":false,"properties":{"additional_direction":{"maxLength":12000,"type":"string"},"digest":{"pattern":"^[0-9a-f]{64}$","type":"string"},"expected_revision":{"description":"Exact revision from the last read or render_prompt; required for every mutation.","minimum":0,"type":"integer"},"local_edit":{"additionalProperties":false,"properties":{"base_asset_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"base_sha256":{"pattern":"^[0-9a-f]{64}$","type":"string"},"base_variant_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"base_version_no":{"minimum":0,"type":"integer"},"compositor_version":{"description":"Signed compositor version; preserve unchanged.","maxLength":120000,"type":"string"},"crop_pixels":{"additionalProperties":false,"properties":{"height":{"minimum":0,"type":"integer"},"width":{"minimum":0,"type":"integer"},"x":{"minimum":0,"type":"integer"},"y":{"minimum":0,"type":"integer"}},"required":["x","y","width","height"],"type":"object"},"feather_pixels":{"minimum":0,"type":"integer"},"original_size":{"additionalProperties":false,"properties":{"height":{"minimum":0,"type":"integer"},"width":{"minimum":0,"type":"integer"}},"required":["width","height"],"type":"object"},"selection":{"additionalProperties":false,"properties":{"height":{"maximum":1,"minimum":0,"type":"number"},"width":{"maximum":1,"minimum":0,"type":"number"},"x":{"maximum":1,"minimum":0,"type":"number"},"y":{"maximum":1,"minimum":0,"type":"number"}},"required":["x","y","width","height"],"type":"object"},"selection_pixels":{"additionalProperties":false,"properties":{"height":{"minimum":0,"type":"integer"},"width":{"minimum":0,"type":"integer"},"x":{"minimum":0,"type":"integer"},"y":{"minimum":0,"type":"integer"}},"required":["x","y","width","height"],"type":"object"}},"required":["base_variant_uuid","base_version_no","base_asset_uuid","base_sha256","original_size","selection","selection_pixels","crop_pixels","feather_pixels","compositor_version"],"type":"object"},"page_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"project_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"setting_reviews":{"description":"Host visual assessments of the selected current images. Read image_target for style_digest and image versions. Compare originals and explicit user requirements; record concrete differences or agreement, not just 'checked'. No-reference selection uses [] or omits this field.","items":{"additionalProperties":false,"properties":{"comparison":{"maxLength":2000,"minLength":1,"type":"string"},"expected_revision":{"description":"Exact revision from the last read or render_prompt; required for every mutation.","minimum":0,"type":"integer"},"reference_basis":{"maxLength":2000,"minLength":1,"type":"string"},"setting_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"style_digest":{"pattern":"^[0-9a-f]{64}$","type":"string"},"variant_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"verdict":{"enum":["passed","needs_revision"],"type":"string"}},"required":["setting_uuid","variant_uuid","expected_revision","style_digest","verdict","reference_basis","comparison"],"type":"object"},"maxItems":12,"type":"array"},"setting_uuids":{"description":"Explicit current-project active settings in collage label order. [] means a real no-reference generation; omitted/null is invalid.","items":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"maxItems":12,"type":"array","uniqueItems":true},"signature":{"pattern":"^[0-9a-f]{64}$","type":"string"}},"required":["project_uuid","page_uuid","expected_revision","setting_uuids","additional_direction","digest","signature"],"type":"object"},"prompt":{"maxLength":12000,"type":"string"},"slot":{"type":"string","enum":["page_image","setting_image","setting_board"]},"target_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"idempotency_key":{"description":"Stable key for this exact request. Replay original input after interruption; use a new key for a different operation.","maxLength":160,"minLength":1,"type":"string"},"source_text":{"maxLength":12000,"type":"string"},"style_snapshot":{"maxLength":12000,"type":"string"}},"required":["slot","expected_revision"],"allOf":[{"if":{"properties":{"slot":{"enum":["page_image","setting_image"]}}},"then":{"additionalProperties":false,"allOf":[{"if":{"properties":{"slot":{"const":"page_image"}}},"then":{"required":["prompt","preparation"]}}],"properties":{"description":{"maxLength":12000,"type":"string"},"expected_revision":{"description":"Exact revision from the last read or render_prompt; required for every mutation.","minimum":0,"type":"integer"},"fit_mode":{"enum":["keep","crop","pad"],"type":"string"},"preparation":{"additionalProperties":false,"properties":{"additional_direction":{"maxLength":12000,"type":"string"},"digest":{"pattern":"^[0-9a-f]{64}$","type":"string"},"expected_revision":{"description":"Exact revision from the last read or render_prompt; required for every mutation.","minimum":0,"type":"integer"},"local_edit":{"additionalProperties":false,"properties":{"base_asset_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"base_sha256":{"pattern":"^[0-9a-f]{64}$","type":"string"},"base_variant_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"base_version_no":{"minimum":0,"type":"integer"},"compositor_version":{"description":"Signed compositor version; preserve unchanged.","maxLength":120000,"type":"string"},"crop_pixels":{"additionalProperties":false,"properties":{"height":{"minimum":0,"type":"integer"},"width":{"minimum":0,"type":"integer"},"x":{"minimum":0,"type":"integer"},"y":{"minimum":0,"type":"integer"}},"required":["x","y","width","height"],"type":"object"},"feather_pixels":{"minimum":0,"type":"integer"},"original_size":{"additionalProperties":false,"properties":{"height":{"minimum":0,"type":"integer"},"width":{"minimum":0,"type":"integer"}},"required":["width","height"],"type":"object"},"selection":{"additionalProperties":false,"properties":{"height":{"maximum":1,"minimum":0,"type":"number"},"width":{"maximum":1,"minimum":0,"type":"number"},"x":{"maximum":1,"minimum":0,"type":"number"},"y":{"maximum":1,"minimum":0,"type":"number"}},"required":["x","y","width","height"],"type":"object"},"selection_pixels":{"additionalProperties":false,"properties":{"height":{"minimum":0,"type":"integer"},"width":{"minimum":0,"type":"integer"},"x":{"minimum":0,"type":"integer"},"y":{"minimum":0,"type":"integer"}},"required":["x","y","width","height"],"type":"object"}},"required":["base_variant_uuid","base_version_no","base_asset_uuid","base_sha256","original_size","selection","selection_pixels","crop_pixels","feather_pixels","compositor_version"],"type":"object"},"page_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"project_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"setting_reviews":{"description":"Host visual assessments of the selected current images. Read image_target for style_digest and image versions. Compare originals and explicit user requirements; record concrete differences or agreement, not just 'checked'. No-reference selection uses [] or omits this field.","items":{"additionalProperties":false,"properties":{"comparison":{"maxLength":2000,"minLength":1,"type":"string"},"expected_revision":{"description":"Exact revision from the last read or render_prompt; required for every mutation.","minimum":0,"type":"integer"},"reference_basis":{"maxLength":2000,"minLength":1,"type":"string"},"setting_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"style_digest":{"pattern":"^[0-9a-f]{64}$","type":"string"},"variant_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"verdict":{"enum":["passed","needs_revision"],"type":"string"}},"required":["setting_uuid","variant_uuid","expected_revision","style_digest","verdict","reference_basis","comparison"],"type":"object"},"maxItems":12,"type":"array"},"setting_uuids":{"description":"Explicit current-project active settings in collage label order. [] means a real no-reference generation; omitted/null is invalid.","items":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"maxItems":12,"type":"array","uniqueItems":true},"signature":{"pattern":"^[0-9a-f]{64}$","type":"string"}},"required":["project_uuid","page_uuid","expected_revision","setting_uuids","additional_direction","digest","signature"],"type":"object"},"prompt":{"maxLength":12000,"type":"string"},"slot":{"enum":["page_image","setting_image"],"type":"string"},"target_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"}},"required":["target_uuid","slot","expected_revision"],"type":"object"}},{"if":{"properties":{"slot":{"const":"setting_board"}}},"then":{"type":"object","additionalProperties":false,"properties":{"expected_revision":{"description":"Exact revision from the last read or render_prompt; required for every mutation.","minimum":0,"type":"integer"},"idempotency_key":{"description":"Stable key for this exact request. Replay original input after interruption; use a new key for a different operation.","maxLength":160,"minLength":1,"type":"string"},"prompt":{"maxLength":12000,"type":"string"},"source_text":{"maxLength":12000,"type":"string"},"style_snapshot":{"maxLength":12000,"type":"string"},"slot":{"const":"setting_board"}},"required":["source_text","prompt","expected_revision","idempotency_key"]}}]};
  // END GENERATED MEDIA CONTRACT
  // BEGIN GENERATED WORKSPACE CONTRACT
  Object.assign(schemas, {"review_book":{"additionalProperties":false,"properties":{"after":{"description":"next_cursor from the previous response.","pattern":"^[0-9]{1,10}$","type":"string"},"before":{"description":"Return a page before this cursor; mutually exclusive with after.","pattern":"^[0-9]{1,10}$","type":"string"},"limit":{"description":"List size, default 20; reduce if result_too_large.","maximum":100,"minimum":1,"type":"integer"}},"required":[],"type":"object"},"review_chapters":{"additionalProperties":false,"properties":{"after":{"description":"next_cursor from the previous response.","pattern":"^[0-9]{1,10}$","type":"string"},"before":{"description":"Return a page before this cursor; mutually exclusive with after.","pattern":"^[0-9]{1,10}$","type":"string"},"limit":{"description":"List size, default 20; reduce if result_too_large.","maximum":100,"minimum":1,"type":"integer"}},"required":[],"type":"object"},"review_chapter":{"additionalProperties":false,"properties":{"after":{"description":"next_cursor from the previous response.","pattern":"^[0-9]{1,10}$","type":"string"},"before":{"description":"Return a page before this cursor; mutually exclusive with after.","pattern":"^[0-9]{1,10}$","type":"string"},"chapter_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"limit":{"description":"List size, default 20; reduce if result_too_large.","maximum":100,"minimum":1,"type":"integer"}},"required":["chapter_uuid"],"type":"object"},"review_page":{"additionalProperties":false,"properties":{"chapter_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"page_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"}},"required":["chapter_uuid","page_uuid"],"type":"object"},"review_settings":{"additionalProperties":false,"properties":{"after":{"description":"next_cursor from the previous response.","pattern":"^[0-9]{1,10}$","type":"string"},"before":{"description":"Return a page before this cursor; mutually exclusive with after.","pattern":"^[0-9]{1,10}$","type":"string"},"limit":{"description":"List size, default 20; reduce if result_too_large.","maximum":100,"minimum":1,"type":"integer"},"setting_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"}},"required":[],"type":"object"},"review_candidates":{"additionalProperties":false,"allOf":[{"if":{"properties":{"kind":{"enum":["page"],"type":"string"}}},"then":{"required":["chapter_uuid","page_uuid"]}},{"if":{"properties":{"kind":{"enum":["setting"],"type":"string"}}},"then":{"required":["setting_uuid"]}},{"if":{"properties":{"kind":{"enum":["text"],"type":"string"}}},"then":{"required":["candidates"]}}],"properties":{"candidates":{"items":{"additionalProperties":false,"properties":{"text":{"description":"Complete candidate text","maxLength":120000,"type":"string"},"title":{"description":"Candidate title","maxLength":120000,"type":"string"}},"required":["title","text"],"type":"object"},"maxItems":12,"minItems":2,"type":"array"},"chapter_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"kind":{"enum":["page","setting","text"],"type":"string"},"page_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"setting_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"}},"required":["kind"],"type":"object"},"read_app_resource":{"additionalProperties":false,"properties":{"query":{"additionalProperties":{"description":"Validated REST query value.","maxLength":120000,"type":"string"},"type":"object"},"resource":{"description":"Allowlisted project-relative resource, or project for the work itself.","maxLength":120000,"type":"string"},"expected_project_uuid":{"description":"Identity constraint from this card launch; never selects a project or task.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"}},"required":["resource","expected_project_uuid"],"type":"object"},"write_app_resource":{"additionalProperties":false,"properties":{"body":{"additionalProperties":true,"type":"object"},"method":{"enum":["POST","PUT","PATCH","DELETE"],"type":"string"},"query":{"additionalProperties":{"description":"Validated REST query value.","maxLength":120000,"type":"string"},"type":"object"},"resource":{"description":"Allowlisted project-relative resource.","maxLength":120000,"type":"string"},"expected_project_uuid":{"description":"Identity constraint from this card launch; never selects a project or task.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"}},"required":["resource","method","expected_project_uuid"],"type":"object"},"read_app_image":{"additionalProperties":false,"properties":{"asset_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"max_edge":{"maximum":1024,"minimum":256,"type":"integer"},"expected_project_uuid":{"description":"Identity constraint from this card launch; never selects a project or task.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"}},"required":["asset_uuid","expected_project_uuid"],"type":"object"},"read_app_image_detail":{"additionalProperties":false,"properties":{"asset_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"max_edge":{"maximum":2048,"minimum":256,"type":"integer"},"expected_project_uuid":{"description":"Identity constraint from this card launch; never selects a project or task.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"}},"required":["asset_uuid","max_edge","expected_project_uuid"],"type":"object"},"watch_app_changes":{"additionalProperties":false,"allOf":[{"if":{"properties":{"action":{"const":"close"}}},"then":{"required":["project_uuid","subscription_uuid"]}},{"if":{"properties":{"action":{"const":"next"}}},"then":{"required":["project_uuid","subscription_uuid","cursor"]}},{"if":{"properties":{"action":{"const":"open"}}},"then":{"required":["project_uuid"]}}],"properties":{"action":{"enum":["open","next","close"],"type":"string"},"cursor":{"pattern":"^(0|[1-9][0-9]{0,19})$","type":"string"},"project_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"subscription_uuid":{"description":"Public UUIDv7 from a previous read.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"},"expected_project_uuid":{"description":"Identity constraint from this card launch; never selects a project or task.","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$","type":"string"}},"required":["action","expected_project_uuid"],"type":"object"}});
  // END GENERATED WORKSPACE CONTRACT
  // BEGIN GENERATED PRODUCTION CONTRACT
  Object.assign(schemas, {"production_records":{"type":"object","additionalProperties":false,"properties":{"action":{"enum":["read","write","schedule"],"type":"string"},"expected_project_uuid":{"type":"string","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"},"record":{"type":"string","pattern":"^(index[.]json|chapters/[0-9a-f-]{36}/(chapter|settings|storyboard-commit|scene-[1-9][0-9]{0,4}|storyboard/(manifest|page-[1-9][0-9]{0,4}))[.]json)$"},"expected_revision":{"type":"integer","minimum":0},"request_key":{"type":"string","minLength":1,"maxLength":160,"pattern":"^[A-Za-z0-9_.:-]+$"},"data":{"type":"object","additionalProperties":true}},"required":["action","expected_project_uuid","record"],"allOf":[{"if":{"properties":{"action":{"const":"write"}}},"then":{"required":["data","expected_revision","request_key"]}}]},"production_tasks":{"type":"object","additionalProperties":false,"properties":{"action":{"type":"string","enum":["capabilities","create","get","send","get_run","read_messages","request_status","resume_request","retry_create"]},"expected_project_uuid":{"type":"string","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"},"chapter_uuid":{"type":"string","pattern":"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"},"role":{"type":"string","enum":["chapter","storyboard","settings","scene"]},"scene":{"type":"integer","minimum":1,"maximum":99999},"request_key":{"type":"string","minLength":1,"maxLength":160,"pattern":"^[A-Za-z0-9_.:-]+$"},"title":{"type":"string","minLength":1,"maxLength":200},"text":{"type":"string","minLength":1,"maxLength":32768},"expected_revision":{"type":"integer","minimum":0},"after":{"type":"string","maxLength":2000},"limit":{"type":"integer","minimum":1,"maximum":50},"new_request_key":{"type":"string","minLength":1,"maxLength":160,"pattern":"^[A-Za-z0-9_.:-]+$","description":"Only for explicit retry_create after the user restores Host permission: a distinct stable key. Unknown, cancelled, conflicting or successful requests cannot be replaced."}},"required":["action","expected_project_uuid"],"allOf":[{"if":{"properties":{"action":{"const":"create"}}},"then":{"required":["chapter_uuid","role","request_key","title"]}},{"if":{"properties":{"action":{"const":"get"}}},"then":{"required":["chapter_uuid","role"]}},{"if":{"properties":{"action":{"const":"send"}}},"then":{"required":["chapter_uuid","role","request_key","expected_revision","text"]}},{"if":{"properties":{"action":{"const":"read_messages"}}},"then":{"required":["chapter_uuid","role"]}},{"if":{"properties":{"action":{"const":"get_run"}}},"then":{"required":["request_key"]}},{"if":{"properties":{"action":{"const":"request_status"}}},"then":{"required":["request_key"]}},{"if":{"properties":{"action":{"const":"resume_request"}}},"then":{"required":["request_key"]}},{"if":{"properties":{"action":{"const":"retry_create"}}},"then":{"required":["request_key","new_request_key"]}}]}});
  // END GENERATED PRODUCTION CONTRACT
  const productionTools = ['production_records', 'production_tasks'];
  const launchTools = ['review_book', 'review_chapters', 'review_chapter', 'review_page', 'review_settings', 'review_candidates'];
  const pageTools = ['read_app_resource', 'write_app_resource', 'read_app_image', 'read_app_image_detail', 'watch_app_changes'];
  const mediaFields = Object.keys(schemas.receive_media.properties);
  const argumentsByMethod = { list_tools: ['category'], call_tool: ['name', 'args'], receive_media: mediaFields };
  for (const name of [...launchTools, ...pageTools, ...productionTools]) argumentsByMethod[name] = Object.keys(schemas[name].properties);
  const fail = (code, message, details = '') => ({ success: false, data: null, error: { code, message, details } });
  const mediaError = (code, message) => { throw { business: fail(code, message, '保留作品并重读目标；使用顶层 attachments 交接原图，不自动重新生成。') }; };
  let receiving = 0;
  async function production(msg, context, input) {
    const node = async extra => {
      const response = await cindy.node.request({ method: 'production_call', params: { context, name: msg.tool, arguments: input, invocation: msg.callId, ...extra },
        callId: msg.callId, cancelWithCall: true, timeoutMs: 60000 });
      if (!response?.ok) throw { business: fail('production_result_unknown', '制作协调结果未确认。', '读取原记录或 request_status；不换键重新创建或发送。') };
      return response.result;
    };
    if (msg.tool === 'production_records') return node({});
    // Verify binding and schema before contacting any task API. Capabilities
    // are read-only; the Host performs its own independent approval checks.
    if (input.action === 'request_status') return node({});
    const checked = await node(input.action === 'resume_request' ? {} : { preflight: true });
    if (!checked?.success || (input.action === 'resume_request' && !checked.data?.host_action)) return checked;
    if (typeof cindy.tasks?.capabilities !== 'function') return fail('host_capability_unavailable', '当前 Cindy 没有普通任务入口。', '保留交接记录，在当前任务中按章、按场串行完成。');
    const capabilities = await cindy.tasks.capabilities();
    const planned = checked?.data?.host_action ? checked : await node({ capabilities });
    if (!planned?.success || !planned.data?.host_action) return planned;
    const action = planned.data.host_action;
    if (!capabilities.operations?.includes(action.operation) || typeof cindy.tasks[action.operation] !== 'function') return fail('host_capability_unavailable', '当前 Cindy 未提供所需任务操作。', '保留原请求，不换键；当前任务可按记录串行完成。');
    let result, error;
    try {
      result = await cindy.tasks[action.operation](action.params);
      if (action.operation === 'readMessages') result = taskHandoff(result);
    }
    catch (e) { error = { code: typeof e?.code === 'string' ? e.code : 'OUTCOME_UNKNOWN' }; }
    if (action.request_key) {
      const receipt = await node({ receipt: { request_key: action.request_key, digest: action.digest, ...(error ? { error } : { result }) } });
      if (!receipt?.success || !error || receipt.data.status === 'succeeded') return receipt;
      return { ...fail(receipt.data.status === 'failed' ? 'host_task_rejected' : 'production_result_unknown', '宿主任务未确认执行成功。', '先用 request_status 读取原键；确定拒绝停止，未知结果不换键重发。'),
        correction: { request_key: action.request_key, status: receipt.data.status, host_error: error.code } };
    }
    return error ? fail('host_task_failed', '宿主任务读取失败。', error.code) : { success: true, data: { host_result: snakeHost(result) } };
  }
  // Ordinary task reads are snapshots, not a wait primitive. Keep this advice
  // on cached receipts and failures too; an accepted send is not a finished run.
  const taskCoordination = (input, result) => {
    const data = result?.success ? result.data : null;
    const receipt = data?.request || data;
    const operation = receipt?.operation || input.action;
    const host = receipt?.host_result;
    let next = 'select_task_and_wait_route', state = 'unobserved';
    if (!result?.success) next = 'handle_error_without_polling';
    else if (input.action === 'get_run') {
      state = typeof host?.status === 'string' ? host.status : 'unknown';
      next = state === 'completed' ? 'read_handoff_and_verify_lumi_once'
        : ['failed', 'cancelled', 'interrupted', 'needs_attention', 'awaiting_user'].includes(state)
          ? 'handle_blocker' : 'end_turn_until_report_or_explicit_resume';
    } else if (receipt?.status === 'failed') next = 'handle_blocker';
    else if (['pending', 'unknown'].includes(receipt?.status)) {
      state = 'unknown'; next = 'preserve_request_and_reconcile_once';
    } else if (operation === 'send' && receipt?.status === 'succeeded') {
      state = 'accepted'; next = 'end_turn_until_report_or_explicit_resume';
    } else if (operation === 'create' && receipt?.status === 'succeeded') {
      state = 'not_started'; next = 'select_wait_route_before_send';
    } else if (input.action === 'read_messages' || input.action === 'get') {
      next = 'follow_saved_wait_strategy';
    }
    const coordination = {
      scope: 'production_tasks_adapter', task_kind: 'ordinary_session',
      wait_supported: false, automatic_completion_notification: false,
      model_polling_allowed: false, next_action: next, run_state: state,
      chapter_completion_verified: false,
      ...(input.request_key ? { request_key: input.request_key } : {}),
      ...(host?.run_id ? { run_id: host.run_id } : {}),
    };
    return result?.success ? { ...result, data: { ...data, coordination } }
      : { ...result, correction: { ...result?.correction, coordination } };
  };
  // Role handoffs need visible text and file identities, never the child's
  // internal reasoning, tool inputs or execution trace. Keep Host cursors exact.
  const taskHandoff = result => {
    const visible = messages => Array.isArray(messages) ? messages
      .filter(message => ['user', 'assistant'].includes(message?.role) && typeof message.content === 'string')
      .map(({ id, role, content, createdAt, created_at }) => ({ id, role, content,
        ...(createdAt !== undefined ? { createdAt } : {}), ...(created_at !== undefined ? { created_at } : {}) })) : [];
    return { ...result,
      ...(Object.hasOwn(result || {}, 'items') ? { items: visible(result.items) } : {}),
      ...(Object.hasOwn(result || {}, 'messages') ? { messages: visible(result.messages) } : {}) };
  };
  const snakeHost = value => Array.isArray(value) ? value.map(snakeHost) : value && typeof value === 'object'
    ? Object.fromEntries(Object.entries(value).map(([key, item]) => [key.replace(/[A-Z]/g, c => '_' + c.toLowerCase()), snakeHost(item)])) : value;
  async function deliverReference(msg, context, result) {
    const reference = result.transport_reference;
    delete result.transport_reference; // Tokens/base64 never enter model JSON.
    const local = result.data.reference_mode === 'local_edit';
    const kind = local ? 'local_edit_crop' : 'full_collage';
    const label = local ? 'Lumi original-size local edit context crop' : 'Lumi complete page reference collage';
    const node = async (method, extra = {}) => {
      const response = await cindy.node.request({ method, params: { context, invocation: msg.callId, transfer: reference.transfer, ...extra },
        callId: msg.callId, cancelWithCall: true, timeoutMs: 60000 });
      if (!response?.ok || response.result?.success !== true) throw new Error('reference_transfer_failed');
      return response.result.data;
    };
    try {
      if (reference.kind !== kind || reference.mime_type !== 'image/png' || !Number.isSafeInteger(reference.bytes) || reference.bytes < 33
        || reference.bytes > 20 * 1024 * 1024 || reference.chunk_bytes !== 48 * 1024
        || !/^[a-f0-9]{64}$/.test(reference.sha256)) throw new Error('reference_metadata_invalid');
      const bytes = new Uint8Array(reference.bytes), chunks = [], started = Date.now();
      for (let offset = 0, index = 0; offset < bytes.length; offset += reference.chunk_bytes, index++) {
        if (Date.now() - started > 90000) throw new Error('reference_timeout');
        const chunk = await node('reference_chunk', { index });
        if (chunk.index !== index || typeof chunk.base64 !== 'string' || chunk.base64.length > 65536) throw new Error('reference_chunk_invalid');
        const binary = atob(chunk.base64);
        if (btoa(binary) !== chunk.base64 || binary.length !== Math.min(reference.chunk_bytes, bytes.length - offset)) throw new Error('reference_chunk_invalid');
        for (let i = 0; i < binary.length; i++) bytes[offset + i] = binary.charCodeAt(i);
        chunks.push(chunk.base64);
      }
      const sha256 = [...new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))].map(b => b.toString(16).padStart(2, '0')).join('');
      if (sha256 !== reference.sha256) throw new Error('reference_checksum_mismatch');
      const deposited = await cindy.send({ type: 'cindy-request', kind: 'deposit_media', data: chunks.join(''), label, callId: msg.callId });
      if (deposited?.ok !== true || deposited.url !== `cindy-media://blobs/${sha256}.png`
        || (deposited.hash !== undefined && deposited.hash !== sha256)
        || (deposited.bytes !== undefined && deposited.bytes !== bytes.length)) throw new Error('reference_deposit_failed');
      result.data.reference_media_url = deposited.url;
      result.data.reference_delivery = { kind, status: 'ready', sha256, byte_size: bytes.length,
        width: reference.width, height: reference.height,
        instructions: local
          ? '先实际查看此原尺寸上下文裁图；它是唯一编辑参考。Core 按当前模型 Guide 将 reference_media_url 作为图片输入，原生引擎使用正式参数引用同一 reference_image_path。原样传 generation_prompt，按裁图比例输出完整裁图，不裁边、加边或重新构图。receive_media 原样传签名 preparation、同一 prompt 和 fit_mode=keep；导入 adopted=false，仅用户明确采用后 select_image。未确认参考进入请求前不得生成。'
          : '先实际查看此完整合图；Cindy Core 按当前模型 Guide 将 reference_media_url 作为图片输入，并原样传 generation_prompt。原生引擎用其正式参考参数和 reference_image_path；未确认参考进入请求前不得生成。' };
    } catch (error) {
      // Preparing a business receipt succeeded; Core delivery did not. Keep the
      // native file and exact prompt available, and never claim Core readiness.
      delete result.data.reference_media_url;
      result.data.reference_delivery = { kind, status: 'failed', code: /^reference_[a-z_]+$/.test(error?.message || '') ? error.message : 'reference_deposit_failed',
        instructions: '完整参考图未交接到 Cindy Core；未提交生图。不要用缩略图、UUID 或路径冒充 Core 参考。保留 preparation；核对当前任务权限后重新准备。原生引擎仅在正式参考参数可读取 reference_image_path 时继续。' };
    } finally { await node('reference_release').catch(() => {}); }
  }
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
    if (msg.type !== 'tool-call') return; // Only Host-routed tool calls may bind a project.
    let result, images;
    const website = msg.tool === 'call_tool' && msg.args?.name === 'export_website';
    const resumeWebsite = '保持缺图参数与原幂等键，再次调用 export_website 续接本次任务；尚不能交付。';
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
        } else if (productionTools.includes(msg.tool)) {
          result = await production(msg, trustedContext, input);
          if (msg.tool === 'production_tasks') result = taskCoordination(input, result);
        } else {
        const prepareReference = msg.tool === 'call_tool' && input.name === 'prepare_page_image';
        const response = await cindy.node.request({
          method: prepareReference ? 'reference_prepare' : pageTools.includes(msg.tool) ? 'workspace_call' : msg.tool,
          params: prepareReference ? { context: trustedContext, invocation: msg.callId, input: input.args ?? {} }
            : pageTools.includes(msg.tool) ? { context: trustedContext, name: msg.tool, expected_project_uuid: input.expected_project_uuid,
              arguments: Object.fromEntries(Object.entries(input).filter(([key]) => key !== 'expected_project_uuid')) }
            : { context: trustedContext, ...(allowed.length ? { arguments: input } : {}) },
          callId: msg.callId, cancelWithCall: true, timeoutMs: 60000,
        });
        if (!response?.ok) {
          result = fail('host_node_failed', '宿主未完成 Node 调用。', website ? resumeWebsite : '检查插件权限或重新执行诊断；写操作结果未知时先重读，不自动重试。');
        } else if (typeof response.result?.success !== 'boolean' || !Object.hasOwn(response.result, 'data')
          || (!response.result.success && (response.result.data !== null
            || typeof response.result.error?.code !== 'string'
            || typeof response.result.error?.message !== 'string'
            || typeof response.result.error?.details !== 'string'))) {
          result = fail('protocol_error', 'Node 返回了无效结果。', '重新执行运行诊断。');
        } else result = response.result;
        }
        if (result?.success && result.transport_reference) await deliverReference(msg, trustedContext, result);
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
      result = error?.business || fail('host_node_failed', '宿主运行时连接失败。', website ? resumeWebsite : '检查插件状态后重新执行诊断；不要自动重放写操作。');
    }
    if (pageTools.includes(msg.tool)) {
      result ||= fail('protocol_error', '页面工具未返回有效结果。');
      const summary = result.success ? { success: true, data: { state: 'available', channel: '_meta' } } : result;
      await cindy.send({ type: 'tool-result', callId: msg.callId, ok: true, result: {
        content: [{ type: 'text', text: JSON.stringify(summary) }],
        structuredContent: msg.tool === 'watch_app_changes' ? result : summary,
        _meta: { 'lumi/app_response': result }, isError: !result.success,
      } });
      return;
    }
    if (launchTools.includes(msg.tool) && result?.success === true) {
      const ui = result.transport_launch_arguments;
      delete result.transport_launch_arguments;
      await cindy.send({ type: 'tool-result', callId: msg.callId, ok: true, result: {
        content: [{ type: 'text', text: JSON.stringify(result) }], structuredContent: result,
        ...(ui ? { _meta: { 'lumi/launch_arguments': ui } } : {}),
      } });
      return;
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
