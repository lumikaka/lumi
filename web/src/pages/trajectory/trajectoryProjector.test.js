import assert from 'node:assert/strict'
import test from 'node:test'

import {
  applyTrajectoryUpserts,
  prependTrajectoryPage,
  replaceTrajectoryProjection,
} from './trajectoryProjector.js'
import { trajectoryKey, trajectorySummaryKey } from './trajectoryIdentity.js'

const uuids = {
  thread: '0198c100-0000-7000-8000-000000000001',
  turn1: '0198c100-0000-7000-8000-000000000002',
  turn2: '0198c100-0000-7000-8000-000000000003',
  user1: '0198c100-0000-7000-8000-000000000004',
  steering: '0198c100-0000-7000-8000-000000000005',
  assistant: '0198c100-0000-7000-8000-000000000006',
  request1: '0198c100-0000-7000-8000-000000000007',
  request2: '0198c100-0000-7000-8000-000000000008',
  request3: '0198c100-0000-7000-8000-000000000009',
  execution: '0198c100-0000-7000-8000-00000000000a',
  call: '0198c100-0000-7000-8000-00000000000b',
  compaction: '0198c100-0000-7000-8000-00000000000c',
}

function turn(uuid = uuids.turn1, sequence = 1, status = 'completed') {
  return { uuid, queue_sequence: sequence, source_type: 'prompt', status, created_at: `2026-08-21T00:00:0${sequence}Z` }
}

function request(uuid, ordinal, overrides = {}) {
  return {
    uuid,
    thread_uuid: uuids.thread,
    turn_uuid: uuids.turn1,
    run_uuid: '0198c100-0000-7000-8000-000000000099',
    request_ordinal: ordinal,
    request_type: 'text',
    provider_uuid: '0198c100-0000-7000-8000-000000000098',
    provider_type: 'test',
    model: 'test-model',
    status: 'completed',
    options: { max_tokens: 4096, stream: false },
    ordering_accuracy: 'exact',
    start_event_sequence: ordinal * 10,
    end_event_sequence: ordinal * 10 + 1,
    created_at: `2026-08-21T00:00:1${ordinal}Z`,
    completed_at: `2026-08-21T00:00:1${ordinal}Z`,
    ...overrides,
  }
}

function page(overrides = {}) {
  return {
    thread: { uuid: uuids.thread, title: 'Trajectory', status: 'idle' },
    turns: [turn()],
    items: [],
    tools: [],
    model_requests: [],
    compactions: [],
    cursor_pagination: { per_page: 80, prev_cursor: 'one', next_cursor: 'tail', has_more: false },
    history_complete: true,
    overview: { turn_count: 1, item_count: 0, timeline: [] },
    ...overrides,
  }
}

test('projects user items into their persisted turn and keeps steering in that turn', () => {
  const projection = replaceTrajectoryProjection(page({
    items: [
      { uuid: uuids.user1, thread_uuid: uuids.thread, turn_uuid: uuids.turn1, sequence: 1, event_sequence: 1, item_type: 'user_message', role: 'user', content: 'Start', content_format: 'text', status: 'completed', metadata: {}, created_at: '2026-08-21T00:00:01Z' },
      { uuid: uuids.steering, thread_uuid: uuids.thread, turn_uuid: uuids.turn1, sequence: 2, event_sequence: 3, item_type: 'user_message', role: 'user', content: 'Steer', content_format: 'text', status: 'completed', metadata: { steering: true }, created_at: '2026-08-21T00:00:03Z' },
    ],
  }))

  assert.equal(projection.items.length, 2)
  assert.equal(projection.items[0].kind, 'user')
  assert.equal(projection.items[1].isSteering, true)
  assert.equal(projection.items[1].turnUuid, uuids.turn1)
  assert.deepEqual(projection.rows.map((row) => row.rowType), ['turn', 'item', 'item'])
})

test('keeps persisted assistant and projects request-only tool-call assistant without duplication', () => {
  const projection = replaceTrajectoryProjection(page({
    items: [{ uuid: uuids.assistant, thread_uuid: uuids.thread, turn_uuid: uuids.turn1, sequence: 4, event_sequence: 31, item_type: 'assistant_message', role: 'assistant', content: 'Done', content_format: 'text', status: 'completed', request_uuid: uuids.request3, request_ordinal: 3, created_at: '2026-08-21T00:00:13Z' }],
    model_requests: [
      request(uuids.request2, 2, { has_response: true, has_tool_calls: true, assistant_preview: 'Tool calls requested' }),
      request(uuids.request3, 3, { has_response: true, has_tool_calls: false, assistant_preview: 'Done' }),
    ],
  }))

  const assistants = projection.items.filter((item) => item.kind === 'assistant')
  assert.equal(assistants.length, 2)
  assert.ok(assistants.some((item) => item.sourceKind === 'model_request_assistant' && item.sourceUuid === uuids.request2))
  assert.ok(assistants.some((item) => item.sourceKind === 'chat_item' && item.sourceUuid === uuids.assistant))
  assert.equal(projection.requests[0].preview, 'Request #2 · test-model')
  assert.ok(projection.rows.some((row) => row.rowType === 'request'))
})

