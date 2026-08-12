import assert from 'node:assert/strict'
import test from 'node:test'

import {
  createProject,
	getProjectDefaults,
	ensureProjectOpen,
	openProjectPath,
  relocateRecentProject,
  preflightImageGeneration,
  preflightProjectImageGeneration,
  selectProjectDirectory,
} from './projects.js'

function success(data = {}) {
  return new Response(JSON.stringify({ success: true, data }), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  })
}

test('project API models create and open flows with snake_case JSON', async (t) => {
  t.mock.method(globalThis, 'fetch', async () => success({ uuid: '019-project' }))

  const pictureBook = { format: 'classic_picture_book', aspect_ratio: { mode: 'landscape' }, large_image_minimal_text: false }
  await createProject({ name: 'Moon', parentPath: '/books', pictureBook })
	await ensureProjectOpen('019-recent')
  await openProjectPath('/books/moon')
  await selectProjectDirectory('/books')

  const calls = globalThis.fetch.mock.calls.map((call) => call.arguments)
  assert.equal(calls[0][0], '/api/v1/projects')
  assert.deepEqual(JSON.parse(calls[0][1].body), { name: 'Moon', parent_path: '/books', generation_language: 'zh-Hans', picture_book: pictureBook })
	assert.equal(calls[1][0], '/api/v1/open-projects/019-recent')
	assert.equal(calls[1][1].method, 'PUT')
	assert.equal(calls[1][1].body, undefined)
  assert.deepEqual(JSON.parse(calls[2][1].body), { root_path: '/books/moon' })
  assert.equal(calls[3][0], '/api/v1/directory-selections')
  assert.deepEqual(JSON.parse(calls[3][1].body), { initial_path: '/books' })
})

test('project API reads platform project defaults', async (t) => {
  t.mock.method(globalThis, 'fetch', async () => success({ parent_path: 'platform documents/Lumi' }))

  const defaults = await getProjectDefaults()

  assert.deepEqual(defaults, { parent_path: 'platform documents/Lumi' })
  assert.equal(globalThis.fetch.mock.calls[0].arguments[0], '/api/v1/project-defaults')
})

test('project API exposes global and project-scoped image preflights', async (t) => {
  t.mock.method(globalThis, 'fetch', async () => success({ output_size: { value: '1024x1024' } }))
  const pictureBook = { format: 'wordless_picture_book', aspect_ratio: { mode: 'square' } }
  await preflightImageGeneration(pictureBook)
  await preflightProjectImageGeneration('019-project')
  const calls = globalThis.fetch.mock.calls.map((call) => call.arguments)
  assert.equal(calls[0][0], '/api/v1/image-generation-preflights')
  assert.deepEqual(JSON.parse(calls[0][1].body), { picture_book: pictureBook })
  assert.equal(calls[1][0], '/api/v1/projects/019-project/image-generation-preflights')
  assert.equal(calls[1][1].method, 'POST')
})

test('project API relocates by public UUID and new root path', async (t) => {
  t.mock.method(globalThis, 'fetch', async () => success({ uuid: '019-recent' }))
  await relocateRecentProject({ uuid: '019-recent', rootPath: '/moved/moon' })
  const [path, options] = globalThis.fetch.mock.calls[0].arguments
  assert.equal(path, '/api/v1/recent-projects/019-recent')
  assert.equal(options.method, 'PATCH')
  assert.deepEqual(JSON.parse(options.body), { root_path: '/moved/moon' })
})
