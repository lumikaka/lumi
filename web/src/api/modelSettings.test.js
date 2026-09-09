import assert from 'node:assert/strict'
import test from 'node:test'

import { getModelSettings, updateModelSettings, updateProjectModelSettings } from './ai.js'

test('global model settings read and patch the revisioned resource, including resets', async (t) => {
  const requests = []
  const data = { revision: 3, settings: {}, options: { text_models: [], image_models: [] } }
  t.mock.method(globalThis, 'fetch', async (path, options) => {
    requests.push({ path, options })
    return new Response(JSON.stringify({ success: true, data }), { status: 200 })
  })
  assert.deepEqual(await getModelSettings(), data)
  const selection = { provider_uuid: '019-provider', model: 'image-pro', prompt_extend: false, enable_thinking: false }
  assert.deepEqual(await updateModelSettings(2, { project_image: selection, story_text: null }), data)
  assert.equal(requests[0].path, '/api/v1/model-settings')
  assert.equal(requests[1].path, '/api/v1/model-settings')
  assert.equal(requests[1].options.method, 'PATCH')
  assert.deepEqual(JSON.parse(requests[1].options.body), { expected_revision: 2, overrides: { project_image: selection, story_text: null } })
  await updateProjectModelSettings('019-project', 4, { story_text: null })
  assert.equal(requests[2].path, '/api/v1/projects/019-project/model-settings')
})

test('a stale model settings save surfaces the conflict instead of retrying or overwriting', async (t) => {
  let requests = 0
  t.mock.method(globalThis, 'fetch', async () => {
    requests++
    return new Response(JSON.stringify({ success: false, data: null, error: { code: 'project_model_settings_conflict', message: '模型设置已变化' } }), { status: 409 })
  })
  await assert.rejects(updateModelSettings(0, { chat_area: null }), { status: 409, code: 'project_model_settings_conflict' })
  assert.equal(requests, 1)
})
