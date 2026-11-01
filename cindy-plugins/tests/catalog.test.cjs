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
  assert.equal(discover({ category: 'media' }).status, 'not_implemented');
  assert.deepEqual(discover({ category: 'review' }).tools, []);
  for (const name of ['read_app_connection', 'watch_app_changes', 'read_app_image', 'import_image', 'setting_boards', 'review_book', 'exec', 'tools/call']) assert.equal(allowed.has(name), false);
});
test('unknown tools and invalid arguments carry schema and usable correction rules', () => {
  for (const [name, args] of [
    ['pages', { action: 'execute' }], ['pages', { action: 'create', chapter_uuid: uuid }],
    ['pages', { action: 'list', chapter_uuid: uuid, limit: 101 }],
    ['pages', { action: 'get', chapter_uuid: uuid, page_uuid: '1' }],
    ['story_profile', { action: 'write', expected_revision: 1.5, story_md: 'x' }],
    ['project_setup', { action: 'update', expected_revision: 1, picture_book: { format: 'comic_story', unknown: true } }],
    ['project_overview', JSON.parse('{"__proto__":{}}')],
    ['book_outline', { after: '1', before: '2' }],
    ['render_prompt', { prompt_group: 'premise', prompt_key: 'setting_image', target: 'setting_board' }],
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
    pages: { chapter_uuid: uuid, page_uuid: uuid, title: 'x', description_md: 'x', storyboard_md: 'x', variant_uuid: uuid, expected_revision: 0 },
    render_prompt: { prompt_group: 'story', prompt_key: 'story_profile', target: 'story_profile' }, export_book: { format: 'markdown', allow_missing_images: true },
  };
  for (const name of allowed) {
    for (const action of specs[name].input_schema.properties.action?.enum || [undefined]) {
      const args = { ...input[name], ...(action ? { action } : {}) };
      const write = ['project_init', 'export_book'].includes(name) || (action !== undefined && !['get', 'list'].includes(action));
      assert.equal(isWrite(name, args), write, `${name}/${action}`);
    }
  }
});
