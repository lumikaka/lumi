import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import {
  CHAPTER_CREATION_ACTIONS,
  chapterContinuationContext,
  chapterGenerationPlan,
  isSupportedStoryFile,
  mayPermanentlyDelete,
  nextChapterCode,
  saveStateForError,
  sortChaptersByDirection,
  storyConflictChoices,
} from './storyWorkspaceState.js'

test('chapter creation suggests the next active public chapter code', () => {
  assert.equal(nextChapterCode([]), 'vol01.ch01')
  assert.equal(nextChapterCode([
    { volume_no: 1, chapter_no: 2 },
    { volume_no: 1, chapter_no: 8 },
  ]), 'vol01.ch09')
})

test('chapter list sorting is stable, immutable, and follows public sort_order', () => {
  const chapters = [{ uuid: 'b', sort_order: 20 }, { uuid: 'a', sort_order: 10 }]
  assert.deepEqual(sortChaptersByDirection(chapters, 'asc').map((item) => item.uuid), ['a', 'b'])
  assert.deepEqual(sortChaptersByDirection(chapters, 'desc').map((item) => item.uuid), ['b', 'a'])
  assert.deepEqual(chapters.map((item) => item.uuid), ['b', 'a'])
})

test('chapter creation actions all map to working Lumi flows', () => {
  assert.deepEqual(CHAPTER_CREATION_ACTIONS.map((item) => item.key), ['batch', 'next', 'continue', 'manual', 'upload'])
})

test('chapter menus and dialogs expose keyboard state and explicit selected hover feedback', () => {
  const layout = readFileSync(new URL('./ChaptersWorkspace.jsx', import.meta.url), 'utf8')
  const styles = readFileSync(new URL('../styles/workspaces.sass', import.meta.url), 'utf8')
  assert.match(layout, /aria-haspopup="menu"/)
  assert.match(layout, /event\.key === 'Escape'/)
  assert.match(layout, /event\.key !== 'Tab'/)
  assert.match(layout, /aria-modal="true"/)
  assert.match(styles, /button\[aria-selected="true"\]:hover/)
  assert.match(styles, /chapter-create-menu__trigger\[aria-expanded="true"\]:hover/)
  assert.match(styles, /chapters-more-button\[aria-expanded="true"\]:hover/)
})

test('chapter toolbar defaults to a desktop row and only stacks at the mobile breakpoint', () => {
  const styles = readFileSync(new URL('../styles/workspaces.sass', import.meta.url), 'utf8')
  const desktopStart = styles.indexOf('.chapters-toolbar')
  const mobileToolbarStart = styles.indexOf('  .chapters-toolbar', desktopStart + 1)
  const mobileStart = styles.lastIndexOf('@media (max-width: 760px)', mobileToolbarStart)
  assert.ok(desktopStart >= 0 && mobileStart > desktopStart)

  const desktopStyles = styles.slice(desktopStart, mobileStart)
  const mobileStyles = styles.slice(mobileStart, styles.indexOf('@media', mobileToolbarStart))
  assert.match(desktopStyles, /\.chapters-toolbar[\s\S]*?display: flex/)
  assert.match(desktopStyles, /\.chapters-toolbar__actions[\s\S]*?display: flex/)
  assert.match(mobileStyles, /\.chapters-toolbar[\s\S]*?display: grid/)
  assert.match(mobileStyles, /\.chapters-toolbar__actions[\s\S]*?display: grid/)
})

test('generation plans cover batch, next, and continuation context', () => {
  const chapters = [{
    chapter_code: 'vol01.ch03', volume_no: 1, chapter_no: 3, sort_order: 3,
    current_story: { content: '上一章正文' },
  }]
  assert.deepEqual(chapterContinuationContext(chapters), {
    sourceChapter: chapters[0], targetChapterCode: 'vol01.ch04', hasCurrentStory: true,
  })
  const batch = chapterGenerationPlan({ mode: 'batch', chapters, prompt: '保持悬念', count: 3, storyMd: '# Story' })
  assert.deepEqual(batch.map((item) => item.chapterCode), ['vol01.ch04', 'vol01.ch05', 'vol01.ch06'])
  assert.equal(batch[0].prompt, '保持悬念')
  const next = chapterGenerationPlan({ mode: 'next', chapters, prompt: '推进旅程' })[0]
  assert.equal(next.prompt, '推进旅程')
  const continuation = chapterGenerationPlan({ mode: 'continue', chapters, prompt: '' })[0]
  assert.match(continuation.prompt, /承接上一章/)
  assert.doesNotMatch(continuation.prompt, /上一章正文：\n上一章正文/)
})

test('chapter import accepts only bounded txt and markdown files', () => {
  assert.equal(isSupportedStoryFile({ name: 'vol01.ch01.md', size: 1024 }), true)
  assert.equal(isSupportedStoryFile({ name: 'vol01.ch01.txt', size: 2 * 1024 * 1024 }), true)
  assert.equal(isSupportedStoryFile({ name: 'chapter.pdf', size: 100 }), false)
  assert.equal(isSupportedStoryFile({ name: 'chapter.md', size: 2 * 1024 * 1024 + 1 }), false)
})

test('editor exposes conflict state instead of hiding failed auto-save', () => {
  assert.equal(saveStateForError({ code: 'chapter_revision_conflict' }), 'conflict')
  assert.equal(saveStateForError({ code: 'story_md_conflict' }), 'conflict')
  assert.equal(saveStateForError({ code: 'disk_error' }), 'failed')
})

test('external STORY.md changes offer both explicit resolutions', () => {
  assert.deepEqual(storyConflictChoices({ projection_state: 'conflict' }), ['import_external', 'regenerate_database'])
  assert.deepEqual(storyConflictChoices({ projection_state: 'synced' }), [])
})

test('permanent deletion requires trash state and explicit confirmation', () => {
  assert.equal(mayPermanentlyDelete({ trashed_at: '2026-08-07T00:00:00Z' }, false), false)
  assert.equal(mayPermanentlyDelete({ trashed_at: null }, true), false)
  assert.equal(mayPermanentlyDelete({ trashed_at: '2026-08-07T00:00:00Z' }, true), true)
})

test('chapter trash empty-all uses a guarded LumiDialog and reports blockers safely', () => {
  const source = readFileSync(new URL('./StoryWorkspacePage.jsx', import.meta.url), 'utf8')
  assert.match(source, /emptyChapterTrash\(projectUuid\)/)
  assert.match(source, /<LumiDialog className="story-trash-empty-dialog"/)
  assert.match(source, /dismissDisabled=\{emptyMutation\.isPending\}/)
  assert.match(source, /emptyResult\.blocked_items/)
  assert.match(source, /item\.error_code/)
})

test('comic storyboard creation refreshes ChatArea lists without auto-opening the workflow', () => {
  const storySource = readFileSync(new URL('./StoryWorkspacePage.jsx', import.meta.url), 'utf8')
  const productionSource = readFileSync(new URL('./ProductionWorkspaces.jsx', import.meta.url), 'utf8')
  const mutationStart = storySource.indexOf('const comicGenerationMutation')
  const mutation = storySource.slice(mutationStart, storySource.indexOf('const trashMutation', mutationStart))
  for (const source of [mutation, productionSource.slice(productionSource.indexOf('const comicGenerate = useMutation'))]) {
    assert.match(source, /\['chat-threads', projectUuid\]/)
    assert.match(source, /\['workflows', projectUuid\]/)
  }
  assert.doesNotMatch(mutation, /setSelectedThreadUuid|chat_thread_uuid|workflow_uuid/)
})
