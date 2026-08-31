import assert from 'node:assert/strict'
import test from 'node:test'

import { createComicExport, createComicSection, createPremiseAsset, createStoryboard, emptyPremiseAssetTrash, generateChapterImagesBatch, generatePremiseAssetVariant, getComicExportReadiness, getComicSnapshot, getPremiseAsset, getProductionTask, listComicExports, listComicSections, listPremiseSources, listProductionTaskEvents, listSettingImages, permanentlyDeletePremiseAsset, reorderComicSections, restoreComicSection, selectImageVariant, setComicSectionPremiseAssets, updateComicSection, updatePremiseSource } from './production.js'

test('production API uses UUID resources, single-resource mutations, and snake_case bodies', async () => {
  const originalFetch = global.fetch
  const calls = []
  global.fetch = async (path, options = {}) => {
    calls.push({ path, options })
    return { ok: true, status: 200, json: async () => ({ success: true, data: { uuid: 'result' } }) }
  }
  try {
    await createPremiseAsset('project', { upload_uuid: 'upload', asset_type: 'character', title: 'Hero', tags: ['lead'] })
	await getPremiseAsset('project', 'asset / linked')
    await updatePremiseSource('project', 'source uuid', { ignored: true, expected_revision: 2 })
    await reorderComicSections('project', 'chapter', ['section-b', 'section-a'])
    await createComicSection('project', 'chapter', { title: 'Cover', page_role: 'front_cover' })
    await updateComicSection('project', 'chapter', 'section-a', { title: 'Page one', page_role: 'body', expected_revision: 4 })
    await createStoryboard('project', 'chapter', 'section-a', { content_md: 'frame', source_type: 'manual', expected_revision: 2 })
    await selectImageVariant('project', 'chapter', 'section-a', 'variant-a', 3)
    await generateChapterImagesBatch('project', 'chapter', { section_uuids: ['section-a', 'section-b'], idempotency_key: 'batch-1' })
    await getComicExportReadiness('project', { scope: 'chapter', chapterUuid: 'chapter' })
    await getComicSnapshot('project', 'chapter', 'snapshot / one')
    await createComicExport('project', { scope: 'chapter', chapter_uuid: 'chapter', format: 'pdf', allow_missing_images: true, idempotency_key: 'export-1' })
    await permanentlyDeletePremiseAsset('project', 'asset / one', 7)
    await emptyPremiseAssetTrash('project')
    await listProductionTaskEvents('project', 'task', { after: '2', limit: '20' })
    await listPremiseSources('project', { page: 2, perPage: 10 })
    await listSettingImages('project', { sourceUuids: ['source-a', 'source b'] })
    await listComicExports('project', { page: 3, perPage: 10, scope: 'chapter', chapterUuid: 'chapter-a' })
    await listComicExports('project', { taskUuid: 'task-a', snapshotHash: 'hash-a', format: 'pdf', status: 'ready' })
    await getProductionTask('project', 'task / one')
    assert.deepEqual(calls.map((call) => call.path), [
      '/api/v1/projects/project/premise-assets',
	  '/api/v1/projects/project/premise-assets/asset%20%2F%20linked',
      '/api/v1/projects/project/premise-sources/source%20uuid',
      '/api/v1/projects/project/chapters/chapter/comic-section-order',
      '/api/v1/projects/project/chapters/chapter/comic-sections',
      '/api/v1/projects/project/chapters/chapter/comic-sections/section-a',
      '/api/v1/projects/project/chapters/chapter/comic-sections/section-a/storyboard-variants',
      '/api/v1/projects/project/chapters/chapter/comic-sections/section-a/image-variants/variant-a/selections',
      '/api/v1/projects/project/chapters/chapter/comic-image-generation-batches',
      '/api/v1/projects/project/comic-exports/readiness?scope=chapter&chapter_uuid=chapter',
      '/api/v1/projects/project/chapters/chapter/comic-snapshots/snapshot%20%2F%20one',
      '/api/v1/projects/project/comic-exports',
      '/api/v1/projects/project/premise-assets/asset%20%2F%20one/permanent?expected_revision=7',
      '/api/v1/projects/project/premise-assets/trash',
      '/api/v1/projects/project/production-tasks/task/events?after=2&limit=20',
      '/api/v1/projects/project/premise-sources?page=2&per_page=10',
      '/api/v1/projects/project/premise-setting-images?source_uuid=source-a&source_uuid=source+b',
      '/api/v1/projects/project/comic-exports?page=3&per_page=10&scope=chapter&chapter_uuid=chapter-a',
      '/api/v1/projects/project/comic-exports?page=1&per_page=20&task_uuid=task-a&snapshot_hash=hash-a&format=pdf&status=ready',
      '/api/v1/projects/project/production-tasks/task%20%2F%20one',
    ])
	assert.deepEqual(JSON.parse(calls[2].options.body), { ignored: true, expected_revision: 2 })
	assert.equal(calls[2].options.method, 'PATCH')
	assert.deepEqual(JSON.parse(calls[3].options.body), { section_uuids: ['section-b', 'section-a'] })
	assert.deepEqual(JSON.parse(calls[4].options.body), { title: 'Cover', page_role: 'front_cover' })
	assert.deepEqual(JSON.parse(calls[5].options.body), { title: 'Page one', page_role: 'body', expected_revision: 4 })
	assert.equal(calls[5].options.method, 'PATCH')
	assert.equal(JSON.parse(calls[7].options.body).expected_revision, 3)
	assert.deepEqual(JSON.parse(calls[8].options.body), { section_uuids: ['section-a', 'section-b'], idempotency_key: 'batch-1' })
	assert.equal(calls[8].options.method, 'POST')
	assert.equal(JSON.parse(calls[11].options.body).allow_missing_images, true)
	assert.equal(JSON.parse(calls[11].options.body).format, 'pdf')
  } finally { global.fetch = originalFetch }
})