test('merges Tool lifecycle by public call UUID and derives error or interruption from facts', () => {
  const failed = replaceTrajectoryProjection(page({
    tools: [{
      uuid: uuids.execution,
      thread_uuid: uuids.thread,
      turn_uuid: uuids.turn1,
      call_item_uuid: uuids.assistant,
      tool_call_uuid: uuids.call,
      tool_name: 'request_api',
      call_sequence: 2,
      status: 'completed',
      arguments: { method: 'GET', url: '/api/v1/projects/example' },
      result: { success: false, data: null, error: { code: 'failed' } },
      created_at: '2026-08-21T00:00:02Z',
    }],
  }))
  const tool = failed.items.find((item) => item.kind === 'tool')
  assert.equal(tool.sourceUuid, uuids.call)
  assert.equal(tool.key, trajectoryKey('tool', uuids.call))
  assert.equal(tool.status, 'error')

  const interrupted = replaceTrajectoryProjection(page({
    turns: [turn(uuids.turn1, 1, 'failed')],
    tools: [{
      uuid: uuids.execution,
      thread_uuid: uuids.thread,
      turn_uuid: uuids.turn1,
      tool_call_uuid: uuids.call,
      tool_name: 'image_gen',
      call_sequence: 2,
      status: 'running',
      arguments: { prompt: 'moon' },
      created_at: '2026-08-21T00:00:02Z',
    }],
  })).items.find((item) => item.kind === 'tool')
  assert.equal(interrupted.status, 'interrupted')
  assert.match(interrupted.derivedReason, /Turn ended/)
})

test('emits a real initial SYSTEM snapshot plus later digest changes and keeps unknown timing unknown', () => {
  const projection = replaceTrajectoryProjection(page({
    model_requests: [
      request(uuids.request1, 1, { system_prompt_digest: 'system-a', tool_catalog_digest: 'tools-a', duration_ms: undefined, input_tokens: undefined }),
      request(uuids.request2, 2, { system_prompt_digest: 'system-a', tool_catalog_digest: 'tools-a' }),
      request(uuids.request3, 3, { system_prompt_digest: 'system-b', tool_catalog_digest: 'tools-a' }),
    ],
  }))
  const systems = projection.items.filter((item) => item.kind === 'system')
  assert.equal(systems.length, 2)
  assert.equal(systems[0].sourceUuid, uuids.request1)
  assert.equal(systems[0].turnUuid, null)
  assert.equal(systems[0].preview, 'Initial System Prompt')
  assert.equal(systems[0].previousRequestUuid, undefined)
  assert.equal(systems[1].sourceUuid, uuids.request3)
  assert.equal(systems[1].preview, 'System Prompt Updated')
  assert.equal(systems[1].previousRequestUuid, uuids.request2)
  assert.equal(projection.requests[0].durationMs, undefined)
  assert.equal(projection.requests[0].source.input_tokens, undefined)
})

test('keeps unassigned compaction at Thread level with stable UUID identity', () => {
  const projection = replaceTrajectoryProjection(page({
    compactions: [{ uuid: uuids.compaction, thread_uuid: uuids.thread, turn_uuid: null, through_item_sequence: 9, summary: 'Earlier facts', source_bytes: 1000, ordering_accuracy: 'approximate', created_at: '2026-08-21T00:00:09Z' }],
  }))
  const compaction = projection.items[0]
  assert.equal(compaction.kind, 'compaction')
  assert.equal(compaction.turnUuid, null)
  assert.equal(compaction.key, trajectoryKey('compaction', uuids.compaction))
  assert.equal(projection.rows[0].rowType, 'item')
})

test('prepend and upsert preserve stable identities while updating lifecycle state', () => {
  const tail = replaceTrajectoryProjection(page({
    history_complete: false,
    items: [{ uuid: uuids.assistant, thread_uuid: uuids.thread, turn_uuid: uuids.turn2, sequence: 9, item_type: 'assistant_message', role: 'assistant', content: 'Tail', status: 'completed', created_at: '2026-08-21T00:00:09Z' }],
    turns: [turn(uuids.turn1, 1), turn(uuids.turn2, 2)],
    tools: [{ uuid: uuids.execution, thread_uuid: uuids.thread, turn_uuid: uuids.turn2, tool_call_uuid: uuids.call, tool_name: 'image_gen', call_sequence: 8, status: 'running', arguments: { prompt: 'moon' }, created_at: '2026-08-21T00:00:08Z' }],
  }))
  const originalToolKey = tail.items.find((item) => item.kind === 'tool').key
  const prepended = prependTrajectoryPage(tail, page({
    items: [{ uuid: uuids.user1, thread_uuid: uuids.thread, turn_uuid: uuids.turn1, sequence: 1, item_type: 'user_message', role: 'user', content: 'Older', status: 'completed', created_at: '2026-08-21T00:00:01Z' }],
    cursor_pagination: { per_page: 80, prev_cursor: 'oldest', next_cursor: 'middle', has_more: false },
    history_complete: true,
  }))
  assert.equal(prepended.historyComplete, true)
  assert.equal(prepended.items.find((item) => item.kind === 'tool').key, originalToolKey)
  assert.ok(prepended.items.some((item) => item.sourceUuid === uuids.user1))

  const updated = applyTrajectoryUpserts(prepended, page({
    tools: [{ uuid: uuids.execution, thread_uuid: uuids.thread, turn_uuid: uuids.turn2, tool_call_uuid: uuids.call, tool_name: 'image_gen', call_sequence: 8, status: 'completed', arguments: { prompt: 'moon' }, result: { success: true }, created_at: '2026-08-21T00:00:08Z', completed_at: '2026-08-21T00:00:10Z', duration_ms: 2000 }],
  }))
  const updatedTool = updated.items.find((item) => item.kind === 'tool')
  assert.equal(updatedTool.key, originalToolKey)
  assert.equal(updatedTool.status, 'completed')
  assert.equal(updatedTool.durationMs, 2000)
  assert.equal(trajectorySummaryKey('tool_group', uuids.turn2, [updatedTool.key]), `summary:tool_group:${uuids.turn2}:${updatedTool.key}`)
})
