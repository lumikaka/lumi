import assert from 'node:assert/strict'
import test from 'node:test'

import { applyTrajectoryCollapse, buildAssistantToolGroups } from './trajectoryCollapse.js'
import {
  filterTrajectoryRows,
  matchingTrajectoryKeys,
  reconcileTrajectorySearchIndex,
  tokenizeTrajectoryQuery,
  updateTrajectoryRequestSearchDocument,
} from './trajectorySearch.js'

const turn = { rowType: 'turn', key: 'turn:t1', sourceUuid: 't1', turnUuid: 't1', ariaRowIndex: 1, turn: { queue_sequence: 1, status: 'completed' } }
const assistant = { rowType: 'item', key: 'chat:a1', sourceUuid: 'a1', turnUuid: 't1', kind: 'assistant', status: 'completed', preview: 'I will inspect', requestUuid: 'r1', ariaRowIndex: 2, source: {} }
const tool = { rowType: 'item', key: 'tool:c1', sourceUuid: 'c1', turnUuid: 't1', kind: 'tool', status: 'error', preview: 'read_file failed', requestUuid: 'r1', callUuid: 'c1', input: { path: 'STORY.md' }, output: { success: false, error: 'missing' }, ariaRowIndex: 3, source: { tool_name: 'read_file' } }
const rows = [turn, assistant, tool]

test('trajectory search tokenizes case-insensitively with whitespace AND semantics', () => {
  const index = reconcileTrajectorySearchIndex(new Map(), rows)
  assert.deepEqual(tokenizeTrajectoryQuery('  READ_file   Missing read_FILE '), ['read_file', 'missing'])
  assert.deepEqual([...matchingTrajectoryKeys(index, 'READ_file missing')], ['tool:c1'])
  assert.equal(matchingTrajectoryKeys(index, 'read_file complete').size, 0)
  assert.deepEqual(filterTrajectoryRows(rows, index, { query: 'story.md missing' }).map((row) => row.key), ['turn:t1', 'tool:c1'])
})

test('LLM detail incrementally updates only documents linked to its Request', () => {
  const index = reconcileTrajectorySearchIndex(new Map(), rows)
  const next = updateTrajectoryRequestSearchDocument(index, 'r1', { request_payload: { tools: [{ function: { name: 'inspect_schema' } }] }, response: { content: 'detail-only phrase' } })
  assert.notEqual(next, index)
  assert.equal(next.get('turn:t1'), index.get('turn:t1'))
  assert.notEqual(next.get('chat:a1'), index.get('chat:a1'))
  assert.deepEqual([...matchingTrajectoryKeys(next, 'inspect_schema detail-only')], ['chat:a1', 'tool:c1'])
  assert.equal(updateTrajectoryRequestSearchDocument(next, 'r1', { request_payload: { tools: [{ function: { name: 'inspect_schema' } }] }, response: { content: 'detail-only phrase' } }), next)
})

test('Turn and Assistant-following Tool collapse remain independent with stable summary keys', () => {
  const groups = buildAssistantToolGroups(rows)
  const group = groups.get('chat:a1')
  assert.equal(group.key, 'summary:assistant-tools:a1:empty')
  const toolCollapsed = applyTrajectoryCollapse(rows, rows, { collapsedToolGroups: new Set(['chat:a1']), toolGroups: groups })
  assert.deepEqual(toolCollapsed.map((row) => row.key), ['chat:a1', group.key])
  assert.equal(toolCollapsed[0].turnStart, true)
  assert.equal(toolCollapsed[1].turnEnd, true)
  const turnCollapsed = applyTrajectoryCollapse(rows, rows, { collapsedTurns: new Set(['t1']), toolGroups: groups })
  assert.deepEqual(turnCollapsed.map((row) => row.key), ['summary:turn:t1:empty'])
  assert.equal(turnCollapsed[0].turn.queue_sequence, 1)
  assert.equal(turnCollapsed[0].turnStart, true)
  assert.equal(turnCollapsed[0].turnEnd, true)
})

test('filter order applies range then search before collapse without mutating projection rows', () => {
  const index = reconcileTrajectorySearchIndex(new Map(), rows)
  const rangeFiltered = filterTrajectoryRows(rows, index, { query: 'missing', rangeKeys: new Set(['chat:a1']) })
  assert.deepEqual(rangeFiltered, [])
  const searched = filterTrajectoryRows(rows, index, { query: 'inspect' })
  const collapsed = applyTrajectoryCollapse(searched, rows, { collapsedTurns: new Set(['t1']) })
  assert.deepEqual(collapsed.map((row) => row.key), ['summary:turn:t1:empty'])
  assert.equal(rows.length, 3)
})

test('Turn is projected as a rail and Request is anchored as a dot on the first matching event', () => {
  const user = { rowType: 'item', key: 'chat:u1', sourceUuid: 'u1', turnUuid: 't1', kind: 'user', status: 'completed', preview: 'Hello', ariaRowIndex: 2 }
  const request = { rowType: 'request', key: 'model_request:r1', sourceUuid: 'r1', sourceKind: 'model_request', turnUuid: 't1', requestUuid: 'r1', requestOrdinal: 1, status: 'completed', ariaRowIndex: 3 }
  const fullRows = [turn, user, request, assistant, tool]
  const ledger = applyTrajectoryCollapse(fullRows, fullRows)

  assert.deepEqual(ledger.map((row) => row.key), ['chat:u1', 'chat:a1', 'tool:c1'])
  assert.equal(ledger[0].turnStart, true)
  assert.equal(ledger[2].turnEnd, true)
  assert.deepEqual(ledger[1].requestBoundaries.map((boundary) => boundary.key), ['model_request:r1'])
  assert.equal(ledger[1].requestBoundaries[0].requestOrdinal, 1)
})
