import assert from 'node:assert/strict'
import test from 'node:test'

import {
  firstReadySimpleImage,
  orderedSimplePages,
  simplePageCounts,
  simpleProjectChatReference,
  simpleProjectRouteState,
  simpleStoryExcerpt,
  storyDocumentBlocks,
} from './simpleProjectState.js'

const projectUuid = '01990c73-4ca2-7aa1-8f4b-0555633ff810'
const chapterUuid = '01990c73-4ca2-7aa1-8f4b-0555633ff811'
const sectionUuid = '01990c73-4ca2-7aa1-8f4b-0555633ff812'
const assetUuid = '01990c73-4ca2-7aa1-8f4b-0555633ff813'

test('simple dashboard deep links parse into stable public-resource route state', () => {
  const base = `/projects/${projectUuid}`
  assert.deepEqual(simpleProjectRouteState(base, projectUuid), { key: 'home', assetUuid: '', chapterUuid: '', sectionUuid: '' })
  assert.deepEqual(simpleProjectRouteState(`${base}/settings`, projectUuid), { key: 'configuration', assetUuid: '', chapterUuid: '', sectionUuid: '' })
  assert.deepEqual(simpleProjectRouteState(`${base}/llm-logs`, projectUuid), { key: 'llm_logs', assetUuid: '', chapterUuid: '', sectionUuid: '' })
  assert.deepEqual(simpleProjectRouteState(`${base}/exports`, projectUuid), { key: 'exports', assetUuid: '', chapterUuid: '', sectionUuid: '' })
  assert.deepEqual(simpleProjectRouteState(`${base}/premise/assets/${assetUuid}`, projectUuid), { key: 'setting', assetUuid, chapterUuid: '', sectionUuid: '' })
  assert.deepEqual(simpleProjectRouteState(`${base}/chapters/${chapterUuid}`, projectUuid), { key: 'pages', assetUuid: '', chapterUuid, sectionUuid: '' })
  assert.deepEqual(simpleProjectRouteState(`${base}/chapters/${chapterUuid}/sections/${sectionUuid}`, projectUuid), { key: 'page', assetUuid: '', chapterUuid, sectionUuid })
  assert.deepEqual(simpleProjectRouteState(`${base}/chapters/${chapterUuid}/preview`, projectUuid), { key: 'book', assetUuid: '', chapterUuid, sectionUuid: '' })
  assert.equal(simpleProjectRouteState(`${base}/unknown`, projectUuid).key, 'not_found')
})

test('ChatArea receives exact setting, chapter, and page reference contexts', () => {
  const imageUuid = '01990c73-4ca2-7aa1-8f4b-0555633ff815'
  const setting = simpleProjectChatReference(
    { key: 'setting', assetUuid },
    { asset: { uuid: assetUuid, title: '小狐狸', current_variant: { asset: { uuid: imageUuid } } } },
  )
  assert.deepEqual(setting, {
    localId: `premise_asset:${assetUuid}`,
    resource_type: 'premise_asset',
    resource_uuid: assetUuid,
    title: '小狐狸',
    image_file_uuid: imageUuid,
    image_available: true,
    status: 'ready',
  })

  const chapter = simpleProjectChatReference(
    { key: 'book', chapterUuid },
    { chapter: { uuid: chapterUuid, chapter_code: 'CH-01', title: '月光森林' } },
  )
  assert.equal(chapter.resource_type, 'chapter')
  assert.equal(chapter.resource_uuid, chapterUuid)
  assert.equal(chapter.title, 'CH-01 · 月光森林')

  const page = simpleProjectChatReference(
    { key: 'page', sectionUuid },
    { section: { uuid: sectionUuid, title: '跨过溪流', current_image: { asset: { uuid: imageUuid } } } },
  )
  assert.equal(page.resource_type, 'comic_section')
  assert.equal(page.resource_uuid, sectionUuid)
  assert.equal(page.image_file_uuid, imageUuid)
  const chapterEntryPage = simpleProjectChatReference(
    { key: 'pages', chapterUuid },
    { section: { uuid: sectionUuid, title: '封面', current_image: { asset: { uuid: imageUuid } } } },
  )
  assert.equal(chapterEntryPage.resource_type, 'comic_section')
  assert.equal(chapterEntryPage.resource_uuid, sectionUuid)
  const emptyChapterEntry = simpleProjectChatReference(
    { key: 'pages', chapterUuid },
    { chapter: { uuid: chapterUuid, chapter_code: 'CH-01', title: '月光森林' } },
  )
  assert.equal(emptyChapterEntry.resource_type, 'chapter')
  assert.equal(emptyChapterEntry.resource_uuid, chapterUuid)
  assert.equal(simpleProjectChatReference({ key: 'home' }), null)
  assert.equal(simpleProjectChatReference({ key: 'page', sectionUuid }, { section: { uuid: 'different' } }), null)
})

test('whole-book pages are ordered as cover, numbered body pages, then back cover', () => {
  const sections = [
    { uuid: 'back', page_role: 'back_cover', section_no: 99 },
    { uuid: 'body-2', page_role: 'body', body_page_no: 2, section_no: 3, current_image: { asset: { status: 'ready', content_url: '/two.png' } } },
    { uuid: 'front', page_role: 'front_cover', section_no: 1, current_image: { asset: { uuid: 'cover-image', status: 'ready', content_url: '/cover.png' } } },
    { uuid: 'body-1', page_role: 'body', body_page_no: 1, section_no: 2, current_image: { asset: { status: 'queued' } } },
  ]
  assert.deepEqual(orderedSimplePages(sections).map((section) => section.uuid), ['front', 'body-1', 'body-2', 'back'])
  assert.equal(firstReadySimpleImage(sections).uuid, 'cover-image')
  assert.deepEqual(simplePageCounts(sections), { total: 2, ready: 1 })
  assert.deepEqual(sections.map((section) => section.uuid), ['back', 'body-2', 'front', 'body-1'])
})

test('story display turns persisted Markdown into safe readable blocks and excerpts', () => {
  const markdown = '# 第一章\n月光下的 **小狐狸** 走向 [森林](https://example.com)。\n\n<!-- hidden -->\n\n> 它听见溪水。'
  assert.deepEqual(storyDocumentBlocks(markdown), [
    { type: 'heading', text: '第一章' },
    { type: 'paragraph', text: '月光下的 小狐狸 走向 森林。' },
    { type: 'paragraph', text: '它听见溪水。' },
  ])
  assert.equal(simpleStoryExcerpt(markdown, 12), '第一章 月光下的 小狐狸…')
})
