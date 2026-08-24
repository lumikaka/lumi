import assert from 'node:assert/strict'
import test from 'node:test'

import {
  captureTrajectoryVirtualAnchor,
  isTrajectoryAtTail,
  measureTrajectoryRows,
  restoreTrajectoryVirtualAnchor,
  shouldFollowTrajectoryTail,
  shouldLoadEarlierTrajectory,
  trajectoryVirtualWindow,
} from './trajectoryVirtualRows.js'

function rows(count, prefix = 'row') {
  return Array.from({ length: count }, (_, index) => ({ key: `${prefix}:${index}`, rowType: 'item', ariaRowIndex: index + 1 }))
}

test('virtual window mounts only visible rows plus overscan and keeps stable logical indices', () => {
  const source = rows(1000)
  const heights = new Map([['row:500', 76]])
  const measured = measureTrajectoryRows(source, heights)
  const visible = trajectoryVirtualWindow(measured, 19000, 380, 76)
  assert.ok(visible.length < 30)
  assert.ok(visible.every((entry) => entry.row.key === entry.key))
  assert.ok(visible.every((entry) => entry.row.ariaRowIndex === entry.index + 1))
  assert.equal(measured.entries[500].size, 76)
})

test('prepend restores the same stable row key and viewport offset', () => {
  const current = measureTrajectoryRows(rows(20, 'new'))
  const anchor = captureTrajectoryVirtualAnchor(current, 300)
  const prepended = measureTrajectoryRows([...rows(10, 'old'), ...rows(20, 'new')])
  const nextScrollTop = restoreTrajectoryVirtualAnchor(prepended, anchor)
  const restored = captureTrajectoryVirtualAnchor(prepended, nextScrollTop)
  assert.equal(restored.key, anchor.key)
  assert.equal(restored.offset, anchor.offset)
})

test('history autoload and tail follow suspend when the user scrolls away', () => {
  assert.equal(shouldLoadEarlierTrajectory({ scrollTop: 120, hasPreviousPage: true }), true)
  assert.equal(shouldLoadEarlierTrajectory({ scrollTop: 120, hasPreviousPage: true, fetchingPreviousPage: true }), false)
  assert.equal(isTrajectoryAtTail({ scrollTop: 900, scrollHeight: 1300, clientHeight: 400 }), true)
  assert.equal(isTrajectoryAtTail({ scrollTop: 400, scrollHeight: 1300, clientHeight: 400 }), false)
  assert.equal(shouldFollowTrajectoryTail({ wasAtTail: true, previousLastKey: 'a', nextLastKey: 'b' }), true)
  assert.equal(shouldFollowTrajectoryTail({ wasAtTail: false, previousLastKey: 'a', nextLastKey: 'b' }), false)
  assert.equal(shouldFollowTrajectoryTail({ wasAtTail: true, previousLastKey: 'a', nextLastKey: 'b', prepending: true }), false)
})
