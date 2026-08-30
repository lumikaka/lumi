import assert from 'node:assert/strict'
import test from 'node:test'

import { sidebarProjectCreateInput } from './sidebarProjectCreatorState.js'

test('sidebar project creation uses only a name plus the server defaults', () => {
  const defaults = {
    parent_path: '/Users/me/Documents/Lumi',
    default_overall_styles: {
      'zh-Hans': '默认中文画风',
      en: 'Default English art style',
    },
  }

  assert.deepEqual(sidebarProjectCreateInput('  月亮邮差  ', defaults, 'zh-Hans'), {
    name: '月亮邮差',
    parentPath: '/Users/me/Documents/Lumi',
    generationLanguage: 'zh-Hans',
    pictureBook: {
      format: 'classic_picture_book',
      aspect_ratio: { mode: 'landscape' },
      large_image_minimal_text: false,
    },
    overallStyle: '默认中文画风',
  })

  assert.equal(sidebarProjectCreateInput('Moon', defaults, 'en').generationLanguage, 'en')
  assert.equal(sidebarProjectCreateInput('Moon', defaults, 'en').overallStyle, 'Default English art style')
})
