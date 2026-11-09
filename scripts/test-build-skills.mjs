// Host skill assembly: Codex output stays byte-identical to the committed
// release skill; the Claude Code skill never mentions Codex-only tools.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import test from 'node:test';
import { assemble, build, diff, outputs } from './build-skills.mjs';

test('host blocks keep shared lines and drop marker lines', () => {
  const text = 'a\n<!-- host:codex -->\ncodex\n<!-- host:claude -->\nclaude\n<!-- /host -->\nb\n';
  assert.equal(assemble(text, 'codex'), 'a\ncodex\nb\n');
  assert.equal(assemble(text, 'claude'), 'a\nclaude\nb\n');
  assert.throws(() => assemble('<!-- host:codex -->\nx', 'codex'), /unterminated/);
  assert.throws(() => assemble('<!-- /host -->', 'codex'), /without an open block/);
  assert.throws(() => assemble('x', 'cindy'), /Unknown host/);
});

test('committed Codex and Claude skills match the shared source byte for byte', () => {
  assert.deepEqual(diff('codex'), []);
  assert.deepEqual(diff('claude'), []);
  for (const [rel, bytes] of Object.entries(build('codex'))) {
    assert.ok(readFileSync(join(outputs.codex, rel)).equals(bytes), rel);
  }
});

const codexOnly = [
  'view_image', 'store/load', 'referenced_image_paths', 'num_last_images_to_include', 'imagegen',
  'review_book', 'review_chapters', 'review_chapter', 'review_page', 'review_settings', 'review_candidates',
  'read_app_review', 'read_app_resource', 'write_app_resource', 'read_app_image', 'read_app_image_detail',
  'watch_app_changes', 'read_app_connection', 'Codex',
];

test('Claude skill has no Codex-only tool names and documents the Claude flow', () => {
  const files = build('claude');
  const all = Object.values(files).map(bytes => bytes.toString('utf8')).join('\n');
  for (const name of codexOnly) assert.ok(!all.includes(name), `Claude skill mentions ${name}`);
  assert.doesNotMatch(all, /<!-- (host:|\/host)/);
  const skill = files['create-picture-book/SKILL.md'].toString('utf8');
  for (const want of ['web_url', 'generate_page_image', 'read_image', 'record_image_review', 'lumi init --json', 'Read']) {
    assert.ok(skill.includes(want), `SKILL.md lacks ${want}`);
  }
  const pageImages = files['create-picture-book/references/page-images.md'].toString('utf8');
  for (const want of ['image_target', 'setting_reviews', 'generate_page_image', 'idempotency_key', 'image_provider_unavailable', 'view=providers']) {
    assert.ok(pageImages.includes(want), `page-images.md lacks ${want}`);
  }
  assert.match(files['create-picture-book/references/batch-settings.md'].toString('utf8'), /不得假装批量设定图可用/);
});

test('Codex skill keeps its host tools', () => {
  const skill = build('codex')['create-picture-book/SKILL.md'].toString('utf8');
  for (const want of ['imagegen', 'review_book', 'referenced_image_paths']) assert.ok(skill.includes(want), want);
  assert.ok(!skill.includes('generate_page_image'));
});
