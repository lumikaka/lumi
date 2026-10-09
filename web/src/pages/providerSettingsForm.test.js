import assert from 'node:assert/strict'
import test from 'node:test'
import { providerSettingsChanges } from './providerSettingsForm.js'

const prefix = 'ai_providers.openai_relay'
const form = { base_url: 'https://text.example/v1', image_base_url: 'https://image.example/v1', default_model: 'gpt-6', default_image_model: 'gpt-6.1-sol' }

test('relay saves independent endpoints, models and trimmed secrets', () => {
  assert.deepEqual(providerSettingsChanges(prefix, form, ' text-key ', ' image-key '), {
    [`${prefix}.base_url`]: form.base_url,
    [`${prefix}.image_base_url`]: form.image_base_url,
    [`${prefix}.default_model`]: 'gpt-6',
    [`${prefix}.default_image_model`]: 'gpt-6.1-sol',
    [`${prefix}.api_key`]: 'text-key',
    [`${prefix}.image_api_key`]: 'image-key',
  })
})

test('blank keys preserve saved credentials independently', () => {
  for (const [text, image] of [['', ''], ['new-text', ''], ['', 'new-image']]) {
    const settings = providerSettingsChanges(prefix, form, text, image)
    assert.equal(Object.hasOwn(settings, `${prefix}.api_key`), Boolean(text))
    assert.equal(Object.hasOwn(settings, `${prefix}.image_api_key`), Boolean(image))
  }
  const existing = providerSettingsChanges('ai_providers.aliyun_bailian', { workspace_id: 'demo' }, 'shared-key', 'unused')
  assert.equal(existing['ai_providers.aliyun_bailian.api_key'], 'shared-key')
  assert.equal(Object.hasOwn(existing, 'ai_providers.aliyun_bailian.image_api_key'), false)
})

test('switching image protocol requires an explicit compatible model selection', async () => {
  const { changeRelayImageMode } = await import('./providerSettingsForm.js')
  const models = { responses: ['gpt-6.1-sol'], images: ['gpt-image-2'] }
  const current = { ...form, image_api_mode: 'responses' }
  const switched = changeRelayImageMode(current, 'images', models)
  assert.equal(switched.default_image_model, '')
  assert.equal(switched.default_model, form.default_model)
  assert.equal(switched.image_base_url, form.image_base_url)
  assert.equal(current.image_api_mode, 'responses')
  const selected = { ...switched, default_image_model: 'gpt-image-2' }
  assert.equal(changeRelayImageMode(selected, 'images', models).default_image_model, 'gpt-image-2')
  assert.equal(changeRelayImageMode(selected, 'responses', models).default_image_model, '')
  const changes = providerSettingsChanges(prefix, selected, '', '')
  assert.equal(changes[`${prefix}.image_api_mode`], 'images')
  assert.equal(changes[`${prefix}.default_image_model`], 'gpt-image-2')
})
