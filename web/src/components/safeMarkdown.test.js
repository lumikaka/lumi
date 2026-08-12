import assert from 'node:assert/strict'
import test from 'node:test'

import { parseMarkdownBlocks, parseMarkdownInline, sanitizeMarkdownUrl } from './safeMarkdown.js'

test('safe markdown parses the supported chat block shapes', () => {
  const blocks = parseMarkdownBlocks('# 标题\n\n- 一\n- 二\n\n> 引用\n\n```js\nalert(1)\n```')
  assert.deepEqual(blocks.map((block) => block.type), ['heading', 'unordered_list', 'blockquote', 'code'])
  assert.equal(blocks[3].language, 'js')
  assert.equal(blocks[3].text, 'alert(1)')
})

test('safe markdown allows useful links and rejects executable protocols', () => {
  assert.equal(sanitizeMarkdownUrl('https://example.com/a'), 'https://example.com/a')
  assert.equal(sanitizeMarkdownUrl('/projects/one'), '/projects/one')
  assert.equal(sanitizeMarkdownUrl('mailto:user@example.com'), 'mailto:user@example.com')
  assert.equal(sanitizeMarkdownUrl('javascript:alert(1)'), '')
  assert.equal(sanitizeMarkdownUrl('data:text/html,bad'), '')
  assert.deepEqual(parseMarkdownInline('[safe](https://example.com) and `code`').map((token) => token.type), ['link', 'text', 'code'])
  assert.deepEqual(parseMarkdownInline('[bad](javascript:alert)').map((token) => token.type), ['text'])
})

test('raw HTML remains plain text instead of becoming executable markup', () => {
  const blocks = parseMarkdownBlocks('<img src=x onerror=alert(1)>')
  assert.deepEqual(blocks, [{ type: 'paragraph', text: '<img src=x onerror=alert(1)>' }])
})
