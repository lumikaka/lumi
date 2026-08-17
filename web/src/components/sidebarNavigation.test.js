import assert from 'node:assert/strict'
import test from 'node:test'

import {
  DEFAULT_VISIBLE_THREAD_COUNT,
  conversationProject,
  matchingThreads,
  newConversationPath,
  orderedProjects,
  orderedThreads,
  providerConnectionState,
  shouldShowThreadToggle,
  visibleThreads,
} from './sidebarNavigation.js'

const threads = Array.from({ length: 7 }, (_, index) => ({ uuid: `thread-${index + 1}`, title: `Conversation ${index + 1}` }))

test('multiple projects show five conversations until explicitly expanded', () => {
  assert.equal(DEFAULT_VISIBLE_THREAD_COUNT, 5)
  assert.deepEqual(visibleThreads(threads, { projectCount: 2 }).map((item) => item.uuid), threads.slice(0, 5).map((item) => item.uuid))
  assert.equal(visibleThreads(threads, { projectCount: 2, expanded: true }).length, 7)
  assert.equal(shouldShowThreadToggle(threads, { projectCount: 2 }), true)
})

test('a single project and search results are never truncated', () => {
  assert.equal(visibleThreads(threads, { projectCount: 1 }).length, 7)
  assert.equal(visibleThreads(threads, { projectCount: 2, searching: true }).length, 7)
  assert.equal(shouldShowThreadToggle(threads, { projectCount: 1 }), false)
  assert.equal(shouldShowThreadToggle(threads, { projectCount: 2, searching: true }), false)
})

test('conversation search matches titles without broadening from project names', () => {
  assert.deepEqual(matchingThreads(threads, 'conversation 6').map((item) => item.uuid), ['thread-6'])
  assert.deepEqual(matchingThreads(threads, 'unrelated project name'), [])
})

test('pinned conversations move first without changing relative order', () => {
  assert.deepEqual(orderedThreads(threads.slice(0, 4), ['thread-3', 'thread-2']).map((item) => item.uuid), ['thread-2', 'thread-3', 'thread-1', 'thread-4'])
})

test('pinned projects move first without changing relative order', () => {
  const projects = [{ uuid: 'project-1' }, { uuid: 'project-2' }, { uuid: 'project-3' }]
  assert.deepEqual(orderedProjects(projects, ['project-3']).map((item) => item.uuid), ['project-3', 'project-1', 'project-2'])
})

test('provider connection state is derived from real readiness fields', () => {
  assert.equal(providerConnectionState([]), 'missing')
  assert.equal(providerConnectionState([{ configured: true, has_secret: true, ready: false }]), 'needs_verification')
  assert.equal(providerConnectionState([{ active: true, ready: true }]), 'ready')
})

test('new conversation prefers the active, open, then first available project', () => {
  const projects = [
    { uuid: 'recent project', available: true, open: false },
    { uuid: 'open-project', available: true, open: true },
    { uuid: 'active-project', available: true, open: false },
  ]
  assert.equal(conversationProject(projects, 'active-project')?.uuid, 'active-project')
  assert.equal(conversationProject(projects)?.uuid, 'open-project')
  assert.equal(newConversationPath(projects, 'active-project'), '/projects/active-project/premise?chat_scope=project&chat_new=1')
})

test('new conversation starts project creation when no usable project exists', () => {
  const projects = [{ uuid: 'missing-project', available: false, open: false }]
  assert.equal(conversationProject(projects), null)
  assert.equal(newConversationPath(projects), '/?create_project=1&continue=new_conversation')
})
