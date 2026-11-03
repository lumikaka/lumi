import test from 'node:test'
import assert from 'node:assert/strict'
import { PreviewCache } from './images.js'
const uuid = n => `01989abc-def0-7000-8000-${String(n).padStart(12, '0')}`
const project = uuid(1)
const wrap = data => ({structuredContent:{success:true,data}})
test('decode retry evicts only the affected image and maintains byte accounting', async () => {
  let calls = 0
  const cache = new PreviewCache(async (_name, args) => {
    calls++
    return wrap({ asset_uuid: args.asset_uuid, mime_type: 'image/jpeg', width: 2, height: 1, data_uri: 'data:image/jpeg;base64,/9j/2Q==' })
  })
  await cache.load(project, uuid(100), 384)
  await cache.load(project, uuid(101), 384)
  const bytes = cache.bytes
  cache.evict(project, uuid(100), 384)
  assert.equal(cache.bytes, bytes / 2)
  await cache.load(project, uuid(101), 384); assert.equal(calls, 2)
  await cache.load(project, uuid(100), 384); assert.equal(calls, 3)
  assert.equal(cache.bytes, bytes)
})
