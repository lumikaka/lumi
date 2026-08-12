import assert from 'node:assert/strict'
import test from 'node:test'

import { INHERIT_MODEL_VALUE, modelOptionsForSetting, modelSelectionValue, parseModelSelection } from './modelSettingsState.js'

test('model selections round trip without relying on a hard-coded model catalog', () => {
  const selection = { provider_uuid: 'provider-uuid', model: 'vendor/model::latest' }
  assert.deepEqual(parseModelSelection(modelSelectionValue(selection)), selection)
  assert.equal(modelSelectionValue(null), INHERIT_MODEL_VALUE)
  assert.equal(parseModelSelection(INHERIT_MODEL_VALUE), null)
})

test('model option lists follow the capability kind and only expose saveable ready models', () => {
  const settings = {
    options: {
      text_models: [{ model: 'text-ready', ready: true }, { model: 'text-unavailable', ready: false }],
      image_models: [{ model: 'image-ready', ready: true }, { model: 'image-unavailable', ready: false }],
    },
  }
  assert.deepEqual(modelOptionsForSetting(settings, { kind: 'text' }), [{ model: 'text-ready', ready: true }])
  assert.deepEqual(modelOptionsForSetting(settings, { kind: 'image' }), [{ model: 'image-ready', ready: true }])
})
