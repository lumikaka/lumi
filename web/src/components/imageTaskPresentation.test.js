import assert from 'node:assert/strict'
import test from 'node:test'
import { imageBatchCounts, imageStageElapsed, imageTaskStage } from './imageTaskPresentation.js'
import { workflowProgressPercent } from '../pages/chatAreaPresentation.js'

test('image progress prefers terminal and cancellation facts over a stale stage', () => {
  assert.equal(imageTaskStage({ status: 'running', stage: 'generating' }), 'generating')
  assert.equal(imageTaskStage({ status: 'running', stage: 'generating', cancel_requested_at: '2026-09-09' }), 'cancelling')
  assert.equal(imageTaskStage({ status: 'cancelled', stage: 'saving', cancel_requested_at: '2026-09-09' }), 'cancelled')
  assert.equal(imageTaskStage({ status: 'completed', stage: 'saving' }), 'completed')
  assert.equal(imageTaskStage({ status: 'queued', stage: 'generating' }), 'queued')
  assert.equal(imageTaskStage({ status: 'running' }), 'running')
})

test('elapsed time survives remounts and resets for each persisted stage', () => {
  const now = Date.parse('2026-09-09T04:05:00Z')
  const task = { status: 'running', started_at: '2026-09-09T04:00:00Z', stage_started_at: '2026-09-09T04:02:00Z' }
  assert.equal(imageStageElapsed(task, now), 180)
  assert.equal(imageStageElapsed({ ...task, stage_started_at: '2026-09-09T04:04:30Z' }, now), 30)
  assert.equal(imageStageElapsed({ ...task, status: 'completed' }, now), null)
  assert.equal(imageStageElapsed({ status: 'running' }, now), null)
  assert.equal(imageStageElapsed(task, now - 3600000), 0)
})

test('batch completion counts images, with failures and cancellations reported separately', () => {
  const workflow = { kind: 'comic_image_generation_batch', steps: [
    { status: 'completed', progress: 100 }, { status: 'running', progress: 80 },
    { status: 'queued', progress: 0 }, { status: 'failed', progress: 0 },
    { status: 'cancelled', progress: 80 }, { status: 'interrupted', progress: 5 },
  ] }
  assert.deepEqual(imageBatchCounts(workflow), { total: 6, completed: 1, running: 1, queued: 1, failed: 2, cancelled: 1 })
  assert.equal(workflowProgressPercent(workflow), 17)
  assert.equal(workflowProgressPercent({ ...workflow, steps: [] }), 0)
})
