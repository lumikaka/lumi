import assert from 'node:assert/strict'
import test from 'node:test'

import {
  activeTaskFor,
  collectPremiseTags,
  moveSection,
  normalizedTags,
  premiseAssetTitleFromFile,
  premiseSourceState,
} from './productionWorkspaceState.js'

test('production state keeps normalized unique tags and contiguous reorder intent', () => {
  assert.deepEqual(normalizedTags(' Hero,hero, LEAD ,scene'), ['hero', 'lead', 'scene'])
  assert.deepEqual(moveSection(['a', 'b', 'c'], 2, -1), ['a', 'c', 'b'])
  assert.deepEqual(moveSection(['a', 'b'], 0, -1), ['a', 'b'])
})

test('production task recovery only returns an active matching resource and kind', () => {
  const tasks = [
    { uuid: 'failed', kind: 'comic_image_generation', resource_uuid: 'section', status: 'failed' },
    { uuid: 'active', kind: 'comic_image_generation', resource_uuid: 'section', status: 'running' },
    { uuid: 'other', kind: 'premise_asset_breakdown', resource_uuid: 'section', status: 'queued' },
  ]
  assert.equal(activeTaskFor(tasks, 'comic_image_generation', 'section').uuid, 'active')
  assert.equal(activeTaskFor(tasks, 'comic_export', 'section'), null)
})

test('premise upload drafts derive readable titles and shared tag filters', () => {
  assert.equal(premiseAssetTitleFromFile({ name: 'main-hero_portrait.PNG' }), 'main hero portrait')
  assert.equal(premiseAssetTitleFromFile('.hidden-scene.svg'), 'hidden scene')
  assert.equal(premiseAssetTitleFromFile('no-extension'), 'no extension')
  assert.deepEqual(collectPremiseTags([
    { tags: ['角色', 'lead'] },
    { tags: ['Lead', ' scene ', ''] },
  ], 'zh-CN'), ['角色', 'lead', 'scene'])
})

test('premise source status reflects generation and breakdown stages', () => {
  const settings = [{ uuid: 'setting-a', source_uuid: 'source-a' }]
  assert.equal(premiseSourceState('source-empty', settings, []), 'draft')
  assert.equal(premiseSourceState('source-a', settings, []), 'ready')
  assert.equal(premiseSourceState('source-a', settings, [{ kind: 'premise_asset_breakdown', resource_uuid: 'setting-a', status: 'running' }]), 'splitting')
  assert.equal(premiseSourceState('source-a', settings, [{ kind: 'premise_asset_breakdown', resource_uuid: 'setting-a', status: 'completed' }]), 'completed')
  assert.equal(premiseSourceState('source-empty', settings, [{ kind: 'premise_setting_generation', resource_uuid: 'source-empty', status: 'failed' }]), 'failed')
  assert.equal(premiseSourceState({ uuid: 'source-a', ignored_at: '2026-08-09T00:00:00Z' }, settings, []), 'ignored')
})
