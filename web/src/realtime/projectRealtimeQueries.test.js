import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

import { projectRealtimeInvalidation, uniqueQueryKeys } from './projectRealtimeQueries.js'

const projectUuid = 'project-uuid'

function keyNames(result) {
  return result.queryKeys.map((queryKey) => queryKey[0])
}

test('chat and workflow events refresh core recovery queries without reloading LLM logs', () => {
  const result = projectRealtimeInvalidation(projectUuid, 'workflow:step_changed', {
    thread_uuid: 'thread-uuid',
    workflow_uuid: 'workflow-uuid',
  })
  assert.equal(result.invalidateAll, false)
  for (const expected of ['chat-threads', 'chat-thread', 'chat-items', 'chat-events', 'chat-trajectory', 'workflows', 'workflow', 'workflow-runs', 'workflow-events']) {
    assert.ok(keyNames(result).includes(expected), expected)
  }
  assert.ok(!keyNames(result).includes('workflow-llm-logs'))
  assert.ok(result.queryKeys.some((key) => key[0] === 'chat-items' && key[2] === 'thread-uuid'))
  assert.ok(result.queryKeys.some((key) => key[0] === 'chat-trajectory' && key[2] === 'thread-uuid'))
  assert.ok(result.queryKeys.some((key) => key[0] === 'workflow-runs' && key[2] === 'workflow-uuid'))
})

test('Model Request and compaction hints invalidate trajectory facts through REST rereads', () => {
  for (const event of ['chat:model_request_changed', 'chat:compaction_changed']) {
    const result = projectRealtimeInvalidation(projectUuid, event, { thread_uuid: 'thread-uuid' })
    assert.equal(result.invalidateAll, false)
    assert.ok(result.queryKeys.some((key) => key[0] === 'chat-trajectory' && key[2] === 'thread-uuid'))
  }
})

test('task events distinguish story work from asset maintenance', () => {
  const story = projectRealtimeInvalidation(projectUuid, 'task:progress', { kind: 'story_chapter_generation' })
  assert.deepEqual(keyNames(story), ['story-tasks'])

  const asset = projectRealtimeInvalidation(projectUuid, 'task:progress', { kind: 'asset_integrity_scan' })
  assert.deepEqual(keyNames(asset), ['asset-maintenance-tasks', 'asset-scans'])

  const legacy = projectRealtimeInvalidation(projectUuid, 'task:completed', {})
  assert.deepEqual(keyNames(legacy), ['story-tasks', 'asset-maintenance-tasks'])
})

test('production, asset and LLM events invalidate exact and aggregate queries', () => {
  const production = projectRealtimeInvalidation(projectUuid, 'production_task:progress', { task_uuid: 'task-uuid' })
  assert.deepEqual(keyNames(production), ['production-tasks', 'production-task', 'comic-export-operation', 'comic-exports'])
  assert.ok(production.queryKeys.some((key) => key[0] === 'production-task' && key[2] === 'task-uuid'))

  const asset = projectRealtimeInvalidation(projectUuid, 'integrity_scan/completed', { scan_uuid: 'scan-uuid' })
  assert.deepEqual(keyNames(asset), ['asset-scans', 'asset-maintenance-tasks', 'assets'])

  const llm = projectRealtimeInvalidation(projectUuid, 'llm_log:changed', { log_uuid: 'log-uuid' })
  assert.deepEqual(keyNames(llm), ['project-llm-logs', 'project-llm-log', 'workflow-llm-logs'])
  assert.ok(llm.queryKeys.some((key) => key[0] === 'project-llm-log' && key[2] === 'log-uuid'))
  assert.ok(!keyNames(llm).includes('workflow-runs'))
  assert.ok(!keyNames(llm).includes('workflow-events'))
})

test('comic export cleanup refreshes REST facts without polling', () => {
  const result = projectRealtimeInvalidation(projectUuid, 'comic:exports_changed', { exports_deleted: 2 })
  assert.equal(result.invalidateAll, false)
  assert.ok(keyNames(result).includes('comic-exports'))
  assert.ok(keyNames(result).includes('production-tasks'))
  assert.ok(result.queryKeys.some((key) => key.length === 1 && key[0] === 'recent-projects'))
})

test('comic resource changes reread recent project covers through REST', () => {
  const result = projectRealtimeInvalidation(projectUuid, 'production:resource_changed', {
    resource_uuid: 'cover-section-uuid',
    kind: 'comic_image_generation',
  })
  assert.equal(result.invalidateAll, false)
  assert.ok(keyNames(result).includes('comic-sections'))
  assert.ok(result.queryKeys.some((key) => key.length === 1 && key[0] === 'recent-projects'))
})

test('project setup changes reread setup and project facts without trusting payload state', () => {
  const result = projectRealtimeInvalidation(projectUuid, 'project:setup_changed', {
    setup_uuid: 'setup-uuid',
    status: 'finalized',
    revision: 4,
  })
  assert.equal(result.invalidateAll, false)
  assert.deepEqual(result.queryKeys, [
    ['project-setup', projectUuid],
    ['story-project', projectUuid],
    ['recent-projects'],
    ['open-projects'],
  ])
})

test('unknown business events fall back to a project resync while protocol events do not', () => {
  assert.deepEqual(projectRealtimeInvalidation(projectUuid, 'future_domain:changed', {}), { queryKeys: [], invalidateAll: true })
  assert.deepEqual(projectRealtimeInvalidation(projectUuid, 'phx_close', {}), { queryKeys: [], invalidateAll: false })
  assert.deepEqual(uniqueQueryKeys([['tasks', projectUuid], ['tasks', projectUuid], ['other', projectUuid]]), [['tasks', projectUuid], ['other', projectUuid]])
})

test('only the HTTP health card retains a refetch interval', () => {
  const sourceRoot = fileURLToPath(new URL('../', import.meta.url))
  const offenders = []
  const visit = (directory) => {
    for (const entry of readdirSync(directory, { withFileTypes: true })) {
      const path = `${directory}/${entry.name}`
      if (entry.isDirectory()) visit(path)
      else if (/\.jsx?$/.test(entry.name) && !entry.name.includes('.test.') && readFileSync(path, 'utf8').includes('refetchInterval')) offenders.push(path.slice(sourceRoot.length).replace(/^\//, ''))
    }
  }
  visit(sourceRoot)
  assert.deepEqual(offenders, ['components/HealthCard.jsx'])
  assert.match(readFileSync(`${sourceRoot}/components/HealthCard.jsx`, 'utf8'), /refetchInterval:\s*30_000/)
})
