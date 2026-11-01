import test from 'node:test'
import assert from 'node:assert/strict'
import { readWholeBook, bookGroups } from './book.js'
import { reviewContext } from './contracts.js'
import { PreviewCache } from './images.js'
const uuid = (n) => `01989abc-def0-7000-8000-${String(n).padStart(12, '0')}`
const project = uuid(1), chapter = uuid(2), secondChapter = uuid(3)
const wrap = (data) => ({ structuredContent: { success: true, data } })
const item = (n, ch = chapter, kind = 'body') => ({ uuid: uuid(n + 10), chapter_uuid: ch, chapter_title: ch === chapter ? '第一章' : '第二章', kind, revision: 1, title: `页 ${n}`, text: '', scene: '', open_url: `http://127.0.0.1:9999/projects/${project}/chapters/${ch}` })
const seed = (args = {}) => ({ view: 'book', project_uuid: project, target_uuid: project, revision: 1, title: '绘本', open_url: `http://127.0.0.1:9999/projects/${project}/chapters`, request: { name: 'review_book', arguments: args }, items: [] })
function response(data, args, items, total, more = false, cursor = items.length) {
  return wrap({ ...data, request: { name: 'review_book', arguments: args }, items, total, cursor_pagination: { per_page: args.limit || 20, prev_cursor: args.after || '0', next_cursor: String(cursor), has_more: more } })
}
test('whole book restarts at first cursor, drains >20 items and preserves chapter/cover/body/back order', async () => {
  const pages = [item(1, chapter, 'front_cover'), ...Array.from({ length: 21 }, (_, i) => item(i + 2)), item(23, chapter, 'back_cover'), item(24, secondChapter)]
  const calls = []
  const data = await readWholeBook(async (name, args) => {
    calls.push({ name, args })
    const start = Number(args.after || 0), end = Math.min(start + args.limit, pages.length)
    return response(seed(), args, pages.slice(start, end), pages.length, end < pages.length, end)
  }, seed({ after: '20', limit: 20 }))
  assert.equal(calls.length, 2); assert.equal(calls[0].args.after, undefined)
  assert.deepEqual(data.items, pages); assert.equal(data.complete, true)
  const groups = bookGroups(data)
  assert.deepEqual(groups.map((group) => group.title), ['第一章', '第二章'])
  assert.deepEqual(groups[0].pages.map((page) => page.label), ['封面', ...Array.from({ length: 21 }, (_, i) => `第 ${i + 1} 页`), '封底'])
  assert.equal(groups[1].pages[0].label, '第 1 页')
  assert.ok(calls.every((call) => call.name === 'review_book'))
})
test('chapter scope is retained across all requests and invalid scope is rejected', async () => {
  const scoped = seed({ chapter_uuid: chapter, limit: 1 })
  const data = await readWholeBook(async (_name, args) => {
    assert.equal(args.chapter_uuid, chapter)
    return response(scoped, args, [item(args.after ? 2 : 1)], 2, !args.after, args.after ? 2 : 1)
  }, scoped)
  assert.equal(data.items.length, 2)
  assert.throws(() => reviewContext(response(scoped, scoped.request.arguments, [item(1, secondChapter)], 1)))
  assert.throws(() => reviewContext(wrap({ ...scoped, scope: 'book' })))
})
test('oversized results reduce batch size; incomplete, duplicated and stalled pages never become a complete book', async () => {
  let calls = 0
  const data = await readWholeBook(async (_name, args) => {
    calls++
    if (args.limit > 1) return { structuredContent: { success: false, data: null, error: { code: 'result_too_large', message: 'too large' } } }
    return response(seed(), args, [item(1)], 1)
  }, seed({ limit: 2 }))
  assert.equal(data.items.length, 1); assert.equal(calls, 2)
  await assert.rejects(readWholeBook(async (_n, args) => response(seed(), args, [item(1)], 2), seed()), /不完整/)
  await assert.rejects(readWholeBook(async (_n, args) => response(seed(), args, [item(1)], 2, true, 0), seed()), /未前进/)
  await assert.rejects(readWholeBook(async (_n, args) => response(seed(), args, [item(1)], 2, true, 1), seed()), /顺序/)
  await assert.rejects(readWholeBook(async (_n, args) => {
    if (args.after) throw new Error('next batch failed')
    return response(seed(), args, [item(1)], 2, true, 1)
  }, seed()), /next batch failed/)
})
test('abort discards an obsolete batch and the adapter uses only the adopted asset', async () => {
  const controller = new AbortController()
  await assert.rejects(readWholeBook(async (_n, args) => {
    controller.abort()
    return response(seed(), args, [item(1)], 1)
  }, seed(), controller.signal), /abort/i)
  const adopted = { ...item(1), asset_uuid: uuid(100), variants: [{ asset_uuid: uuid(101) }] }
  assert.equal(bookGroups({ items: [adopted] })[0].pages[0].image_key, uuid(100))
})
test('decode retry evicts only the affected image and maintains byte accounting', async () => {
  let calls = 0
  const cache = new PreviewCache(async (_name, args) => {
    calls++
    return wrap({ asset_uuid: args.asset_uuid, mime_type: 'image/jpeg', width: 2, height: 1, data_uri: 'data:image/jpeg;base64,/9j/2Q==' })
  })
  await cache.load(project, uuid(100), 384)
  await cache.load(project, uuid(101), 384)
  const bytes = cache.bytes
  cache.evict(project, uuid(100), 384)
  assert.equal(cache.bytes, bytes / 2)
  await cache.load(project, uuid(101), 384); assert.equal(calls, 2)
  await cache.load(project, uuid(100), 384); assert.equal(calls, 3)
  assert.equal(cache.bytes, bytes)
})
