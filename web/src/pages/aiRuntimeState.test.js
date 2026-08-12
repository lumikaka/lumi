import assert from 'node:assert/strict'
import test from 'node:test'

import { latestTaskForResource, shouldPollTasks, taskControls } from './aiRuntimeState.js'

test('task recovery selects the newest persisted task for the chapter', () => {
  const items = [
    { uuid: 'new', resource_uuid: 'chapter-a', status: 'running' },
    { uuid: 'other', resource_uuid: 'chapter-b', status: 'completed' },
    { uuid: 'old', resource_uuid: 'chapter-a', status: 'failed' },
  ]
  assert.equal(latestTaskForResource(items, 'chapter-a').uuid, 'new')
  assert.equal(shouldPollTasks(items), true)
})

test('task controls expose cancel and explicit retry only in stable states', () => {
  assert.deepEqual(taskControls({ status: 'running', retryable: true }), { canCancel: true, canRetry: false })
  assert.deepEqual(taskControls({ status: 'failed', retryable: true }), { canCancel: false, canRetry: true })
  assert.deepEqual(taskControls({ status: 'cancelled', retryable: true }), { canCancel: false, canRetry: false })
})
