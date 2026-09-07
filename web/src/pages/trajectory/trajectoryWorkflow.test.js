import assert from 'node:assert/strict'
import test from 'node:test'

import { applyTrajectoryUpserts, prependTrajectoryPage, replaceTrajectoryProjection } from './trajectoryProjector.js'
import { applyTrajectoryCollapse } from './trajectoryCollapse.js'
import { filterTrajectoryRows, reconcileTrajectorySearchIndex, updateTrajectoryRequestSearchDocument } from './trajectorySearch.js'
import { buildTrajectoryTimeline, trajectoryTimelineEntries } from './trajectoryTimeline.js'
import { trajectoryRequestOrigin, trajectoryWorkflowTitle } from './trajectoryRequestOrigin.js'
import { RESOURCES } from '../../i18n/messages/index.js'

function request(uuid, ordinal, overrides = {}) {
  return {
    uuid,
    thread_uuid: 'workflow-thread',
    source_type: 'production',
    request_type: 'image',
    request_ordinal: ordinal,
    attempt: 1,
    scenario: 'premise_asset_generation',
    model: 'qwen-image-3.0-pro',
    status: 'completed',
    has_response: true,
    output_summary: 'mime_type=image/png; byte_size=100',
    system_prompt_digest: 'image-prompt',
    created_at: `2026-09-07T01:47:0${ordinal}Z`,
    completed_at: `2026-09-07T01:48:0${ordinal}Z`,
    duration_ms: 47061,
    ...overrides,
  }
}

function page(requests, overrides = {}) {
  return {
    thread: { uuid: 'workflow-thread', thread_type: 'workflow' },
    turns: [], items: [], tools: [], compactions: [],
    model_requests: requests,
    history_complete: true,
    overview: {
      model_request_count: requests.length,
      tool_count: 0,
      timeline: requests.map((source) => ({
        uuid: source.uuid, source_kind: 'model_request', kind: 'model_request',
        started_at: source.created_at, completed_at: source.completed_at, duration_ms: source.duration_ms,
        request_uuid: source.uuid, request_ordinal: source.request_ordinal, status: source.status,
      })),
    },
    ...overrides,
  }
}

test('workflow origin stays distinct from the production log source and is searchable', () => {
  const source = request('image-request', 1, { workflow_origins: [{ uuid: 'workflow-uuid', kind: 'premise_asset_generation', title: 'premise_asset_generation' }] })
  const t = (key, values = {}) => Object.entries(values).reduce((text, [name, value]) => text.replaceAll(`{${name}}`, value), RESOURCES['zh-Hans'][key])
  assert.equal(trajectoryRequestOrigin(source, t), 'Workflow：设定项图片生成 → 生产任务')
  assert.equal(trajectoryRequestOrigin({ ...source, source_type: 'workflow' }, t), 'Workflow：设定项图片生成')
  assert.equal(trajectoryRequestOrigin({ source_type: 'project_chat' }, t), '项目对话')
  assert.equal(trajectoryWorkflowTitle({ kind: 'comic_section_image_generation' }, t, { format: 'classic_picture_book' }), '页面图片生成')
  assert.equal(trajectoryWorkflowTitle({ kind: 'comic_section_image_generation' }, t, { format: 'vertical_strip' }), '画面段落图片生成')
  const projection = replaceTrajectoryProjection(page([source]))
  const index = reconcileTrajectorySearchIndex(new Map(), projection.rows)
  assert.deepEqual(filterTrajectoryRows(projection.rows, index, { query: 'workflow-uuid' }).map((row) => row.sourceUuid), ['image-request'])
  assert.equal(projection.requests[0].source.source_type, 'production')
})

test('workflow image requests render without manufactured assistant, system, or tool items', () => {
  const projection = replaceTrajectoryProjection(page([request('image-request', 1)]))
  const ledger = applyTrajectoryCollapse(projection.rows)
  assert.equal(projection.items.length, 0)
  assert.equal(ledger.length, 1)
  assert.equal(ledger[0].rowType, 'request')
  assert.equal(ledger[0].kind, 'model_request')
  assert.equal(ledger[0].sourceKind, 'model_request')
  assert.equal(ledger[0].key, 'model_request:image-request')
  assert.equal(ledger[0].durationMs, 47061)
  assert.equal(ledger[0].turnUuid, null)
  assert.deepEqual(ledger[0].requestBoundaries.map((request) => request.key), ['model_request:image-request'])
  for (const mode of ['sequence', 'duration', 'time', 'actual']) {
    const timeline = buildTrajectoryTimeline(trajectoryTimelineEntries(projection), mode)
    assert.equal(timeline.items.length, 1)
    assert.equal(timeline.items[0].sourceUuid, ledger[0].sourceUuid)
    assert.equal(timeline.items[0].durationMs, 47061)
  }
})

