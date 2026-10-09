import assert from 'node:assert/strict'
import test from 'node:test'
import { createModelTest, executeModelTest, getModelTest, listModelTests, stopModelTest } from './modelTests.js'

test('model tests use global resource endpoints and idempotent execution with explicit model selections', async () => {
  const calls = []
  const original = global.fetch
  const models = { text_model: { provider_uuid: 'text-provider', model: 'text' }, image_model: { provider_uuid: 'image-provider', model: 'image' } }
  global.fetch = async (url, options = {}) => {
    calls.push([url, options])
    return new Response(JSON.stringify({ success: true, data: { uuid: 'test-uuid' } }), { headers: { 'Content-Type': 'application/json' } })
  }
  try {
    assert.deepEqual(await createModelTest(models), { uuid: 'test-uuid' })
    await executeModelTest('test uuid', models)
    await getModelTest('test uuid')
    await listModelTests(2)
    await stopModelTest('test uuid')
  } finally { global.fetch = original }
  assert.equal(calls[0][0], '/api/v1/model-tests')
  assert.equal(calls[0][1].method, 'POST')
  assert.deepEqual(JSON.parse(calls[0][1].body), models)
  assert.equal(calls[1][0], '/api/v1/model-tests/test%20uuid/execution')
  assert.equal(calls[1][1].method, 'PUT')
  assert.deepEqual(JSON.parse(calls[1][1].body), models)
  assert.equal(calls[2][0], '/api/v1/model-tests/test%20uuid')
  assert.equal(calls[3][0], '/api/v1/model-tests?page=2&per_page=20')
  assert.equal(calls[4][0], '/api/v1/model-tests/test%20uuid/execution')
  assert.equal(calls[4][1].method, 'DELETE')
})
