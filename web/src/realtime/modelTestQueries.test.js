import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { QueryClient } from '@tanstack/react-query'
import { modelTestKeys } from '../api/modelTests.js'
import { invalidateModelTestQueries } from './modelTestQueries.js'

test('a test change invalidates history and its detail without copying websocket state', () => {
  const client = new QueryClient()
  const keys = [modelTestKeys.list(1), modelTestKeys.list(2), modelTestKeys.detail('a'), modelTestKeys.detail('b'), ['model-settings']]
  keys.forEach((key) => client.setQueryData(key, { status: 'queued' }))
  invalidateModelTestQueries(client, { test_uuid: 'a' })
  keys.forEach((key, i) => assert.equal(client.getQueryState(key).isInvalidated, i < 3))
  assert.deepEqual(client.getQueryData(modelTestKeys.detail('a')), { status: 'queued' })
  invalidateModelTestQueries(client)
  assert.equal(client.getQueryState(modelTestKeys.detail('b')).isInvalidated, true)
  assert.equal(client.getQueryState(['model-settings']).isInvalidated, false)
  client.clear()
})

test('test history and details calibrate on initial join, rejoin, focus and visibility', () => {
  const source = readFileSync(new URL('./useSiteSettingsRealtime.js', import.meta.url), 'utf8')
  assert.match(source, /channel\.on\('model_test:changed', \(payload\) => invalidateModelTestQueries\(queryClient, payload\)\)/)
  assert.match(source, /const invalidateAll = \(\) => {[\s\S]*?invalidateModelTestQueries\(queryClient\)/)
  assert.match(source, /channel.on\('phx_joined', invalidateAll\)/)
  assert.match(source, /const handleFocus = \(\) => invalidateAll\(\)/)
  assert.match(source, /document.visibilityState === 'visible'\) invalidateAll\(\)/)
  const pages = ['ToolsPage.jsx', 'ModelTestPage.jsx'].map((file) => readFileSync(new URL(`../pages/${file}`, import.meta.url), 'utf8')).join('\n')
  assert.doesNotMatch(source + pages, /setInterval|refetchInterval/)
})
