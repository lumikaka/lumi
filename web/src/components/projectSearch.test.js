import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import { filterProjectSearchResults, projectSearchDialogHeight } from './projectSearch.js'

const projects = [
  { uuid: 'moon', name: '月亮邮差', root_path: '/books/moon' },
  { uuid: 'umbrella', name: '会发芽的雨伞', root_path: '/books/umbrella' },
  { uuid: 'cloud', name: 'Cloud Collector', root_path: '/books/cloud' },
]

test('project search matches names and local paths with whitespace AND semantics', () => {
  assert.deepEqual(filterProjectSearchResults(projects, '月亮').map((item) => item.uuid), ['moon'])
  assert.deepEqual(filterProjectSearchResults(projects, 'books umbrella').map((item) => item.uuid), ['umbrella'])
  assert.deepEqual(filterProjectSearchResults(projects, 'CLOUD').map((item) => item.uuid), ['cloud'])
  assert.strictEqual(filterProjectSearchResults(projects, ''), projects)
})

test('project search dialog height follows the initial result capacity instead of the filtered count', () => {
  assert.equal(projectSearchDialogHeight(0), 224)
  assert.equal(projectSearchDialogHeight(1), 224)
  assert.equal(projectSearchDialogHeight(3), 332)
  assert.equal(projectSearchDialogHeight(8), 632)
  assert.equal(projectSearchDialogHeight(20), 632)
})

test('project search dialog uses the shared modal and switches only available projects', () => {
  const source = readFileSync(new URL('./ProjectSearchDialog.jsx', import.meta.url), 'utf8')
  assert.match(source, /<LumiDialog[\s\S]*aria-labelledby=\{titleId\}/)
  assert.match(source, /autoFocus[\s\S]*type="search"/)
  assert.match(source, /inputRef\.current\?\.focus\(\)/)
  assert.match(source, /projectSearchDialogHeight\(projects\.length\)/)
  assert.match(source, /event\.key !== 'Escape'[\s\S]*onClose\?\.\(\)/)
  assert.match(source, /disabled=\{!available\}/)
  assert.match(source, /onSwitchProject\?\.\(project\.uuid\)/)
})
