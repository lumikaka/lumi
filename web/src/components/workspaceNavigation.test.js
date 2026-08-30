import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import { workspaceRoute, workspaceSectionForPath } from './workspaceNavigation.js'

test('workspace routes collapse into the three top-level sections', () => {
  const project = '/projects/019fdb00-0000-7000-8000-000000000001'
  for (const suffix of ['', '/story', '/prompts', '/llm-logs', '/exports']) {
    assert.equal(workspaceSectionForPath(`${project}${suffix}`), 'overview')
  }
  for (const suffix of ['/premise', '/assets']) {
    assert.equal(workspaceSectionForPath(`${project}${suffix}`), 'premise')
  }
  for (const suffix of ['/chapters', '/chapters/019-chapter', '/chapters/019-chapter/sections/019-section']) {
    assert.equal(workspaceSectionForPath(`${project}${suffix}`), 'chapters')
  }
})

test('workspace route definitions merge collection filters with active chat context', () => {
  assert.deepEqual(
    workspaceRoute('019-project', 'chapters?state=trashed', '?chat_thread_uuid=019-thread'),
    {
      pathname: '/projects/019-project/chapters',
      search: '?chat_thread_uuid=019-thread&state=trashed',
    },
  )
  assert.deepEqual(
    workspaceRoute('019-project', 'chapters', '?chat_thread_uuid=019-thread&state=trashed'),
    {
      pathname: '/projects/019-project/chapters',
      search: '?chat_thread_uuid=019-thread',
    },
  )
})

test('workspace destinations keep public UUID paths and active ChatArea query', () => {
  assert.deepEqual(
    workspaceRoute('019 project/unsafe', 'chapters', '?chat_thread_uuid=019-thread&workflow_uuid=019-workflow'),
    {
      pathname: '/projects/019%20project%2Funsafe/chapters',
      search: '?chat_thread_uuid=019-thread&workflow_uuid=019-workflow',
    },
  )
})

test('chapter list uses its own compact toolbar instead of duplicate group tabs', () => {
  const source = readFileSync(new URL('./WorkspaceGroupTabs.jsx', import.meta.url), 'utf8')
  assert.match(source, /activeSection === 'chapters'.*endsWith\('\/chapters'\)/)
})