test('a persisted workflow gets its own row while requests remain independent dots and tools stay genuine', () => {
  const workflow = { uuid: 'workflow', thread_uuid: 'workflow-thread', kind: 'premise_asset_generation', title: 'Workflow title', status: 'completed', created_at: '2026-09-07T01:46:00Z', started_at: '2026-09-07T01:46:01Z', completed_at: '2026-09-07T01:49:00Z', duration_ms: 179000 }
  const projection = replaceTrajectoryProjection(page([
    request('failed', 1, { status: 'failed', attempt: 1, workflow_origins: [workflow] }),
    request('success', 2, { attempt: 2, workflow_origins: [workflow] }),
  ], {
    workflows: [workflow],
    tools: [{ uuid: 'execution', tool_call_uuid: 'call', tool_name: 'request_api', request_uuid: 'success', status: 'completed', created_at: '2026-09-07T01:48:10Z' }],
  }))
  const ledger = applyTrajectoryCollapse(projection.rows)
  assert.deepEqual(ledger.map((row) => row.kind), ['workflow', 'model_request', 'tool'])
  assert.equal(ledger[0].sourceKind, 'workflow')
  assert.equal(ledger[0].sourceUuid, workflow.uuid)
  assert.equal(ledger[0].status, 'completed')
  assert.equal(ledger[0].durationMs, 179000)
  assert.deepEqual(ledger[0].requestBoundaries, [])
  assert.equal(ledger[1].rowType, 'request')
  assert.equal(ledger[1].status, 'error')
  assert.deepEqual(ledger[1].requestBoundaries.map((request) => request.sourceUuid), ['failed', 'success'])
  assert.deepEqual(ledger[2].requestBoundaries, [])
  assert.equal(ledger[2].requestUuid, 'success')
  const index = reconcileTrajectorySearchIndex(new Map(), projection.rows)
  assert.deepEqual(filterTrajectoryRows(projection.rows, index, { query: 'Workflow title', kind: 'workflow' }).map((row) => row.key), ['workflow:workflow'])
  const localizedIndex = reconcileTrajectorySearchIndex(index, projection.rows, { workflowTitle: () => '设定项图片生成' })
  assert.deepEqual(filterTrajectoryRows(projection.rows, localizedIndex, { query: '设定项图片生成', kind: 'workflow' }).map((row) => row.key), ['workflow:workflow'])
  const requestFilter = filterTrajectoryRows(projection.rows, index, { kind: 'request' })
  assert.deepEqual(applyTrajectoryCollapse(requestFilter, projection.rows).flatMap((row) => row.requestBoundaries.map((request) => request.sourceUuid)), ['failed', 'success'])
  const timeline = buildTrajectoryTimeline(trajectoryTimelineEntries(projection), 'duration')
  assert.equal(timeline.items.length, 2)
  assert.equal(timeline.recordedDurationMs, 94122)
})

test('workflow rows exist before model calls and remain unique across history and state updates', () => {
  const workflow = { uuid: 'workflow', kind: 'premise_asset_generation', status: 'queued', created_at: '2026-09-07T01:46:00Z' }
  let projection = replaceTrajectoryProjection(page([], { workflows: [workflow], history_complete: false }))
  const selectedKey = projection.items[0].key
  assert.equal(applyTrajectoryCollapse(projection.rows)[0].kind, 'workflow')
  assert.equal(projection.items[0].status, 'pending')
  projection = applyTrajectoryUpserts(projection, page([request('request', 1)], { workflows: [{ ...workflow, status: 'completed', duration_ms: 60000 }], history_complete: false }))
  projection = prependTrajectoryPage(projection, page([], { workflows: [workflow] }))
  assert.equal(projection.items.length, 1)
  assert.equal(projection.items[0].key, selectedKey)
  assert.equal(projection.items[0].status, 'completed')
  assert.equal(projection.requests.length, 1)
  projection = applyTrajectoryUpserts(projection, page([], { workflows: [{ ...workflow, status: 'cancelled' }] }))
  assert.equal(projection.items[0].status, 'interrupted')
  assert.equal(projection.items[0].key, selectedKey)
})

test('pending, failed and response-free requests survive history merging, search and lifecycle updates', () => {
  const pending = request('third', 3, { status: 'pending', attempt: 3, has_response: false, duration_ms: undefined, completed_at: undefined })
  let projection = replaceTrajectoryProjection(page([pending], { history_complete: false }))
  projection = prependTrajectoryPage(projection, page([
    request('first', 1, { status: 'failed', has_response: false, error_code: 'image_timeout' }),
    request('second', 2, { status: 'failed', has_response: false, attempt: 2, error_code: 'image_network_error' }),
  ]))
  assert.deepEqual(applyTrajectoryCollapse(projection.rows).flatMap((row) => row.requestBoundaries.map((request) => request.status)), ['error', 'error', 'pending'])
  const pendingKey = projection.requests.find((row) => row.sourceUuid === 'third').key
  projection = applyTrajectoryUpserts(projection, page([request('third', 3, { attempt: 3 })]))
  assert.equal(projection.requests.length, 3)
  assert.equal(projection.requests.find((row) => row.sourceUuid === 'third').key, pendingKey)
  assert.equal(projection.requests.find((row) => row.sourceUuid === 'third').status, 'completed')
  let index = reconcileTrajectorySearchIndex(new Map(), projection.rows)
  const filtered = filterTrajectoryRows(projection.rows, index, { query: 'production image_timeout', kind: 'request' })
  assert.deepEqual(applyTrajectoryCollapse(filtered, projection.rows).map((row) => row.sourceUuid), ['first'])
  index = updateTrajectoryRequestSearchDocument(index, 'third', { response: { content: 'detail-only-value' } })
  const detailMatches = filterTrajectoryRows(projection.rows, index, { query: 'detail-only-value', rangeKeys: new Set(['third']) })
  assert.deepEqual(applyTrajectoryCollapse(detailMatches, projection.rows).map((row) => row.key), [pendingKey])
})

