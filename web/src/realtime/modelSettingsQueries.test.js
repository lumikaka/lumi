import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { QueryClient } from '@tanstack/react-query'

import { invalidateModelSettingsQueries } from './modelSettingsQueries.js'

test('global changes invalidate all projects and preflights without disturbing task snapshots', () => {
  const client = new QueryClient()
  const affected = [['model-settings'], ['project-model-settings', 'a'], ['project-model-settings', 'b'], ['project-image-generation-preflight', 'a', 'image-1']]
  const frozen = ['production-tasks', 'a']
  for (const key of [...affected, frozen]) client.setQueryData(key, { revision: 1 })
  invalidateModelSettingsQueries(client)
  for (const key of affected) assert.equal(client.getQueryState(key).isInvalidated, true)
  assert.equal(client.getQueryState(frozen).isInvalidated, false)
  client.clear()
})

test('system subscriptions calibrate models on changes, first join, rejoin and focus', () => {
  const source = readFileSync(new URL('./useSiteSettingsRealtime.js', import.meta.url), 'utf8')
  assert.match(source, /channel.on\('model_settings:changed', invalidateModels\)/)
  assert.match(source, /const invalidateSiteSettings = \(\) => {\s*invalidateModels\(\)/)
  assert.match(source, /channel.on\('site_settings:updated', invalidateSiteSettings\)/)
  assert.match(source, /channel.on\('phx_joined', invalidateAll\)/)
  assert.match(source, /const invalidateAll = \(\) => {\s*invalidateSiteSettings\(\)/)
  assert.match(source, /const handleFocus = \(\) => invalidateAll\(\)/)
  assert.doesNotMatch(source, /setInterval|refetchInterval/)
})
