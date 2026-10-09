import assert from 'node:assert/strict'
import test from 'node:test'
import { initialTestSelection, modelTestPage, repeatTestSelection, testModelAvailable, testModelOptions } from './modelTestState.js'

const text = { provider_uuid: 'configured', model: 'text', ready: false }
const image = { provider_uuid: 'configured', model: 'image', ready: false }
const unavailable = { provider_uuid: 'missing-key', model: 'other', ready: false }
const settings = {
  options: { text_models: [text, unavailable], image_models: [image] },
  settings: { project_text: { effective: text }, project_image: { effective: image } },
}
const providers = { items: [{ uuid: 'configured', configured: true, verified: false }, { uuid: 'missing-key', configured: false }] }

test('diagnostic choices include configured but unverified providers and separate capabilities', () => {
  assert.deepEqual(testModelOptions(settings, providers, 'text'), [text])
  assert.deepEqual(testModelOptions(settings, providers, 'image'), [image])
  assert.equal(testModelAvailable([text], image), false)
  assert.equal(testModelAvailable([text], null), false)
})

test('defaults use current global selections but unavailable defaults require user selection', () => {
  assert.deepEqual(initialTestSelection({}, settings, providers, 'text'), { provider_uuid: text.provider_uuid, model: text.model })
  assert.equal(initialTestSelection({}, settings, { items: [] }, 'text'), null)
  assert.equal(initialTestSelection({}, { options: {} }, providers, 'image'), null)
})

test('repeat keeps independent original models, even if they later become unavailable', () => {
  const original = { text_model: { ...text, provider_name: 'Provider' }, image_model: image }
  const copied = repeatTestSelection(original)
  assert.deepEqual(copied, { text_model: { provider_uuid: text.provider_uuid, model: text.model }, image_model: { provider_uuid: image.provider_uuid, model: image.model } })
  assert.deepEqual(initialTestSelection(copied, settings, { items: [] }, 'text'), copied.text_model)
  assert.notEqual(copied.text_model, original.text_model)
})

test('history page input stays within the supported server range', () => {
  for (const value of [null, '', 'NaN', '-1', '1.5', '1000001', 'Infinity']) assert.equal(modelTestPage(value), 1)
  assert.equal(modelTestPage('2'), 2)
})
