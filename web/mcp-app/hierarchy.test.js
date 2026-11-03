import test from 'node:test'
import assert from 'node:assert/strict'
import { reviewContext } from './contracts.js'
const uuid = n => `01989abc-def0-7000-8000-${String(n).padStart(12, '0')}`
const project = uuid(1), chapter = uuid(2), page = uuid(3)
const base = `http://127.0.0.1:49237/projects/${project}/`
const wrap = data => ({ structuredContent: { success: true, data } })
const ch = () => ({ uuid: chapter, revision: 3, chapter_code: 'vol01.ch01', title: '第一章', summary: '摘要', story: '完整故事', page_count: 23, image_count: 0, open_url: `${base}chapters/${chapter}` })
const item = n => ({ uuid: uuid(n + 3), revision: 1, chapter_uuid: chapter, title: `第${n + 1}页`, text: '完整正文', scene: '完整分镜', kind: 'body', open_url: `${base}chapters/${chapter}/sections/${uuid(n + 3)}` })
const data = (view, args = {}) => ({ contract_version: 2, view, project_uuid: project, target_uuid: project, revision: 1, title: '作品', open_url: base, request: { name: `review_${view}`, arguments: args }, items: [] })
const slice = (d, args, rows, total, start = 0) => ({ ...d, request: { ...d.request, arguments: args }, items: rows, total, snapshot: 'a'.repeat(64), cursor_pagination: { per_page: args.limit || 20, prev_cursor: String(start), next_cursor: String(start + rows.length), has_more: start + rows.length < total } })
function chapterData(args = {}) { return { ...data('chapter', args), target_uuid: chapter, revision: 3, chapter: ch(), open_url: ch().open_url } }
test('four view contracts distinguish work, chapter collection, chapter and page with runtime URLs', () => {
  const book = { ...data('book'), book: { description: '简介', generation_language: 'zh-Hans', setup_status: 'ready', picture_book: null, chapter_count: 1, page_count: 23, image_count: 0 } }
  assert.equal(reviewContext(wrap(book)).view, 'book')
  const chapters = { ...slice(data('chapters'), {}, [], 1), chapters: [ch()], open_url: `${base}chapters`, cursor_pagination: { per_page: 20, prev_cursor: '0', next_cursor: '1', has_more: false } }
  assert.equal(reviewContext(wrap(chapters)).chapters[0].page_count, 23)
  assert.equal(reviewContext(wrap(slice(chapterData({ chapter_uuid: chapter }), { chapter_uuid: chapter }, [item(0)], 23))).target_uuid, chapter)
  const single = { ...data('page', { chapter_uuid: chapter, page_uuid: page }), chapter: ch(), target_uuid: page, revision: 1, open_url: item(0).open_url, items: [item(0)], total: 23, position: 1, next_page_uuid: uuid(4) }
  assert.equal(reviewContext(wrap(single)).items[0].text, '完整正文')
  for (const invalid of [{ ...book, target_uuid: chapter }, { ...book, open_url: `${base}chapters` }, { ...chapters, chapters: [{ ...ch(), image_count: 24 }] }, { ...single, chapter: { ...ch(), uuid: project } }, { ...single, revision: 4 }, { ...single, previous_page_uuid: uuid(9) }, { ...single, next_page_uuid: undefined }]) assert.throws(() => reviewContext(wrap(invalid)))
})
