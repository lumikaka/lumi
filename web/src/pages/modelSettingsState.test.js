import assert from 'node:assert/strict'
import test from 'node:test'

import { INHERIT_MODEL_VALUE, modelOptionsForSetting, modelSelectionValue, parseModelSelection, imageThinkingSelection, imagePromptExtendSelection, imageThinkingEnabled } from './modelSettingsState.js'

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

test('thinking controls follow image capability and preserve explicit false', () => {
  const selected = { provider_uuid: 'bailian', model: 'qwen-image-3.0-pro', enable_thinking: false }
  const settings = { options: { image_models: [{ ...selected, ready: true, supports_thinking: true }] } }
  assert.deepEqual(imageThinkingSelection(settings, { kind: 'image', override: selected }), selected)
  assert.deepEqual(imageThinkingSelection(settings, { kind: 'image', effective: selected }), selected)
  assert.equal(imageThinkingSelection(settings, { kind: 'text', override: selected }), null)
  assert.equal(imageThinkingSelection(settings, { kind: 'image', override: selected, override_status: 'invalid' }), null)
  settings.options.image_models[0].supports_thinking = false
  assert.equal(imageThinkingSelection(settings, { kind: 'image', override: selected }), null)
})

test('prompt rewriting defaults on and suspends thinking without losing its preference', () => {
  const selection = { provider_uuid: 'bailian', model: 'qwen-image-3.0' }
  const settings = { options: { image_models: [{ ...selection, ready: true, supports_prompt_extend: true }] } }
  assert.deepEqual(imagePromptExtendSelection(settings, { kind: 'image', effective: selection }), selection)
  assert.equal(imageThinkingEnabled(selection), true)
  const paused = { ...selection, prompt_extend: false }
  assert.equal(imageThinkingEnabled(paused), false)
  assert.equal(imageThinkingEnabled({ ...paused, prompt_extend: true }), true)
  assert.equal(imageThinkingEnabled({ ...paused, prompt_extend: true, enable_thinking: false }), false)
  assert.equal(imageThinkingEnabled(null), false)
  assert.equal(imagePromptExtendSelection(settings, { kind: 'text', effective: selection }), null)
  assert.equal(imagePromptExtendSelection(settings, { kind: 'image', override: selection, override_status: 'invalid' }), null)
  settings.options.image_models[0].supports_prompt_extend = false
  assert.equal(imagePromptExtendSelection(settings, { kind: 'image', effective: selection }), null)
})