test('chat request fallback never attaches a failed request to an unrelated assistant', () => {
  const turn = { rowType: 'turn', key: 'turn:one', turnUuid: 'one', turn: { queue_sequence: 1, status: 'completed' } }
  const first = { rowType: 'request', key: 'model_request:first', sourceUuid: 'first', turnUuid: 'one', requestUuid: 'first', status: 'error' }
  const second = { ...first, key: 'model_request:second', sourceUuid: 'second', requestUuid: 'second', status: 'completed' }
  const assistant = { rowType: 'item', key: 'assistant:second', sourceUuid: 'assistant', turnUuid: 'one', requestUuid: 'second', kind: 'assistant', status: 'completed' }
  const rows = [turn, first, second, assistant]
  const ledger = applyTrajectoryCollapse(rows)
  assert.deepEqual(ledger.map((row) => row.key), [first.key, assistant.key])
  assert.deepEqual(ledger[0].requestBoundaries.map((row) => row.key), [first.key, second.key])
  assert.deepEqual(ledger[1].requestBoundaries, [])
  assert.equal(ledger[1].requestUuid, second.sourceUuid)
  const filtered = filterTrajectoryRows(rows, new Map(), { kind: 'request' })
  assert.deepEqual(applyTrajectoryCollapse(filtered, rows).flatMap((row) => row.requestBoundaries.map((request) => request.key)), [first.key, second.key])
  const collapsed = applyTrajectoryCollapse(rows, rows, { collapsedTurns: new Set(['one']) })
  assert.equal(collapsed.length, 1)
  assert.equal(collapsed[0].hiddenCount, 2)
})

test('workflow requests use boundary dots on genuine tool execution rows and preserve API errors', () => {
  const projection = replaceTrajectoryProjection(page([request('image-request', 1, { has_tool_calls: true })], {
    tools: [{
      uuid: 'execution', tool_call_uuid: 'call', tool_name: 'request_api', request_uuid: 'image-request', request_ordinal: 1,
      status: 'completed', arguments: { method: 'GET' }, result: { success: false, error: { code: 'missing' } },
      created_at: '2026-09-07T01:48:10Z',
    }],
  }))
  const ledger = applyTrajectoryCollapse(projection.rows)
  assert.deepEqual(ledger.map((row) => row.rowType), ['item'])
  assert.equal(ledger[0].kind, 'tool')
  assert.equal(ledger[0].status, 'error')
  assert.equal(ledger[0].requestUuid, 'image-request')
  assert.deepEqual(ledger[0].output, { success: false, error: { code: 'missing' } })
  assert.deepEqual(ledger[0].requestBoundaries.map((request) => request.sourceUuid), ['image-request'])
  const requestsOnly = filterTrajectoryRows(projection.rows, new Map(), { kind: 'request' })
  const filteredLedger = applyTrajectoryCollapse(requestsOnly, projection.rows)
  assert.equal(filteredLedger.length, 1)
  assert.equal(filteredLedger[0].sourceUuid, 'image-request')
})

test('a request keeps its selection identity when a real tool becomes its dot anchor', () => {
  let projection = replaceTrajectoryProjection(page([request('pending-request', 1, { status: 'pending', has_response: false, duration_ms: undefined })]))
  const selectedKey = projection.requests[0].key
  assert.equal(applyTrajectoryCollapse(projection.rows)[0].key, selectedKey)
  projection = applyTrajectoryUpserts(projection, page([request('pending-request', 1)], {
    tools: [{ uuid: 'execution', tool_call_uuid: 'call', tool_name: 'request_api', request_uuid: 'pending-request', status: 'running', created_at: '2026-09-07T01:48:10Z' }],
  }))
  const ledger = applyTrajectoryCollapse(projection.rows)
  assert.equal(ledger.length, 1)
  assert.equal(ledger[0].kind, 'tool')
  assert.equal(ledger[0].requestBoundaries[0].key, selectedKey)
  assert.equal(ledger[0].requestBoundaries[0].status, 'completed')
  assert.equal(projection.requests[0].key, selectedKey)
})
