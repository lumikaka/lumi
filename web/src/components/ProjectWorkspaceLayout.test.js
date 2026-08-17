import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./ProjectWorkspaceLayout.jsx', import.meta.url), 'utf8')
const styles = readFileSync(new URL('../styles/shell.sass', import.meta.url), 'utf8')

test('desktop workbench keeps conversation before canvas in document order', () => {
  assert.ok(source.indexOf('project-chat-host') < source.indexOf('<section className="project-canvas"'))
})

test('canvas fullscreen hides the mounted conversation without discarding its draft state', () => {
  assert.match(source, /const chatMounted = !hideChat/)
  assert.match(source, /chatHidden \? 'project-workbench--solo'/)
  assert.match(styles, /\.project-workbench--solo[\s\S]*?> \.project-chat-host[\s\S]*?display: none/)
})

test('responsive overlay reuses the mounted ChatArea instance', () => {
  assert.match(source, /project-chat-host\$\{compact && overlayOpen \? ' is-overlay'/)
  assert.match(source, /expanded=\{compact \|\| !collapsed \|\| canvasCollapsed\}/)
  assert.doesNotMatch(source, /project-chat-overlay__panel[^\n]*<ChatArea/)
  assert.match(styles, /\.project-chat-host\.is-overlay[\s\S]*?position: fixed/)
})

test('canvas chrome uses assets exported from the Figma workspace frame', () => {
  assert.match(source, /figma\/workspace\/canvas-collapse\.svg/)
  assert.match(source, /figma\/workspace\/canvas-fullscreen\.svg/)
  assert.match(source, /figma\/workspace\/panel-menu\.svg/)
  assert.match(source, /project-canvas__heading/)
  assert.match(source, /projects\.title/)
})

test('canvas creation requests reopen the real conversation composer', () => {
  assert.match(source, /composerDraftRequest=\{composerDraftRequest\}/)
  assert.match(source, /if \(!composerDraftRequest\?\.id \|\| hideChat\) return/)
  assert.match(source, /setCanvasFullscreen\(false\)/)
  assert.match(source, /setOverlayOpen\(true\)/)
  assert.match(source, /setCollapsed\(false\)/)
})
