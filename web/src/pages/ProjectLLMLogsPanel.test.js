import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./ProjectLLMLogsPanel.jsx', import.meta.url), 'utf8')
const messages = readFileSync(new URL('../i18n/messages/settings.js', import.meta.url), 'utf8')

test('unified AI logs label all call families and distinguish image requests', () => {
  for (const scenario of [
    'story_chapter_generation', 'story_profile_generation', 'story_profile_from_chapters',
    'comic_storyboard_generation', 'premise_setting_generation', 'premise_asset_breakdown',
    'premise_asset_generation', 'comic_reference_selection', 'comic_image_generation',
  ]) {
    assert.match(messages, new RegExp(`settings\\.llm_logs\\.scenario\\.${scenario}`))
  }
  assert.match(source, /log\.request_type === 'image' \? '—'/)
  assert.match(source, /settings\.llm_logs\.request_type/)
})

test('LLM logs expose combinable server filters and nullable usage metrics', () => {
  for (const filter of ['providerUuid', 'providerType', 'model', 'scenario', 'status', 'requestType', 'keyword']) {
    assert.match(source, new RegExp(filter))
  }
  assert.match(source, /filter_groups/)
  for (const metric of ['cached_input_tokens', 'input_characters', 'output_characters', 'output_tokens_per_second', 'output_characters_per_second']) {
    assert.match(source, new RegExp(metric))
  }
  assert.match(source, /value == null \? '—'/)
})

test('failed AI calls render diagnostics as React text fields', () => {
  for (const field of ['error_message', 'http_status', 'provider_error_code', 'provider_request_id']) {
    assert.match(source, new RegExp(`displayLog\\.${field}`))
  }
  assert.match(source, /localizedErrorPresentation\(t, \{ code: displayLog\.error_code \}\)/)
  assert.doesNotMatch(source, /dangerouslySetInnerHTML/)
})

test('log dialog loads full payload details on demand and falls back to legacy summaries', () => {
  assert.match(source, /getProjectLLMLog\(projectUuid, log\.uuid\)/)
  assert.match(source, /queryKey: \['project-llm-log', projectUuid, log\.uuid\]/)
  assert.match(source, /query\.state\.data\?\.status === 'pending' \? 2000 : false/)
  assert.match(source, /detail\?\.request_payload/)
  assert.match(source, /detail\?\.response/)
  assert.match(source, /log\.input_summary/)
  assert.match(source, /log\.output_summary/)
  assert.match(source, /JSON\.stringify\(value, null, 2\)/)
  for (const key of ['request_payload', 'response', 'legacy_payload_notice', 'detail_load_failed', 'response_pending', 'response_unavailable']) {
    assert.match(messages, new RegExp(`settings\\.llm_logs\\.${key}`))
  }
})

test('log dialog opens a safe readable prompt dialog when request payload has a prompt', () => {
  assert.match(source, /extractReadablePrompt\(detail\?\.request_payload\)/)
  assert.match(source, /settings\.llm_logs\.read_log/)
  assert.match(source, /overview-llm-reader-dialog/)
  assert.match(source, /openReader === 'prompt' && readablePrompt/)
  assert.match(source, /<pre className="overview-llm-readable-log" data-user-content>\{content\}<\/pre>/)
  assert.doesNotMatch(source, /dangerouslySetInnerHTML/)
  for (const key of ['read_log', 'prompt_reader_title', 'prompt_reader_description']) {
    assert.match(messages, new RegExp(`settings\\.llm_logs\\.${key}`))
  }
})

test('response payload exposes the same reader for text and chat response content', () => {
  assert.match(source, /extractReadableResponse\(detail\?\.response\)/)
  assert.match(source, /setOpenReader\('response'\)/)
  assert.match(source, /openReader === 'response' && readableResponse/)
  for (const key of ['response_reader_title', 'response_reader_description']) {
    assert.match(messages, new RegExp(`settings\\.llm_logs\\.${key}`))
  }
})
