import test from 'node:test'
import assert from 'node:assert/strict'
import { detailEdge, PreviewCache } from './images.js'
import { imageData } from './contracts.js'
const uuid = n => `01989abc-def0-7000-8000-${String(n).padStart(12, '0')}`
const project = uuid(1)
const wrap = data => ({structuredContent:{success:true,data}})

test('an opened dialog sizes portrait artwork in device pixels and caches its UI-only image separately', async () => {
  const asset = { width: 1200, height: 1800 }
  assert.equal(detailEdge(450, 1, asset), 1024)
  assert.equal(detailEdge(450, 2, asset), 1536)
  assert.equal(detailEdge(1000, 2, asset), 2048)
  const calls = []
  const data = { asset_uuid: uuid(100), mime_type: 'image/jpeg', width: 640, height: 960, data_uri: 'data:image/jpeg;base64,' + 'A'.repeat(240000) }
  const image = { _meta: { 'lumi/app_response': { success: true, data } } }
  const call = async (name, args) => {
    calls.push([name, args.max_edge])
    return name === 'read_app_image_detail' ? image : wrap({ ...data, data_uri: 'data:image/jpeg;base64,/9j/2Q==' })
  }
  const previews = new PreviewCache(call)
  const details = new PreviewCache(call, { detail: true })
  await previews.load(project, data.asset_uuid, 1024)
  await details.load(project, data.asset_uuid, 1024)
  await details.load(project, data.asset_uuid, 1024)
  assert.deepEqual(calls, [['read_app_image', 1024], ['read_app_image_detail', 1024]])
  assert.throws(() => imageData(image, data.asset_uuid, 1024))
  assert.throws(() => imageData(wrap(data), data.asset_uuid, 1024, true))
})
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
