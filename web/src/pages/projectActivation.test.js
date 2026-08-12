import assert from 'node:assert/strict'
import test from 'node:test'

import { ensureProjectOpen } from './projectActivation.js'

function success(data) {
  return new Response(JSON.stringify({ success: true, data }), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  })
}

test('project route idempotently opens its own project', async (t) => {
  const project = { uuid: '01989abc-def0-7000-8000-000000000001' }
  t.mock.method(globalThis, 'fetch', async () => success(project))
	assert.deepEqual(await ensureProjectOpen(project.uuid), project)
  assert.equal(globalThis.fetch.mock.callCount(), 1)
	const [path, options] = globalThis.fetch.mock.calls[0].arguments
	assert.equal(path, `/api/v1/open-projects/${project.uuid}`)
	assert.equal(options.method, 'PUT')
})
