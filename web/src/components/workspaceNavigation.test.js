import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import { canvasModeForPath, workspaceRoute, workspaceSectionForPath } from './workspaceNavigation.js'

test('workspace routes collapse into the three top-level sections', () => {
  const project = '/projects/019fdb00-0000-7000-8000-000000000001'
  for (const suffix of ['', '/overview', '/overview/summary', '/overview/profile', '/overview/prompts', '/overview/llm-logs', '/overview/exports', '/story', '/prompts']) {
    assert.equal(workspaceSectionForPath(`${project}${suffix}`), 'overview')
  }
  for (const suffix of ['/premise', '/assets']) {
    assert.equal(workspaceSectionForPath(`${project}${suffix}`), 'premise')
  }
  for (const suffix of ['/chapters', '/chapters/019-chapter', '/comic', '/comic/019-chapter', '/trash']) {
    assert.equal(workspaceSectionForPath(`${project}${suffix}`), 'chapters')
  }
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

test('canvas modes map existing domain routes into premise, chapters and works', () => {
  const project = '/projects/019fdb00-0000-7000-8000-000000000001'
  assert.equal(canvasModeForPath(`${project}/premise`), 'premise')
  assert.equal(canvasModeForPath(`${project}/chapters/019-chapter`), 'chapters')
  assert.equal(canvasModeForPath(`${project}/comic/019-chapter`), 'works')
})

test('chapter list uses its own compact toolbar instead of duplicate group tabs', () => {
  const source = readFileSync(new URL('./WorkspaceGroupTabs.jsx', import.meta.url), 'utf8')
  assert.match(source, /activeSection === 'chapters'.*endsWith\('\/chapters'\)/)
})
