import assert from 'node:assert/strict'
import test from 'node:test'

import { initializationChatSearch, projectInitializationControls } from './projectInitialization.js'

test('running initialization permits cancellation without manual takeover or retry', () => {
  for (const status of ['queued', 'running']) {
    assert.deepEqual(projectInitializationControls({ status, locked: true, workflow_uuid: 'workflow', can_take_over: true }), {
      locked: true, canCancel: true, canRetry: false, canTakeOver: false,
    })
  }
})

test('failed, cancelled, and interrupted initialization keep the project locked until an explicit choice', () => {
  for (const status of ['failed', 'cancelled', 'interrupted']) {
    assert.deepEqual(projectInitializationControls({ status, locked: true, workflow_uuid: 'workflow', can_take_over: true }), {
      locked: true, canCancel: false, canRetry: true, canTakeOver: true,
    })
    assert.equal(projectInitializationControls({ status, locked: true, workflow_uuid: 'workflow', can_take_over: false }).canTakeOver, false)
  }
})

test('completed or handed-off initialization opens editing and cannot restart historical initialization', () => {
  for (const status of ['idle', 'completed', 'handed_off']) {
    assert.deepEqual(projectInitializationControls({ status, locked: false, workflow_uuid: 'workflow', can_take_over: false }), {
      locked: false, canCancel: false, canRetry: false, canTakeOver: false,
    })
  }
})

test('initialization navigation pins its original conversation and removes new-chat entry parameters', () => {
  const current = 'workspace_tab=story&chat_thread_uuid=another&workflow_uuid=another-workflow&chat_new=1&chat_reference_type=chapter&chat_reference_uuid=chapter&chat_reference_title=chapter-title'
  const next = initializationChatSearch(current, { locked: true, thread_uuid: 'initial-thread', workflow_uuid: 'initial-workflow' })
  assert.equal(next.get('workspace_tab'), 'story')
  assert.equal(next.get('chat_thread_uuid'), 'initial-thread')
  assert.equal(next.get('workflow_uuid'), 'initial-workflow')
  for (const key of ['chat_new', 'chat_reference_type', 'chat_reference_uuid', 'chat_reference_title']) assert.equal(next.has(key), false)
  assert.equal(initializationChatSearch(current, { locked: false }).toString(), current)
})
