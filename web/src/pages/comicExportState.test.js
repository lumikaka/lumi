import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import { comicExportEmptyError, comicExportOperationState, comicExportReadinessDecision, comicExportSnapshotMetrics } from './comicExportState.js'

test('comic export readiness blocks empty exports and confirms partial exports', () => {
  assert.equal(comicExportReadinessDecision(null), 'blocked')
  assert.equal(comicExportReadinessDecision({ can_export: false, image_section_count: 0, missing_section_count: 4 }), 'blocked')
  assert.equal(comicExportReadinessDecision({ can_export: true, image_section_count: 2, missing_section_count: 1 }), 'confirm')
  assert.equal(comicExportReadinessDecision({ can_export: true, image_section_count: 2, missing_section_count: 0 }), 'ready')
})

test('empty comic export errors remain distinguishable from network failures', () => {
  assert.deepEqual(comicExportEmptyError(), { code: 'production_export_empty', status: 422 })
})

test('comic export dialog projects stable operation states and frozen snapshot metrics', () => {
  assert.equal(comicExportOperationState({ status: 'running' }, { status: 'running' }), 'running')
  assert.equal(comicExportOperationState({ status: 'completed' }, { status: 'running' }), 'finalizing')
  assert.equal(comicExportOperationState({ status: 'completed' }, { status: 'ready', download_url: '/media/export' }), 'ready')
  assert.deepEqual(comicExportSnapshotMetrics(null, { snapshot: { version: 2, section_count: 5, exported_section_count: 4, missing_section_count: 1 } }), { version: 2, total: 5, ready: 4, missing: 1 })
})

test('project and chapter export histories use independent server pagination', () => {
  const source = readFileSync(new URL('./ProjectOverviewPanels.jsx', import.meta.url), 'utf8')
  assert.match(source, /const \[projectPage, setProjectPage\] = useState\(1\)/)
  assert.match(source, /const \[chapterPage, setChapterPage\] = useState\(1\)/)
  assert.match(source, /listComicExports\(projectUuid, \{ page: projectPage, perPage: exportPageSize, scope: 'project' \}\)/)
  assert.match(source, /listComicExports\(projectUuid, \{ page: chapterPage, perPage: exportPageSize, scope: 'chapter' \}\)/)
  assert.match(source, /projectPagination\.total \+ chapterPagination\.total/)
})

test('all new comic export entry points open the shared operation dialog while history keeps direct downloads', () => {
  const overview = readFileSync(new URL('./ProjectOverviewPanels.jsx', import.meta.url), 'utf8')
  const chapter = readFileSync(new URL('./ChapterWorkbenchPage.jsx', import.meta.url), 'utf8')
  const comic = readFileSync(new URL('./ProductionWorkspaces.jsx', import.meta.url), 'utf8')
  const dialog = readFileSync(new URL('../components/ComicExportDialog.jsx', import.meta.url), 'utf8')
  for (const source of [overview, chapter, comic]) {
    assert.match(source, /<ComicExportDialog/)
    assert.match(source, /comicExportDialogRequest/)
  }
  assert.match(overview, /item\.download_url/)
  assert.match(comic, /item\.download_url/)
  assert.match(dialog, /cancelProductionTask/)
  assert.match(dialog, /retryProductionTask/)
  assert.match(dialog, /exportRecord\.download_url/)
  for (const source of [overview, comic, dialog]) assert.doesNotMatch(source, /output_asset/)
  assert.doesNotMatch(dialog, /refetchInterval/)
  assert.match(dialog, /loadActiveOperation/)
  assert.match(dialog, /nextError\?\.code === 'task_conflict'/)
  assert.match(dialog, /onClick=\{onClose\}/)
  assert.match(dialog, /\{!ready \? <div className="comic-export-dialog__progress"/)
  assert.match(dialog, /projects\.exports\.created[\s\S]*comic-export-dialog__hash/)
  assert.doesNotMatch(dialog, /initializedRef/)
})
