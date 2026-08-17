import assert from 'node:assert/strict'
import test from 'node:test'

import { readPinnedProjects, threadReadState, writePinnedProjects } from './sidebarPreferences.js'

test('thread read state prioritizes running work and detects updates after last read', () => {
  const thread = { uuid: 'thread-a', status: 'busy', updated_at: '2026-08-13T12:00:00Z' }
  assert.equal(threadReadState(thread, { 'thread-a': '2026-08-13T13:00:00Z' }), 'in_progress')
  assert.equal(threadReadState({ ...thread, status: 'idle' }, { 'thread-a': '2026-08-13T11:00:00Z' }), 'unread')
  assert.equal(threadReadState({ ...thread, status: 'idle' }, { 'thread-a': '2026-08-13T13:00:00Z' }), 'read')
})

test('project pins are stored once and tolerate unavailable storage', (t) => {
  const values = new Map()
  global.window = { localStorage: { getItem: (key) => values.get(key) || null, setItem: (key, value) => values.set(key, value) } }
  t.after(() => { delete global.window })
  writePinnedProjects(['project-a', 'project-a', 'project-b'])
  assert.deepEqual(readPinnedProjects(), ['project-a', 'project-b'])
})
