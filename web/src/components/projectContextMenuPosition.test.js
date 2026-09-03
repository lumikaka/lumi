import assert from 'node:assert/strict'
import test from 'node:test'

import { projectContextMenuPosition } from './projectContextMenuPosition.js'

test('project context menus stay inside the viewport near each edge', () => {
  assert.deepEqual(projectContextMenuPosition({
    x: 790,
    y: 590,
    width: 264,
    height: 220,
    viewportWidth: 800,
    viewportHeight: 600,
  }), { left: 528, top: 372 })

  assert.deepEqual(projectContextMenuPosition({
    x: 0,
    y: 0,
    width: 264,
    height: 220,
    viewportWidth: 800,
    viewportHeight: 600,
  }), { left: 8, top: 8 })
})
