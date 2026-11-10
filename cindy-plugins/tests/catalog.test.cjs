'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { allowed, categories, specs, discover, validate, validateCall, isWrite } = require('../lumi/node/catalog.cjs');
const { envelope } = require('../lumi/node/errors.cjs');
const uuid = '01989abc-def0-7000-8000-000000000001';

test('two-stage catalog discloses schemas and rules; unavailable categories have no tools', () => {
  assert.deepEqual(discover({}).categories.map(c => c.name), ['project', 'story', 'settings', 'pages', 'prompts', 'media', 'review', 'export']);
  for (const category of Object.keys(categories)) {
    const d = discover({ category });
    assert.ok(d.rules.length);
    for (const spec of d.tools) { assert.ok(spec.description); assert.equal(spec.input_schema.additionalProperties, false); }
  }
  assert.equal(discover({ category: 'media' }).status, 'available');
  assert.deepEqual(discover({ category: 'media' }).tools.map(t => t.name), ['image_target', 'prepare_page_image', 'read_image', 'receive_media']);
  assert.deepEqual(discover({ category: 'review' }).tools, []);
  assert.deepEqual(discover({ category: 'settings' }).tools.map(t => t.name), ['settings', 'setting_boards', 'apply_setting_breakdown']);
  for (const name of ['read_app_connection', 'watch_app_changes', 'read_app_image', 'import_image', 'review_book', 'exec', 'tools/call', 'reference_prepare', 'reference_chunk', 'reference_release']) assert.equal(allowed.has(name), false);
  assert.equal(specs.prepare_page_image.description.includes('referenced_image_paths'), false);
  assert.ok(specs.prepare_page_image.description.includes('reference_media_url'));
});
test('unknown tools and invalid arguments carry schema and usable correction rules', () => {
  for (const [name, args] of [
    ['export_website', {action:'status'}], ['export_website', {chapter_uuid:uuid}], ['export_website', {path:'/tmp/website'}],
    ['pages', { action: 'execute' }], ['pages', { action: 'create', chapter_uuid: uuid }],
    ['pages', { action: 'list', chapter_uuid: uuid, limit: 101 }],
    ['pages', { action: 'get', chapter_uuid: uuid, page_uuid: '1' }],
    ['story_profile', { action: 'write', expected_revision: 1.5, story_md: 'x' }],
    ['project_setup', { action: 'update', expected_revision: 1, picture_book: { format: 'comic_story', unknown: true } }],
    ['project_overview', JSON.parse('{"__proto__":{}}')],
    ['book_outline', { after: '1', before: '2' }],
    ['render_prompt', { prompt_group: 'premise', prompt_key: 'asset_breakdown', target: 'setting_breakdown' }],
    ['setting_boards', { action: 'import', path: '/tmp/forged.png' }],
    ['setting_boards', { action: 'get', board_uuid: uuid, path: '/tmp/forged.png' }],
    ['chapters', { action: 'write', chapter_uuid: uuid, expected_revision: 1, content: 'x'.repeat(120001) }],
  ]) {
    assert.throws(() => validate(name, args), e => {
      const result = envelope(e); assert.equal(result.success, false);
      assert.equal(result.error.code, 'invalid_arguments'); assert.deepEqual(result.correction.tool.input_schema, specs[name].input_schema);
      assert.ok(result.correction.rules.length); return true;
    });
  }
  assert.throws(() => validate('import_image', {}), e => e.code === 'unknown_tool' && e.correction.available_tools.includes('pages'));
  assert.throws(() => discover({ category: 'fake' }), e => !!e.correction.input_schema);
  for (const value of [undefined, null, [], {}, { name: 'pages', args: 'not an object' }, { name: 'pages', context: {} }]) {
    assert.throws(() => validateCall(value), e => e.code === 'invalid_arguments' && e.correction.input_schema.required.includes('name'));
  }
});
test('every action is classified explicitly; finalize, selection and export are writes', () => {
  const input = {
    project_init: { idea: 'x' }, project_setup: { expected_revision: 0 }, story_profile: { story_md: 'x', expected_revision: 0 },
    chapters: { chapter_uuid: uuid, chapter_code: 'v01.ch01', title: 'x', content: 'x', expected_revision: 0 },
    settings: { setting_uuid: uuid, asset_type: 'character', title: 'x', summary: 'x', variant_uuid: uuid, expected_revision: 0 },
    pages: { chapter_uuid: uuid, page_uuid: uuid, title: 'x', description_md: 'x', storyboard_md: 'x', variant_uuid: uuid, expected_revision: 0, verdict: 'passed', observation: 'Fixture visual inspection.' },
    render_prompt: { prompt_group: 'story', prompt_key: 'story_profile', target: 'story_profile' }, export_book: { format: 'markdown', allow_missing_images: true },
    image_target: { target_uuid: uuid, slot: 'page_image' }, read_image: { asset_uuid: uuid },
    prepare_page_image: { page_uuid: uuid, setting_uuids: [] },
    receive_media: { target_uuid: uuid, slot: 'setting_image', expected_revision: 0 },
    setting_boards: { board_uuid: uuid },
    apply_setting_breakdown: { board_uuid: uuid, expected_revision: 0, expected_premise_revision: 0, idempotency_key: 'split', assets: [{ type: 'character', title: 'Fox', summary: 'Fox', tags: ['fox'], confidence: 1, crop_box: { x: 0, y: 0, width: 1, height: 1 } }] },
  };
  for (const name of allowed) {
    for (const action of specs[name].input_schema.properties.action?.enum || [undefined]) {
      const args = { ...input[name], ...(action ? { action } : {}) };
      const write = ['project_init', 'export_book', 'export_website', 'receive_media', 'apply_setting_breakdown'].includes(name) || (action !== undefined && !['get', 'list'].includes(action));
      assert.equal(isWrite(name, args), write, `${name}/${action}`);
    }
  }
});

test('page visual review evidence passes unchanged while attachment slots reject mixed or forged imports', () => {
  const review = { setting_uuid: uuid, variant_uuid: uuid, expected_revision: 0, style_digest: 'a'.repeat(64), verdict: 'passed', reference_basis: 'User linework reference', comparison: 'Thin contours and flat ochre match the original.' };
  const preparation = { project_uuid: uuid, page_uuid: uuid, expected_revision: 0, setting_uuids: [uuid], setting_reviews: [review], additional_direction: '', digest: 'a'.repeat(64), signature: 'b'.repeat(64) };
  validate('prepare_page_image', { page_uuid: uuid, setting_uuids: [uuid], setting_reviews: [review] });
  const page = { target_uuid: uuid, slot: 'page_image', expected_revision: 0, prompt: 'complete prompt', preparation };
  assert.equal(validate('receive_media', page).preparation.setting_reviews[0], review);
  const board = { slot: 'setting_board', source_text: 'Fox and lantern', prompt: 'Complete board prompt', expected_revision: 0, idempotency_key: 'board-a' };
  validate('receive_media', board);
  for (const invalid of [
    { ...board, target_uuid: uuid }, { ...board, preparation }, { ...board, path: '/tmp/forged' },
    { ...board, idempotency_key: '' }, { ...page, source_text: 'mixed' },
    { ...page, preparation: { ...preparation, setting_reviews: [{ ...review, comparison: '' }] } },
    { target_uuid: uuid, slot: 'page_image', expected_revision: 0 },
  ]) assert.throws(() => validate('receive_media', invalid), { code: 'invalid_arguments' });
});
