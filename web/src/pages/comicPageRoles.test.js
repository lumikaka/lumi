import assert from 'node:assert/strict'
import test from 'node:test'

import {
  comicBodyPageNumber,
  comicBodyReorderUuids,
  comicBodySections,
  comicPageFallbackTitle,
  comicPageLabel,
  comicPageRole,
  comicPageRoleOptionDisabled,
  reorderedComicBodyUuids,
} from './comicPageRoles.js'

const t = (key, values = {}) => ({
  'comic.page_role.front_cover': 'Front cover',
  'comic.page_role.body_label': `Body page ${values.number}`,
  'comic.page_role.back_cover': 'Back cover',
}[key] || key)

const pages = [
  { uuid: 'cover', section_no: 1, page_role: 'front_cover' },
  { uuid: 'body-a', section_no: 2, page_role: 'body' },
  { uuid: 'body-b', section_no: 3 },
  { uuid: 'back', section_no: 4, page_role: 'back_cover' },
]

test('page roles remain backward compatible and label body pages independently of binding order', () => {
  assert.equal(comicPageRole(pages[2]), 'body')
  assert.deepEqual(comicBodySections(pages).map((page) => page.uuid), ['body-a', 'body-b'])
  assert.equal(comicBodyPageNumber(pages, pages[1]), 1)
  assert.equal(comicBodyPageNumber(pages, pages[2]), 2)
  assert.equal(comicPageLabel(t, pages, pages[0]), 'Front cover')
  assert.equal(comicPageLabel(t, pages, pages[1]), 'Body page 1')
  assert.equal(comicPageLabel(t, pages, pages[3]), 'Back cover')
  assert.equal(comicPageLabel(t, [], { uuid: 'missing', page_role: 'body', section_no: 7, body_page_no: 3 }), 'Body page 3')
  assert.equal(comicPageFallbackTitle((key) => key, pages[0]), 'comic.page_role.front_cover_untitled')
})

test('cover slots are unique while the selected page may keep its current role', () => {
  assert.equal(comicPageRoleOptionDisabled(pages, 'front_cover'), true)
  assert.equal(comicPageRoleOptionDisabled(pages, 'front_cover', 'cover'), false)
  assert.equal(comicPageRoleOptionDisabled(pages, 'body'), false)
  assert.equal(comicPageRoleOptionDisabled([pages[0], pages[1]], 'back_cover', 'body-a'), true)
  assert.equal(comicPageRoleOptionDisabled([], 'front_cover'), true)
  assert.equal(comicPageRoleOptionDisabled([], 'back_cover'), true)
})

test('reordering emits only the complete ordered body UUID list and keeps covers fixed', () => {
  assert.deepEqual(comicBodyReorderUuids(pages, 'body-a', 1), ['body-b', 'body-a'])
  assert.equal(comicBodyReorderUuids(pages, 'cover', 1), null)
  assert.deepEqual(reorderedComicBodyUuids(pages, 'body-a', 'body-b', 'after'), ['body-b', 'body-a'])
  assert.equal(reorderedComicBodyUuids(pages, 'body-a', 'back', 'before'), null)
})
