import assert from 'node:assert/strict'
import test from 'node:test'

import {
  abortChatTurn,
  cancelWorkflow,
  createChatThread,
  createChatTurn,
  createFollowUp,
  createYoloWorkflow,
  getChatTrajectory,
  listChatThreads,
  listChatEvents,
  listChatItems,
  listWorkflowEvents,
  listWorkflowLLMLogs,
  listWorkflowRuns,
  moveFollowUp,
  respondUserInput,
  resolveWorkflowConflict,
  retryWorkflow,
  steerChatRun,
  steerFollowUp,
  updateFollowUp,
} from './chat.js'

function envelope(data) {
  return new Response(JSON.stringify({ success: true, data }), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

test('chat resources use project-scoped UUID routes and snake_case payloads', async () => {
  const premiseReference = { resource_type: 'premise_asset', resource_uuid: 'asset-uuid' }
  const fileReference = { resource_type: 'file', resource_uuid: 'file-uuid' }
  const calls = []
  const original = global.fetch
  global.fetch = async (url, options = {}) => { calls.push([url, options]); return envelope({ uuid: 'public-uuid' }) }
  try {
    await listChatThreads('project uuid')
    await createChatThread('project uuid', { title: 'Asset' })
    await createChatTurn('project uuid', 'thread uuid', { input_text: 'hello', references: [premiseReference] })
    await createFollowUp('project uuid', 'thread uuid', 'next', [fileReference])
    await moveFollowUp('project uuid', 'thread uuid', 'follow uuid', 2)
    await updateFollowUp('project uuid', 'thread uuid', 'follow uuid', 'edited')
    await steerFollowUp('project uuid', 'thread uuid', 'follow uuid')
    await steerChatRun('project uuid', 'thread uuid', 'change direction', [premiseReference, fileReference])
    await abortChatTurn('project uuid', 'thread uuid')
    await respondUserInput('project uuid', 'thread uuid', 'request uuid', { answers: { art_style: { selected_option_uuid: 'option-uuid', other_text: '' } } })
  } finally { global.fetch = original }

  assert.equal(calls[0][0], '/api/v1/projects/project%20uuid/chat_threads?page=1&per_page=30')
  assert.equal(calls[1][0], '/api/v1/projects/project%20uuid/chat_threads')
  assert.deepEqual(JSON.parse(calls[1][1].body), { title: 'Asset' })
  assert.equal(calls[2][0], '/api/v1/projects/project%20uuid/chat_threads/thread%20uuid/turns')
  assert.deepEqual(JSON.parse(calls[2][1].body), { input_text: 'hello', references: [premiseReference] })
  assert.equal(calls[3][0], '/api/v1/projects/project%20uuid/chat_threads/thread%20uuid/follow_ups')
  assert.deepEqual(JSON.parse(calls[3][1].body), { input_text: 'next', references: [fileReference] })
  assert.equal(calls[4][0], '/api/v1/projects/project%20uuid/chat_threads/thread%20uuid/follow_ups/follow%20uuid/position')
  assert.equal(calls[5][0], '/api/v1/projects/project%20uuid/chat_threads/thread%20uuid/follow_ups/follow%20uuid')
  assert.deepEqual(JSON.parse(calls[5][1].body), { input_text: 'edited' })
  assert.equal(calls[6][0], '/api/v1/projects/project%20uuid/chat_threads/thread%20uuid/follow_ups/follow%20uuid/steerings')
  assert.equal(calls[7][0], '/api/v1/projects/project%20uuid/chat_threads/thread%20uuid/steerings')
  assert.deepEqual(JSON.parse(calls[7][1].body), { input_text: 'change direction', references: [premiseReference, fileReference] })
  assert.equal(calls[8][0], '/api/v1/projects/project%20uuid/chat_threads/thread%20uuid/cancellations')
  assert.equal(calls[9][0], '/api/v1/projects/project%20uuid/chat_threads/thread%20uuid/user_input_requests/request%20uuid/responses')
  assert.deepEqual(JSON.parse(calls[9][1].body), { answers: { art_style: { selected_option_uuid: 'option-uuid', other_text: '' } } })
  assert.equal(calls[0][1].method, undefined)
  assert.ok(calls.slice(1).every(([, options]) => ['POST', 'PATCH'].includes(options.method)))
  assert.deepEqual(JSON.parse(calls[4][1].body), { position: 2 })
})

test('chat and workflow recovery reads use cursors and resource actions', async () => {
  const calls = []
  const original = global.fetch
  global.fetch = async (url, options = {}) => { calls.push([url, options]); return envelope({ items: [] }) }
  try {
    await listChatItems('project-uuid', 'thread-uuid', { before: 'cursor-a', limit: 25 })
    await listChatEvents('project-uuid', 'thread-uuid', { after: 'cursor-b', limit: 50 })
    await getChatTrajectory('project uuid', 'thread uuid', { before: 'cursor-c', limit: 80 })
    await getChatTrajectory('project uuid', 'thread uuid', { itemUuid: 'item uuid', limit: 40 })
    await createYoloWorkflow('project-uuid', { title: 'Book', story_prompt: 'idea', provider_uuid: 'provider-uuid', idempotency_key: 'yolo-key-one' })
    await cancelWorkflow('project-uuid', 'workflow-uuid')
    await retryWorkflow('project-uuid', 'workflow-uuid')
    await listWorkflowRuns('project-uuid', 'workflow-uuid', { before: 'runs-cursor', limit: 10 })
    await listWorkflowEvents('project-uuid', 'workflow-uuid', { before: 'events-cursor', limit: 20 })
    await listWorkflowLLMLogs('project-uuid', 'workflow-uuid', { page: 2, perPage: 10, stepUuid: 'step-uuid' })
    await resolveWorkflowConflict('project-uuid', 'workflow-uuid', { action: 'overwrite', expected_comic_state_revision: 7 })
  } finally { global.fetch = original }

  assert.equal(calls[0][0], '/api/v1/projects/project-uuid/chat_threads/thread-uuid/items?limit=25&before=cursor-a')
  assert.equal(calls[1][0], '/api/v1/projects/project-uuid/chat_threads/thread-uuid/events?limit=50&after=cursor-b')
  assert.equal(calls[2][0], '/api/v1/projects/project%20uuid/chat_threads/thread%20uuid/trajectory?limit=80&before=cursor-c')
  assert.equal(calls[3][0], '/api/v1/projects/project%20uuid/chat_threads/thread%20uuid/trajectory?limit=40&item_uuid=item+uuid')
  assert.equal(calls[4][0], '/api/v1/projects/project-uuid/workflows')
  assert.equal(calls[5][0], '/api/v1/projects/project-uuid/workflows/workflow-uuid/cancellations')
  assert.equal(calls[6][0], '/api/v1/projects/project-uuid/workflows/workflow-uuid/retries')
  assert.equal(calls[7][0], '/api/v1/projects/project-uuid/workflows/workflow-uuid/runs?limit=10&before=runs-cursor')
  assert.equal(calls[8][0], '/api/v1/projects/project-uuid/workflows/workflow-uuid/events?limit=20&before=events-cursor')
  assert.equal(calls[9][0], '/api/v1/projects/project-uuid/workflows/workflow-uuid/llm-logs?page=2&per_page=10&workflow_step_uuid=step-uuid')
  assert.equal(calls[10][0], '/api/v1/projects/project-uuid/workflows/workflow-uuid/conflict-resolutions')
  assert.equal(calls[10][1].method, 'POST')
  assert.deepEqual(JSON.parse(calls[10][1].body), { action: 'overwrite', expected_comic_state_revision: 7 })
})
