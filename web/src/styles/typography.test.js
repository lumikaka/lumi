import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const stylesDirectory = fileURLToPath(new URL('.', import.meta.url))
const tokenSource = readFileSync(new URL('./design-tokens.sass', import.meta.url), 'utf8')
const commonSource = readFileSync(new URL('./common.sass', import.meta.url), 'utf8')

test('typography tokens keep the body and minimum text baselines', () => {
  assert.match(tokenSource, /^\$font-size-body: 14px$/m)
  assert.match(tokenSource, /^\$font-size-small: 12px$/m)
  assert.match(tokenSource, /^\$font-size-micro: 11px$/m)
  assert.match(tokenSource, /^\$font-weight-regular: 430$/m)
  assert.match(tokenSource, /^\$font-weight-medium: 500$/m)
  assert.match(tokenSource, /^\$font-weight-semibold: 600$/m)
})

test('Sass font literals do not bypass the 12px text baseline', () => {
  const sassFiles = readdirSync(stylesDirectory).filter((file) => file.endsWith('.sass'))
  const declarationPattern = /(?:font-size|font)\s*:\s*(\d+(?:\.\d+)?px|\.\d+rem)/g

  for (const file of sassFiles) {
    const source = readFileSync(new URL(`./${file}`, import.meta.url), 'utf8')

    for (const match of source.matchAll(declarationPattern)) {
      const value = match[1]
      const pixels = value.endsWith('rem') ? Number.parseFloat(value) * 16 : Number.parseFloat(value)
      const line = source.slice(0, match.index).split('\n').length

      assert.ok(pixels === 0 || pixels >= 12, `${file}:${line} uses ${value}; use a typography token instead`)
    }
  }
})

test('workspace group tabs use the shared body size', () => {
  assert.ok(commonSource.includes('.workspace-group-tabs\n  a,\n  button\n    font-size: $font-size-body'))
})
