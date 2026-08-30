import assert from 'node:assert/strict'
import test from 'node:test'

import {
  createChapter,
  emptyChapterTrash,
  importChapters,
  importExternalStoryMD,
  listChapters,
  permanentlyDeleteChapter,
  reorderChapters,
  restoreChapter,
  restoreStoryProfileVersion,
  trashChapter,
  updateChapterStory,
} from './story.js'

function envelope(data) {
  return new Response(JSON.stringify({ success: true, data }), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

test('story API create, edit, trash and restore use UUID resources and revisions', async () => {
  const calls = []
  globalThis.fetch = async (path, options = {}) => {
    calls.push({ path, options })
    return envelope({ uuid: '01989abc-def0-7000-8000-000000000002' })
  }
  const projectUuid = '01989abc-def0-7000-8000-000000000001'
  const chapterUuid = '01989abc-def0-7000-8000-000000000002'

  await listChapters(projectUuid, 'trashed')
  await createChapter(projectUuid, { chapter_code: 'vol01.ch01', title: 'First', content: '', content_format: '' })
  await updateChapterStory(projectUuid, chapterUuid, { content: 'Draft', content_format: 'md', expected_revision: 1 })
  await trashChapter(projectUuid, chapterUuid, 2)
  await restoreChapter(projectUuid, chapterUuid, 3)
  await permanentlyDeleteChapter(projectUuid, chapterUuid, 4)
  await emptyChapterTrash(projectUuid)

  assert.equal(calls[0].path, `/api/v1/projects/${projectUuid}/chapters?state=trashed`)
  assert.equal(calls[1].options.method, 'POST')
  assert.deepEqual(JSON.parse(calls[2].options.body), { content: 'Draft', content_format: 'md', expected_revision: 1 })
  assert.equal(calls[3].path, `/api/v1/projects/${projectUuid}/chapters/${chapterUuid}?expected_revision=2`)
  assert.deepEqual(JSON.parse(calls[4].options.body), { expected_revision: 3 })
  assert.equal(calls[5].path, `/api/v1/projects/${projectUuid}/chapters/${chapterUuid}/permanent?expected_revision=4`)
  assert.equal(calls[6].path, `/api/v1/projects/${projectUuid}/chapters/trash`)
  assert.equal(calls[6].options.method, 'DELETE')
})

test('chapter ordering and story profile restoration use stable resource routes', async () => {
  const originalFetch = globalThis.fetch
  const calls = []
  globalThis.fetch = async (path, options = {}) => {
    calls.push({ path, options })
    return envelope({ items: [] })
  }
  const projectUuid = '01989abc-def0-7000-8000-000000000001'
  const versionUuid = '01989abc-def0-7000-8000-000000000009'
  try {
    await reorderChapters(projectUuid, ['chapter-b', 'chapter-a'])
    await restoreStoryProfileVersion(projectUuid, versionUuid, 8)
    assert.equal(calls[0].path, `/api/v1/projects/${projectUuid}/chapter-order`)
    assert.equal(calls[0].options.method, 'PUT')
    assert.deepEqual(JSON.parse(calls[0].options.body), { chapter_uuids: ['chapter-b', 'chapter-a'] })
    assert.equal(calls[1].path, `/api/v1/projects/${projectUuid}/story-profile/versions/${versionUuid}/restorations`)
    assert.equal(calls[1].options.method, 'POST')
    assert.deepEqual(JSON.parse(calls[1].options.body), { expected_revision: 8 })
  } finally {
    globalThis.fetch = originalFetch
  }
})

test('story file import uses multipart without persisting a local path', async () => {
  let captured
  globalThis.fetch = async (path, options = {}) => {
    captured = { path, options }
    return envelope({ uuid: '01989abc-def0-7000-8000-000000000003', items: [] })
  }
  const file = new Blob(['chapter body'], { type: 'text/markdown' })
  await importChapters('01989abc-def0-7000-8000-000000000001', {
    mode: 'single',
    files: [file],
    chapterCode: 'vol01.ch01',
    title: 'Imported',
  })

  assert.equal(captured.path, '/api/v1/projects/01989abc-def0-7000-8000-000000000001/chapter-imports')
  assert.equal(captured.options.method, 'POST')
  assert.equal(captured.options.headers['Content-Type'], undefined)
  assert.equal(captured.options.body.get('mode'), 'single')
  assert.equal(captured.options.body.get('chapter_code'), 'vol01.ch01')
  assert.equal(captured.options.body.get('title'), 'Imported')
  assert.equal(captured.options.body.has('root_path'), false)
})

test('external STORY.md conflict action sends only expected revision', async () => {
  let captured
  globalThis.fetch = async (path, options = {}) => {
    captured = { path, options }
    return envelope({ projection_state: 'synced', revision: 3 })
  }
  await importExternalStoryMD('01989abc-def0-7000-8000-000000000001', 2)
  assert.equal(captured.path, '/api/v1/projects/01989abc-def0-7000-8000-000000000001/story-profile/imports')
  assert.deepEqual(JSON.parse(captured.options.body), { expected_revision: 2 })
})
