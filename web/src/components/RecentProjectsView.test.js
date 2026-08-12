import assert from 'node:assert/strict'
import test from 'node:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

import RecentProjectsView, { projectErrorCopy } from './RecentProjectsView.js'

test('recent projects renders the first-use empty state', () => {
  const html = renderToStaticMarkup(createElement(RecentProjectsView, { items: [] }))
  assert.match(html, /从第一本绘本开始/)
  assert.match(html, /自包含项目/)
})

test('recent projects can be entered without claiming the indexed path is verified', () => {
  const html = renderToStaticMarkup(createElement(RecentProjectsView, {
    items: [{
      uuid: '019fdb00-0000-7000-8000-000000000001',
      name: 'Recent Book',
      root_path: '/offline/Recent Book',
      status: 'recent',
      status_detail: '',
      available: true,
		open: false,
    }],
  }))
  assert.match(html, /最近使用/)
  assert.match(html, />进入项目</)
  assert.match(html, /重新定位/)
  assert.match(html, /从最近列表移除/)
})

test('recent projects keeps missing paths recoverable', () => {
  const html = renderToStaticMarkup(createElement(RecentProjectsView, {
    items: [{
      uuid: '019fdb00-0000-7000-8000-000000000001',
      name: 'Moved Book',
      root_path: '/offline/Moved Book',
      status: 'project_not_found',
      status_detail: '目录可能已移动，或所在磁盘暂时离线。',
      available: false,
		open: false,
    }],
  }))
  assert.match(html, /路径已失效/)
  assert.match(html, /重新定位/)
  assert.match(html, /从最近列表移除/)
  assert.doesNotMatch(html, />进入项目</)
})

test('recent projects explains newer formats and permission failures', () => {
  const html = renderToStaticMarkup(createElement(RecentProjectsView, {
    items: [
      { uuid: '019-newer', name: 'Future', root_path: '/future', status: 'project_format_too_new', available: false },
      { uuid: '019-readonly', name: 'Read only', root_path: '/readonly', status: 'project_permission_denied', available: false },
    ],
  }))
  assert.match(html, /需要升级 Lumi/)
  assert.match(html, /没有写权限/)
})

test('project action errors cover migration and cross-instance locks', () => {
  assert.equal(projectErrorCopy.project_migration_failed, 'projects.error.project_migration_failed')
  assert.equal(projectErrorCopy.project_locked, 'projects.error.project_locked')
  assert.equal(projectErrorCopy.project_permission_denied, 'projects.error.project_permission_denied')
})
