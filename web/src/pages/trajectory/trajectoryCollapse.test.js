import assert from 'node:assert/strict'
import test from 'node:test'

import { applyTrajectoryCollapse } from './trajectoryCollapse.js'

function request(uuid, turnUuid = 'turn') {
  return { rowType: 'request', key: `model_request:${uuid}`, sourceUuid: uuid, requestUuid: uuid, turnUuid, kind: 'model_request', status: 'completed' }
}

test('consecutive Request dots share a strip in execution order without changing the Tool request', () => {
  const first = request('first')
  const second = { ...request('second'), status: 'error' }
  const third = { ...request('third'), status: 'pending' }
  const turn = { rowType: 'turn', key: 'turn:turn', turnUuid: 'turn', turn: { queue_sequence: 1 } }
  const tool = { rowType: 'item', key: 'tool:call', sourceUuid: 'call', turnUuid: 'turn', kind: 'tool', requestUuid: first.sourceUuid }
  const ledger = applyTrajectoryCollapse([turn, first, second, third, tool])
  assert.equal(ledger.length, 2)
  assert.equal(ledger[0].key, first.key)
  assert.equal(ledger[0].turnStart, true)
  assert.deepEqual(ledger[0].requestBoundaries.map((dot) => [dot.key, dot.status]), [
    [first.key, 'completed'], [second.key, 'error'], [third.key, 'pending'],
  ])
  assert.equal(ledger[1].requestUuid, first.sourceUuid)
  assert.deepEqual(ledger[1].requestBoundaries, [])
})

test('Request-only filtering keeps separate strips across real content and Turn boundaries', () => {
  const first = request('first')
  const second = request('second')
  const third = request('third', 'next-turn')
  const turn = { rowType: 'turn', key: 'turn:turn', turnUuid: 'turn', turn: { queue_sequence: 1 } }
  const nextTurn = { rowType: 'turn', key: 'turn:next-turn', turnUuid: 'next-turn', turn: { queue_sequence: 2 } }
  const tool = { rowType: 'item', key: 'tool:call', sourceUuid: 'call', turnUuid: 'turn', kind: 'tool', requestUuid: first.sourceUuid }
  const fullRows = [turn, first, tool, second, nextTurn, third]
  const ledger = applyTrajectoryCollapse([first, second, third], fullRows)
  assert.deepEqual(ledger.map((row) => row.requestBoundaries.map((dot) => dot.key)), [[first.key], [second.key], [third.key]])
  assert.equal(ledger[0].turnStart, true)
  assert.equal(ledger[1].turnEnd, true)
  assert.equal(ledger[2].turnStart, true)
  assert.equal(ledger[2].turn.queue_sequence, 2)
  assert.equal(applyTrajectoryCollapse([first, third]).length, 2)
})
