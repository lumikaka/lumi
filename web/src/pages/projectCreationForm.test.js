import assert from 'node:assert/strict'
import test from 'node:test'

import {
  MAX_OVERALL_STYLE_CHARACTERS,
  overallStyleForLanguage,
  overallStyleUsesDefault,
  projectCreationErrors,
  projectDefaultOverallStyle,
} from './projectCreationForm.js'

test('YOLO project creation reports every missing priority field', () => {
  assert.deepEqual(projectCreationErrors({
    name: ' ',
    parentPath: '',
    storyPrompt: '\n',
    createMode: 'yolo',
    pictureBookValid: true,
  }), {
    name: 'projects.validation.name_required',
    storyPrompt: 'projects.validation.story_idea_required',
    parentPath: 'projects.validation.parent_path_required',
  })
})

test('manual project creation does not require a story idea', () => {
  assert.deepEqual(projectCreationErrors({
    name: '月亮邮差',
    parentPath: '/tmp/Lumi',
    storyPrompt: '',
    createMode: 'manual',
    pictureBookValid: true,
  }), {})
})

test('project creation preserves picture-book validation', () => {
  assert.deepEqual(projectCreationErrors({
    name: '月亮邮差',
    parentPath: '/tmp/Lumi',
    storyPrompt: '一只小狐狸第一次独自穿过森林。',
    createMode: 'yolo',
    pictureBookValid: false,
  }), {
    pictureBook: 'projects.picture_book.custom.invalid',
  })
})

test('untouched overall style follows generation language while edited style is preserved', () => {
  const defaults = { 'zh-Hans': '默认中文画风', en: 'Default English style' }
  assert.equal(projectDefaultOverallStyle(defaults, 'zh-Hans'), '默认中文画风')
  assert.equal(overallStyleForLanguage({ currentStyle: '默认中文画风', dirty: false, defaultOverallStyles: defaults, generationLanguage: 'en' }), 'Default English style')
  assert.equal(overallStyleForLanguage({ currentStyle: 'My custom style', dirty: true, defaultOverallStyles: defaults, generationLanguage: 'en' }), 'My custom style')
  assert.equal(overallStyleUsesDefault('', defaults.en), true)
  assert.equal(overallStyleUsesDefault('Default English style', defaults.en), true)
  assert.equal(overallStyleUsesDefault('My custom style', defaults.en), false)
})

test('overall style validation counts Unicode characters and enforces the backend limit', () => {
  const base = { name: '月亮邮差', parentPath: '/tmp/Lumi', storyPrompt: '', createMode: 'manual', pictureBookValid: true }
  assert.deepEqual(projectCreationErrors({ ...base, overallStyle: '🎨'.repeat(MAX_OVERALL_STYLE_CHARACTERS) }), {})
  assert.deepEqual(projectCreationErrors({ ...base, overallStyle: '🎨'.repeat(MAX_OVERALL_STYLE_CHARACTERS + 1) }), {
    overallStyle: 'projects.validation.overall_style_too_long',
  })
})
