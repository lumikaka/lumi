import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import { agentQueryKeysForEvent, shouldPollAgentState, workflowControls } from './chatWorkspaceState.js'

test('realtime payload invalidates persistent thread and workflow recovery queries', () => {
  assert.deepEqual(agentQueryKeysForEvent('project-uuid', { thread_uuid: 'thread-uuid', workflow_uuid: 'workflow-uuid' }), [
    ['chat-threads', 'project-uuid'],
    ['chat-items', 'project-uuid', 'thread-uuid'],
    ['chat-turns', 'project-uuid', 'thread-uuid'],
    ['chat-follow-ups', 'project-uuid', 'thread-uuid'],
    ['chat-input-requests', 'project-uuid', 'thread-uuid'],
    ['chat-events', 'project-uuid', 'thread-uuid'],
    ['chat-thread', 'project-uuid', 'thread-uuid'],
    ['workflows', 'project-uuid'],
    ['workflow', 'project-uuid', 'workflow-uuid'],
    ['workflow-runs', 'project-uuid', 'workflow-uuid'],
    ['workflow-events', 'project-uuid', 'workflow-uuid'],
    ['workflow-llm-logs', 'project-uuid', 'workflow-uuid'],
  ])
  assert.deepEqual(agentQueryKeysForEvent('project-uuid', { thread_uuid: 'thread-uuid' }).map((key) => key[0]), ['chat-threads', 'chat-items', 'chat-turns', 'chat-follow-ups', 'chat-input-requests', 'chat-events', 'chat-thread'])
  assert.deepEqual(agentQueryKeysForEvent('project-uuid', { workflow_uuid: 'workflow-uuid' }).map((key) => key[0]), ['workflows', 'workflow', 'workflow-runs', 'workflow-events', 'workflow-llm-logs'])
  assert.deepEqual(agentQueryKeysForEvent('project-uuid', {}), [])
})

test('polling and workflow controls cover disconnect, retry and cancellation states', () => {
  assert.equal(shouldPollAgentState([{ status: 'waiting_for_input' }], []), true)
  assert.equal(shouldPollAgentState([], [{ status: 'running' }]), true)
  assert.equal(shouldPollAgentState([{ status: 'idle' }], [{ status: 'completed' }]), false)
  assert.deepEqual(workflowControls({ status: 'failed' }), { canCancel: false, canRetry: true })
  assert.deepEqual(workflowControls({ status: 'running' }), { canCancel: true, canRetry: false })
})

test('ChatArea presents comic storyboard workflows with localized kind and step copy', () => {
  const chatArea = readFileSync(new URL('../components/ChatArea.jsx', import.meta.url), 'utf8')
  const messages = readFileSync(new URL('../i18n/messages/chat.js', import.meta.url), 'utf8')
  assert.match(chatArea, /comic_storyboard_generation: 'chat\.workflow\.kind\.comic_storyboard_generation'/)
  assert.match(chatArea, /comic_storyboard: 'chat\.workflow\.step\.comic_storyboard'/)
  assert.match(messages, /'chat\.workflow\.kind\.comic_storyboard_generation': \['漫画分镜生成', 'Comic storyboard generation'\]/)
  assert.match(messages, /'chat\.workflow\.step\.comic_storyboard': \['生成漫画分镜', 'Generate comic storyboard'\]/)
})
