import assert from 'node:assert/strict'
import test from 'node:test'

import { projectCreationErrors } from './projectCreationForm.js'

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
