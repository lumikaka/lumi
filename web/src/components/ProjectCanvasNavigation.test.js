import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./ProjectCanvasNavigation.jsx', import.meta.url), 'utf8')
const styles = readFileSync(new URL('../styles/shell.sass', import.meta.url), 'utf8')
const tokens = readFileSync(new URL('../styles/design-tokens.sass', import.meta.url), 'utf8')

test('canvas navigation uses the three SVG icons exported by Figma', () => {
  assert.match(source, /canvas-tab-premise\.svg/)
  assert.match(source, /canvas-tab-chapters\.svg/)
  assert.match(source, /canvas-tab-works\.svg/)
  assert.match(source, /activeMode === mode\.key \? <FigmaIcon src=\{mode\.icon\} size=\{20\} \/>/)
})

test('canvas navigation follows the 320 by 48 Figma segmented control frame', () => {
  const navigationStyles = styles.slice(styles.indexOf('.project-canvas-nav'), styles.indexOf('.project-canvas-rail'))
  assert.match(navigationStyles, /width: min\(320px, calc\(100% - 32px\)\)/)
  assert.match(navigationStyles, /height: 48px/)
  assert.match(navigationStyles, /border-radius: 12px/)
  assert.match(navigationStyles, /box-shadow: 0 0 12px rgba\(0, 0, 0, \.1\)/)
  assert.match(navigationStyles, /background: #fef9ed/)
  assert.match(tokens, /^\$font-size-control: 14px$/m)
  assert.match(navigationStyles, /font-size: \$font-size-control/)
})
