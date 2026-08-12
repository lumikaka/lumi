import assert from 'node:assert/strict'
import test from 'node:test'

import { extractReadablePrompt, extractReadableResponse } from './llmLogPrompt.js'

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
