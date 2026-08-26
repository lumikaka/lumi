import assert from 'node:assert/strict'
import test from 'node:test'

import { localizedErrorPresentation } from './errorLocalization.js'
import { sourceTypeLabel, statusLabel } from './labels.js'
import { RESOURCES } from './messages/index.js'
import { translateMessage } from './runtime.js'

const translator = (locale) => (key, values) => translateMessage(RESOURCES, locale, key, values)

test('status and source labels localize known values and preserve unknown machine codes', () => {
  assert.equal(statusLabel(translator('en'), 'queued'), 'Queued')
  assert.equal(sourceTypeLabel(translator('zh-Hans'), 'ai_generated'), 'AI 生成')
  assert.equal(statusLabel(translator('en'), 'custom_state'), 'Unknown (custom_state)')
})

test('known API errors use localized copy and retain raw details only for diagnostics', () => {
  const presentation = localizedErrorPresentation(translator('en'), {
    code: 'project_not_found',
    message: '项目目录不存在',
    details: '/Volumes/offline/book',
    status: 404,
  })
  assert.equal(presentation.message, 'The project record or folder is unavailable. It may have been removed from recent projects, moved, or its disk may be offline.')
  assert.equal(presentation.diagnostic, '项目目录不存在\n/Volumes/offline/book')

  const notOpen = localizedErrorPresentation(translator('zh-Hans'), {
    code: 'project_not_open',
    message: '项目尚未打开',
    status: 409,
  })
  assert.equal(notOpen.message, '项目尚未打开，Lumi 将尝试重新打开。')

  const exhaustedEnglish = localizedErrorPresentation(translator('en'), {
    code: 'project_directory_name_exhausted',
    status: 409,
  })
  const exhaustedChinese = localizedErrorPresentation(translator('zh-Hans'), {
    code: 'project_directory_name_exhausted',
    status: 409,
  })
  assert.equal(exhaustedEnglish.message, 'All automatically numbered folders for this project name are in use. Choose another project name or parent folder.')
  assert.equal(exhaustedChinese.message, '同名项目目录的自动编号已用尽，请更换项目名称或父目录。')
  assert.equal(
    localizedErrorPresentation(translator('en'), { code: 'default_project_parent_unavailable', status: 500 }).message,
    'The system Documents folder could not be resolved. Check the system folder settings and try again.',
  )

  assert.equal(
    localizedErrorPresentation(translator('en'), { code: 'chat_reference_invalid_mime', status: 422 }).message,
    'Only PNG, JPEG, or WebP images can be attached.',
  )
  assert.equal(
    localizedErrorPresentation(translator('zh-Hans'), { code: 'chat_reference_limit_exceeded', status: 422 }).message,
    '每条消息最多引用 16 个资源。',
  )
  assert.equal(
    localizedErrorPresentation(translator('zh-Hans'), { code: 'production_export_empty', status: 422 }).message,
    '没有可导出的漫画图片。请先为至少一个 Section 生成或导入图片。',
  )
})

test('unknown API and persisted task errors never expose server copy as the interface message', () => {
  const api = localizedErrorPresentation(translator('en'), { code: 'future_error', message: '中文服务端消息', status: 500 })
  const task = localizedErrorPresentation(translator('en'), { code: 'future_task_error', message: '任务失败' })
  assert.equal(api.message, 'The Lumi service encountered a problem. Please try again later.')
  assert.equal(task.message, 'The operation could not be completed. Please try again.')
  assert.equal(api.diagnostic, '中文服务端消息')
})