test('simple premise generation and page reference selection use public resource actions', async () => {
  const originalFetch = global.fetch
  const calls = []
  global.fetch = async (path, options = {}) => {
    calls.push({ path, options })
    return { ok: true, status: 200, json: async () => ({ success: true, data: { uuid: 'result' } }) }
  }
  try {
    await generatePremiseAssetVariant('project uuid', 'asset / one', { prompt: 'new pose', idempotency_key: 'variant-1' })
    await setComicSectionPremiseAssets('project uuid', 'chapter / one', 'section / one', ['asset-a', 'asset-b'], 7)
    assert.equal(calls[0].path, '/api/v1/projects/project%20uuid/premise-assets/asset%20%2F%20one/generations')
    assert.equal(calls[0].options.method, 'POST')
    assert.deepEqual(JSON.parse(calls[0].options.body), { prompt: 'new pose', idempotency_key: 'variant-1' })
    assert.equal(calls[1].path, '/api/v1/projects/project%20uuid/chapters/chapter%20%2F%20one/comic-sections/section%20%2F%20one/premise-assets')
    assert.equal(calls[1].options.method, 'PUT')
    assert.deepEqual(JSON.parse(calls[1].options.body), { premise_asset_uuids: ['asset-a', 'asset-b'], expected_revision: 7 })
  } finally {
    global.fetch = originalFetch
  }
})

test('comic page trash uses a state-filtered collection and a restoration resource', async () => {
  const originalFetch = global.fetch
  const calls = []
  global.fetch = async (path, options = {}) => {
    calls.push({ path, options })
    return { ok: true, status: 200, json: async () => ({ success: true, data: { items: [] } }) }
  }
  try {
    await listComicSections('project uuid', 'chapter / one', { state: 'trashed' })
    await restoreComicSection('project uuid', 'chapter / one', 'section / one', 7)

    assert.equal(calls[0].path, '/api/v1/projects/project%20uuid/chapters/chapter%20%2F%20one/comic-sections?state=trashed')
    assert.equal(calls[0].options.method, undefined)
    assert.equal(calls[1].path, '/api/v1/projects/project%20uuid/chapters/chapter%20%2F%20one/comic-sections/section%20%2F%20one/restorations')
    assert.equal(calls[1].options.method, 'POST')
    assert.deepEqual(JSON.parse(calls[1].options.body), { expected_revision: 7 })
  } finally {
    global.fetch = originalFetch
  }
})
