import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

import { isDialogOverlayClick, requestDialogDismiss } from './dialogOverlay.js'

function clickEvent({ clientX, clientY, descendant = false }) {
  const dialog = {
    getBoundingClientRect: () => ({ left: 100, right: 500, top: 80, bottom: 420 }),
  }
  return {
    clientX,
    clientY,
    currentTarget: dialog,
    target: descendant ? {} : dialog,
  }
}

function jsxFiles(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) return jsxFiles(path)
    return entry.isFile() && entry.name.endsWith('.jsx') ? [path] : []
  })
}

test('dialog overlay clicks are outside the rendered dialog bounds', () => {
  assert.equal(isDialogOverlayClick(clickEvent({ clientX: 99, clientY: 200 })), true)
  assert.equal(isDialogOverlayClick(clickEvent({ clientX: 501, clientY: 200 })), true)
  assert.equal(isDialogOverlayClick(clickEvent({ clientX: 300, clientY: 79 })), true)
  assert.equal(isDialogOverlayClick(clickEvent({ clientX: 300, clientY: 421 })), true)
})

test('clicks inside dialog content or dialog whitespace are not overlay clicks', () => {
  assert.equal(isDialogOverlayClick(clickEvent({ clientX: 300, clientY: 200, descendant: true })), false)
  assert.equal(isDialogOverlayClick(clickEvent({ clientX: 300, clientY: 200 })), false)
  assert.equal(isDialogOverlayClick(clickEvent({ clientX: 100, clientY: 80 })), false)
  assert.equal(isDialogOverlayClick(clickEvent({ clientX: 500, clientY: 420 })), false)
})

test('dialog dismissal respects the busy-state guard', () => {
  let closeCount = 0
  const onClose = () => { closeCount += 1 }

  requestDialogDismiss(onClose, true)
  assert.equal(closeCount, 0)

  requestDialogDismiss(onClose)
  assert.equal(closeCount, 1)
})

test('application dialogs use the shared LumiDialog component', () => {
  const sourceRoot = fileURLToPath(new URL('../', import.meta.url))
  const componentPath = fileURLToPath(new URL('./LumiDialog.jsx', import.meta.url))
  for (const path of jsxFiles(sourceRoot)) {
    if (path === componentPath) continue
    assert.doesNotMatch(readFileSync(path, 'utf8'), /<dialog\b/, `${path} renders a native dialog directly`)
  }
})
