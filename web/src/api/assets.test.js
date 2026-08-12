import assert from 'node:assert/strict'
import test from 'node:test'

import { createAssetUpload, createIntegrityScan, finalizeAssetUpload, listAssets, rebuildAssetThumbnail, reconcileAssets } from './assets.js'

test('asset upload sends purpose before bytes and finalizes by public UUID', async () => {
  const originalFetch = global.fetch
  const calls = []
  global.fetch = async (path, options = {}) => {
    calls.push({ path, options })
    return { ok: true, status: 200, json: async () => ({ success: true, data: { uuid: '01900000-0000-7000-8000-000000000001' } }) }
  }
  try {
    const file = new Blob(['image'], { type: 'image/png' })
    await createAssetUpload('01900000-0000-7000-8000-000000000000', { purpose: 'premise_asset', displayName: 'Hero', file })
    const formEntries = [...calls[0].options.body.entries()]
    assert.deepEqual(formEntries.slice(0, 2), [['purpose', 'premise_asset'], ['display_name', 'Hero']])
    await finalizeAssetUpload('01900000-0000-7000-8000-000000000000', '01900000-0000-7000-8000-000000000001', 'premise_asset')
    assert.equal(calls[1].path, '/api/v1/projects/01900000-0000-7000-8000-000000000000/asset-uploads/01900000-0000-7000-8000-000000000001/completions')
    assert.deepEqual(JSON.parse(calls[1].options.body), { purpose: 'premise_asset' })
  } finally { global.fetch = originalFetch }
})

test('asset maintenance API uses resource routes', async () => {
  const originalFetch = global.fetch
  const paths = []
  global.fetch = async (path) => { paths.push(path); return { ok: true, status: 200, json: async () => ({ success: true, data: { items: [] } }) } }
  try {
    await listAssets('project', { deleted: true, limit: 20 })
    await createIntegrityScan('project')
    await reconcileAssets('project')
    await rebuildAssetThumbnail('project', 'asset', 'detail_1024')
    assert.deepEqual(paths, [
      '/api/v1/projects/project/assets?deleted=true&limit=20',
      '/api/v1/projects/project/integrity-scans',
      '/api/v1/projects/project/asset-reconciliations',
      '/api/v1/projects/project/assets/asset/thumbnails?profile=detail_1024',
    ])
  } finally { global.fetch = originalFetch }
})
