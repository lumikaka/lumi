import assert from 'node:assert/strict'
import test from 'node:test'

import { RESOURCES } from '../i18n/messages/index.js'
import {
  extractReadablePrompt,
  extractReadableResponse,
  isProviderResponseDiagnostic,
  providerDiagnosticPreview,
  providerDiagnosticReasonMessageKey,
} from './llmLogPrompt.js'

test('extractReadablePrompt preserves decoded line breaks and blank lines', () => {
  const requestPayload = JSON.parse('{"prompt":"第一行\\n\\n第三行"}')

  assert.equal(extractReadablePrompt(requestPayload), '第一行\n\n第三行')
})

test('extractReadablePrompt normalizes CRLF without trimming prompt content', () => {
  assert.equal(extractReadablePrompt({ prompt: '  第一行\r\n第二行\r结尾  ' }), '  第一行\n第二行\n结尾  ')
})

test('extractReadablePrompt does not reinterpret literal backslash-n text', () => {
  assert.equal(extractReadablePrompt({ prompt: String.raw`第一行\n第二行` }), String.raw`第一行\n第二行`)
})

test('extractReadablePrompt rejects empty, missing, and non-string prompts', () => {
  assert.equal(extractReadablePrompt({ prompt: ' \r\n ' }), '')
  assert.equal(extractReadablePrompt({}), '')
  assert.equal(extractReadablePrompt(null), '')
  assert.equal(extractReadablePrompt({ prompt: ['第一行', '第二行'] }), '')
})

test('extractReadablePrompt formats chat messages as a readable role transcript', () => {
  assert.equal(
    extractReadablePrompt({
      messages: [
        { role: 'system', content: '系统规则\r\n第二行' },
        { role: 'user', content: '用户输入' },
        { role: 'assistant', content: ' \n ' },
        { role: 'tool', content: '{"success":true}' },
      ],
    }),
    '[system]\n系统规则\n第二行\n\n[user]\n用户输入\n\n[tool]\n{"success":true}',
  )
})

test('extractReadablePrompt includes direct and OpenAI-style tool calls', () => {
  assert.equal(
    extractReadablePrompt({
      messages: [
        {
          role: 'assistant',
          tool_calls: [
            { name: 'image_gen', arguments: '{"prompt":"花朵"}' },
            { function: { name: 'save_asset', arguments: { title: '花朵' } } },
          ],
        },
      ],
    }),
    '[assistant]\n[tool_call: image_gen]\n{"prompt":"花朵"}\n\n[tool_call: save_asset]\n{\n  "title": "花朵"\n}',
  )
})

test('extractReadablePrompt reads text parts and ignores non-text message parts', () => {
  assert.equal(
    extractReadablePrompt({
      messages: [{
        role: 'user',
        content: [
          { type: 'text', text: '第一段' },
          { type: 'image_url', image_url: { url: 'data:image/png;base64,hidden' } },
          { type: 'input_text', text: '第二段\r\n结尾' },
        ],
      }],
    }),
    '[user]\n第一段\n第二段\n结尾',
  )
})

test('extractReadablePrompt prefers a top-level prompt over chat messages', () => {
  assert.equal(
    extractReadablePrompt({ prompt: '旧格式正文', messages: [{ role: 'user', content: '聊天正文' }] }),
    '旧格式正文',
  )
})

test('extractReadablePrompt rejects chat payloads without readable text', () => {
  assert.equal(extractReadablePrompt({ messages: [] }), '')
  assert.equal(extractReadablePrompt({ messages: [{ role: 'user', content: null }] }), '')
  assert.equal(extractReadablePrompt({ messages: [{ role: 'user', content: [{ type: 'image_url' }] }] }), '')
})

test('extractReadableResponse supports text and chat response snapshots', () => {
  assert.equal(extractReadableResponse({ content: '第一行\r\n第二行' }), '第一行\n第二行')
  assert.equal(extractReadableResponse({ message: { content: '回复正文\n\n结尾' } }), '回复正文\n\n结尾')
})

test('extractReadableResponse rejects empty and non-text response snapshots', () => {
  assert.equal(extractReadableResponse({ content: ' \n ' }), '')
  assert.equal(extractReadableResponse({ message: { content: null } }), '')
  assert.equal(extractReadableResponse({ mime_type: 'image/png', byte_size: 1024 }), '')
  assert.equal(extractReadableResponse(null), '')
})

test('provider diagnostics are never treated as readable assistant responses', () => {
  const diagnostic = {
    snapshot_type: 'provider_response_diagnostic',
    schema_version: 1,
    reason: 'malformed_json',
    preview: '{"choices": [',
  }
  assert.equal(isProviderResponseDiagnostic(diagnostic), true)
  assert.equal(extractReadableResponse(diagnostic), '')
  assert.equal(isProviderResponseDiagnostic({ ...diagnostic, schema_version: 2 }), false)
})

test('provider diagnostic preview preserves the complete bounded 8 KiB text', () => {
  const preview = '诊'.repeat(Math.floor(8192 / 3)) + 'ab'
  const diagnostic = {
    snapshot_type: 'provider_response_diagnostic',
    schema_version: 1,
    preview,
  }

  assert.equal(Buffer.byteLength(preview), 8192)
  assert.equal(providerDiagnosticPreview(diagnostic), preview)
  assert.equal(providerDiagnosticPreview({ ...diagnostic, schema_version: 2 }), '')
  assert.equal(providerDiagnosticPreview({ ...diagnostic, preview: null }), '')
})

test('every provider diagnostic reason resolves to distinct Chinese and English labels', () => {
  const reasons = [
    'empty_body', 'body_read_error', 'body_too_large', 'malformed_json', 'trailing_json',
    'empty_choices', 'empty_message', 'missing_tool_call_id', 'duplicate_tool_call_id',
    'missing_tool_name', 'tool_arguments_wrong_type', 'tool_arguments_too_large', 'finish_reason_length',
    'negative_usage', 'request_user_input_not_exclusive',
  ]

  for (const reason of reasons) {
    const key = providerDiagnosticReasonMessageKey(reason)
    assert.equal(key, `settings.llm_logs.diagnostic.reason.${reason}`)
    assert.ok(RESOURCES['zh-Hans'][key], `missing zh-Hans label for ${reason}`)
    assert.ok(RESOURCES.en[key], `missing English label for ${reason}`)
    assert.notEqual(RESOURCES['zh-Hans'][key], RESOURCES.en[key])
  }
  assert.equal(providerDiagnosticReasonMessageKey('future_reason'), '')
})

test('body_truncated is labeled as response-body truncation in both locales', () => {
  assert.equal(RESOURCES['zh-Hans']['settings.llm_logs.diagnostic.body_truncated'], '响应 Body 已截断')
  assert.equal(RESOURCES.en['settings.llm_logs.diagnostic.body_truncated'], 'Response body truncated')
})
