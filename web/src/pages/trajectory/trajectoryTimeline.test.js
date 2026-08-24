import assert from 'node:assert/strict'
import test from 'node:test'

import {
  buildTrajectoryTimeline,
  panTrajectoryView,
  trajectoryTimelineEntries,
  trajectoryRangeSourceUuids,
  trajectoryTimelineLane,
  zoomTrajectoryView,
} from './trajectoryTimeline.js'

const entries = [
  { uuid: 'u1', source_kind: 'chat_item', kind: 'user', event_sequence: 1, item_sequence: 1, status: 'completed', started_at: '2026-08-21T00:00:00Z' },
  { uuid: 'r1', source_kind: 'model_request', kind: 'model_request', event_sequence: 2, status: 'completed', started_at: '2026-08-21T00:00:01Z', duration_ms: 800 },
  { uuid: 'c1', source_kind: 'tool', kind: 'tool', event_sequence: 3, status: 'running', started_at: '2026-08-21T01:00:00Z' },
]

test('timeline maps three semantic lanes and all four modes', () => {
  assert.equal(trajectoryTimelineLane('error'), 0)
  assert.equal(trajectoryTimelineLane('assistant'), 1)
  assert.equal(trajectoryTimelineLane('tool'), 2)
  for (const mode of ['sequence', 'duration', 'time', 'actual']) assert.equal(buildTrajectoryTimeline(entries, mode).mode, mode)
})

test('only recorded duration creates spans; pending and unknown durations remain markers', () => {
  assert.ok(buildTrajectoryTimeline(entries, 'sequence').items.every((item) => item.span))
  for (const mode of ['duration', 'time', 'actual']) {
    const model = buildTrajectoryTimeline(entries, mode)
    assert.equal(model.items.find((item) => item.sourceUuid === 'r1').span, true)
    const tool = model.items.find((item) => item.sourceUuid === 'c1')
    assert.equal(tool.span, false)
    assert.equal(tool.start, tool.end)
  }
  assert.ok(buildTrajectoryTimeline(entries, 'actual').domain.max > buildTrajectoryTimeline(entries, 'time').domain.max)
})

test('timeline adds real SYSTEM snapshots and avoids duplicate persisted Assistant output', () => {
  const projection = {
    thread: { created_at: '2026-08-21T00:00:00Z' },
    overview: { timeline: [
      ...entries,
      { uuid: 'a1', source_kind: 'chat_item', kind: 'assistant', started_at: '2026-08-21T00:00:02Z' },
    ] },
    items: [
      { sourceUuid: 'a1', sourceKind: 'chat_item', kind: 'assistant', requestUuid: 'r1' },
      { sourceUuid: 'r1', sourceKind: 'system_change', kind: 'system', systemChangeType: 'initial', status: 'completed', preview: 'Initial System Prompt', startedAt: Date.parse('2026-08-21T00:00:01Z') },
    ],
  }
  const result = trajectoryTimelineEntries(projection)
  assert.ok(result.some((entry) => entry.kind === 'system' && entry.source_kind === 'system_change'))
  assert.ok(!result.some((entry) => entry.uuid === 'a1'))
  const model = buildTrajectoryTimeline(result, 'sequence')
  assert.deepEqual(model.turnBoundaries.map((boundary) => boundary.turnUuid), [])
})

test('timeline supports zoom, pan and range focus without changing item identities', () => {
  const model = buildTrajectoryTimeline(entries, 'actual')
  const zoomed = zoomTrajectoryView(model.domain, model.domain, -1, 0.5)
  assert.ok(zoomed.end - zoomed.start < model.domain.max - model.domain.min)
  const panned = panTrajectoryView(zoomed, model.domain, 200)
  assert.equal(panned.end - panned.start, zoomed.end - zoomed.start)
  const focused = trajectoryRangeSourceUuids(model, { start: 0, end: 2000 })
  assert.deepEqual([...focused], ['u1', 'r1'])
  assert.deepEqual(model.items.map((item) => item.key), ['chat_item:u1', 'model_request:r1', 'tool:c1'])
})
