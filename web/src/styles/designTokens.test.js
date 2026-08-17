import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const tokenSource = readFileSync(new URL('./design-tokens.sass', import.meta.url), 'utf8')
const commonSource = readFileSync(new URL('./common.sass', import.meta.url), 'utf8')
const workspaceSource = readFileSync(new URL('./workspaces.sass', import.meta.url), 'utf8')
const chatSource = readFileSync(new URL('./chat.sass', import.meta.url), 'utf8')

test('the UI token system keeps brand emphasis restrained and primary controls neutral', () => {
  assert.match(tokenSource, /^\$color-brand: #ffb411$/m)
  assert.match(tokenSource, /^\$color-control-primary: #212121$/m)
  assert.match(commonSource, /button:not\(\[class\]\),[\s\S]*?border: 1px solid \$color-control-primary[\s\S]*?background: \$color-control-primary/)
  assert.match(commonSource, /:focus-visible[\s\S]*?outline: 2px solid \$color-brand/)
  assert.doesNotMatch(commonSource, /#1d4ed8|#dce8ff|#eaf1ff|#0088ff/)
})

test('compact text controls share height, padding, and radius tokens', () => {
  assert.match(tokenSource, /^\$control-height-compact: 29px$/m)
  assert.match(tokenSource, /^\$control-padding-block-compact: 0$/m)
  assert.match(tokenSource, /^\$control-padding-inline-compact: 12px$/m)
  assert.match(tokenSource, /^\$control-radius-compact: \$radius-md$/m)

  assert.match(workspaceSource, /\.premise-import__trigger[\s\S]*?height: \$control-height-compact[\s\S]*?padding: \$control-padding-block-compact \$control-padding-inline-compact/)
  assert.match(workspaceSource, /\.premise-add__trigger[\s\S]*?height: \$control-height-compact[\s\S]*?border-radius: \$control-radius-compact/)
  assert.match(workspaceSource, /\.premise-empty-state[\s\S]*?> button[\s\S]*?height: \$control-height-compact[\s\S]*?border-radius: \$control-radius-compact/)
  assert.match(workspaceSource, /\.premise-type-filters[\s\S]*?button[\s\S]*?height: \$control-height-compact[\s\S]*?padding: \$control-padding-block-compact \$control-padding-inline-compact[\s\S]*?border-radius: \$control-radius-compact/)
  assert.match(chatSource, /\.chat-input-request[\s\S]*?footer[\s\S]*?> button[\s\S]*?height: \$control-height-compact[\s\S]*?padding: \$control-padding-block-compact \$control-padding-inline-compact[\s\S]*?border-radius: \$control-radius-compact/)
})
