import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import { codeMirrorSearchPhrases } from './codeMirrorSearchPhrases.js'

test('CodeMirror search phrases localize native find and replace controls', () => {
  const phrases = codeMirrorSearchPhrases((key) => `zh:${key}`)
  assert.equal(phrases.Find, 'zh:comic.workbench.search.find')
  assert.equal(phrases.Replace, 'zh:comic.workbench.search.replace')
  assert.equal(phrases['replace all'], 'zh:comic.workbench.search.replace_all')
  assert.equal(phrases['current match'], 'zh:comic.workbench.search.current_match')
  assert.equal(phrases['Go to line'], 'zh:comic.workbench.search.go_to_line')
})

test('MarkdownEditor exposes search, controlled sync, and readonly reconfiguration', () => {
  const source = readFileSync(new URL('./MarkdownEditor.jsx', import.meta.url), 'utf8')
  assert.match(source, /openSearchPanel\(view\)/)
  assert.match(source, /view\.state\.doc\.toString\(\) === nextValue/)
  assert.match(source, /EditorState\.readOnly\.of\(disabled\)/)
  assert.match(source, /editableRef\.current\.reconfigure/)
  assert.match(source, /EditorView\.lineWrapping/)
  assert.match(source, /markdown\(\)/)
})
