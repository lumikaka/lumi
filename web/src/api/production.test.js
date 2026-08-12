import assert from 'node:assert/strict'
import test from 'node:test'

import { createComicExport, createPremiseAsset, createStoryboard, emptyPremiseAssetTrash, getComicExportReadiness, getComicSnapshot, getProductionTask, listComicExports, listPremiseSources, listProductionTaskEvents, listSettingImages, permanentlyDeletePremiseAsset, reorderComicSections, selectImageVariant, updatePremiseSource } from './production.js'

test('production API uses UUID resources, single-resource mutations, and snake_case bodies', async () => {
  const originalFetch = global.fetch
  const calls = []
  global.fetch = async (path, options = {}) => {
    calls.push({ path, options })
    return { ok: true, status: 200, json: async () => ({ success: true, data: { uuid: 'result' } }) }
  }
  try {
    await createPremiseAsset('project', { upload_uuid: 'upload', asset_type: 'character', title: 'Hero', tags: ['lead'] })
    await updatePremiseSource('project', 'source uuid', { ignored: true, expected_revision: 2 })
    await reorderComicSections('project', 'chapter', ['section-b', 'section-a'])
    await createStoryboard('project', 'chapter', 'section-a', { content_md: 'frame', source_type: 'manual', expected_revision: 2 })
    await selectImageVariant('project', 'chapter', 'section-a', 'variant-a', 3)
    await getComicExportReadiness('project', { scope: 'chapter', chapterUuid: 'chapter' })
    await getComicSnapshot('project', 'chapter', 'snapshot / one')
    await createComicExport('project', { scope: 'chapter', chapter_uuid: 'chapter', allow_missing_images: true, idempotency_key: 'export-1' })
    await permanentlyDeletePremiseAsset('project', 'asset / one', 7)
    await emptyPremiseAssetTrash('project')
    await listProductionTaskEvents('project', 'task', { after: '2', limit: '20' })
    await listPremiseSources('project', { page: 2, perPage: 10 })
    await listSettingImages('project', { sourceUuids: ['source-a', 'source b'] })
    await listComicExports('project', { page: 3, perPage: 10, scope: 'chapter', chapterUuid: 'chapter-a' })
    await listComicExports('project', { taskUuid: 'task-a', snapshotHash: 'hash-a', status: 'ready' })
    await getProductionTask('project', 'task / one')
    assert.deepEqual(calls.map((call) => call.path), [
      '/api/v1/projects/project/premise-assets',
      '/api/v1/projects/project/premise-sources/source%20uuid',
      '/api/v1/projects/project/chapters/chapter/comic-section-order',
      '/api/v1/projects/project/chapters/chapter/comic-sections/section-a/storyboard-variants',
      '/api/v1/projects/project/chapters/chapter/comic-sections/section-a/image-variants/variant-a/selections',
      '/api/v1/projects/project/comic-exports/readiness?scope=chapter&chapter_uuid=chapter',
      '/api/v1/projects/project/chapters/chapter/comic-snapshots/snapshot%20%2F%20one',
      '/api/v1/projects/project/comic-exports',
      '/api/v1/projects/project/premise-assets/asset%20%2F%20one/permanent?expected_revision=7',
      '/api/v1/projects/project/premise-assets/trash',
      '/api/v1/projects/project/production-tasks/task/events?after=2&limit=20',
      '/api/v1/projects/project/premise-sources?page=2&per_page=10',
      '/api/v1/projects/project/premise-setting-images?source_uuid=source-a&source_uuid=source+b',
      '/api/v1/projects/project/comic-exports?page=3&per_page=10&scope=chapter&chapter_uuid=chapter-a',
      '/api/v1/projects/project/comic-exports?page=1&per_page=20&task_uuid=task-a&snapshot_hash=hash-a&status=ready',
      '/api/v1/projects/project/production-tasks/task%20%2F%20one',
    ])
    assert.deepEqual(JSON.parse(calls[1].options.body), { ignored: true, expected_revision: 2 })
    assert.equal(calls[1].options.method, 'PATCH')
    assert.deepEqual(JSON.parse(calls[2].options.body), { section_uuids: ['section-b', 'section-a'] })
    assert.equal(JSON.parse(calls[4].options.body).expected_revision, 3)
    assert.equal(JSON.parse(calls[7].options.body).allow_missing_images, true)
  } finally { global.fetch = originalFetch }
})
