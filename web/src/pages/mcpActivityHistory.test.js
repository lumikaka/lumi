import assert from 'node:assert/strict'
import test from 'node:test'
import { loadMCPActivityHistory } from './mcpActivityHistory.js'

const page = (uuids, next = '') => ({ items: uuids.map((uuid) => ({ uuid })), cursor_pagination: { next_cursor: next, has_more: Boolean(next) } })

test('history reload replaces the loaded window when a late success merges segments', async () => {
  const before = await loadMCPActivityHistory(async (after) => after ? page(['third']) : page(['first'], 'next'), 2)
  assert.deepEqual(before.items.map((item) => item.uuid), ['first', 'third'])
  const after = await loadMCPActivityHistory(async () => page(['first']), 2)
  assert.deepEqual(after.items.map((item) => item.uuid), ['first'])
})

test('a merge between page requests restarts from the beginning without mixing revisions', async () => {
  const calls = []
  let changed = false
  const result = await loadMCPActivityHistory(async (cursor) => {
    calls.push(cursor)
    if (cursor === 'old') {
      changed = true
      throw Object.assign(new Error('changed'), { code: 'mcp_activity_changed' })
    }
    return changed ? page(['merged', 'failure']) : page(['first'], 'old')
  }, 3)
  assert.deepEqual(calls, ['', 'old', ''])
  assert.deepEqual(result.items.map((item) => item.uuid), ['merged', 'failure'])
})

test('non-revision errors propagate and continuous changes have bounded retries', async () => {
  let calls = 0
  await assert.rejects(loadMCPActivityHistory(async () => {
    calls += 1
    throw Object.assign(new Error('changed'), { code: 'mcp_activity_changed' })
  }, 1), /changed/)
  assert.equal(calls, 3)
  await assert.rejects(loadMCPActivityHistory(async () => { throw new Error('offline') }, 1), /offline/)
})
