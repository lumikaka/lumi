import assert from 'node:assert/strict'
import test from 'node:test'

import {
  applyOverallStylePresetDraft,
  promptIssues,
  promptPlaceholders,
  promptServerValues,
  promptUpdatePayload,
  reconcilePromptDrafts,
  resetPromptGroupDrafts,
} from './promptCatalogState.js'

const definition = (key, effective, version = 1, placeholders = []) => ({
  prompt_group: 'chapter',
  prompt_key: key,
  effective_value: effective,
  default_value: `default ${key}`,
  current_version: { version_no: version },
  placeholders,
})

test('prompt placeholder validation warns about missing values and rejects unknown values', () => {
  assert.deepEqual(promptPlaceholders('{{storyboard}} {{storyboard}} {{guidance_prompt}}'), ['guidance_prompt', 'storyboard'])
  assert.deepEqual(promptIssues({ placeholders: ['storyboard', 'style_prompt'] }, '{{storyboard}} {{invented}}'), {
    missing: ['style_prompt'],
    unknown: ['invented'],
  })
  assert.deepEqual(promptIssues({ placeholders: [] }, '{{not-supported}} {{ spaced }}'), {
    missing: [],
    unknown: [' spaced ', 'not-supported'],
  })
})

test('single prompt update payload submits one prompt with its optimistic version', () => {
  const payload = promptUpdatePayload(definition('section_image', 'image', 4), ' changed image ')
  assert.deepEqual(payload, {
    prompt_group: 'chapter',
    prompt_key: 'section_image',
    prompt: 'changed image',
    expected_current_version: 4,
  })
})

test('catalog refresh preserves local conflict drafts and refreshes untouched cards', () => {
  const previous = [definition('json_system', 'server one'), definition('section_image', 'image one')]
  const current = { 'chapter/json_system': 'local draft', 'chapter/section_image': 'image one' }
  const nextCatalog = [definition('json_system', 'server two', 2), definition('section_image', 'image two', 2)]
  const reconciled = reconcilePromptDrafts(current, promptServerValues(previous), nextCatalog)
  assert.equal(reconciled['chapter/json_system'], 'local draft')
  assert.equal(reconciled['chapter/section_image'], 'image two')
})

test('group default reset and art-style preset application only change local drafts', () => {
  const first = definition('json_system', 'custom')
  const second = definition('section_image', 'custom image')
  const reset = resetPromptGroupDrafts({ untouched: 'value' }, [first, second])
  assert.equal(reset['chapter/json_system'], 'default json_system')
  assert.equal(reset['chapter/section_image'], 'default section_image')
  assert.equal(reset.untouched, 'value')

  const preset = { prompt_group: 'premise_style', prompt_key: 'simple', effective_value: 'preset server' }
  const target = { prompt_group: 'premise_style', prompt_key: 'project_overall_style' }
  const applied = applyOverallStylePresetDraft({ 'premise_style/simple': 'preset draft', untouched: 'value' }, preset, target)
  assert.equal(applied['premise_style/project_overall_style'], 'preset draft')
  assert.equal(applied.untouched, 'value')
})
