import assert from 'node:assert/strict'
import test from 'node:test'

import { ApiError, apiRequest } from './client.js'
import {
  clearDesktopAuthenticationRequirement,
  desktopAuthenticationRequired,
} from '../desktopSession.js'

test('apiRequest returns envelope data', async (t) => {
  t.mock.method(globalThis, 'fetch', async () => new Response(JSON.stringify({
    success: true,
    data: { status: 'ok' },
  }), { status: 200, headers: { 'content-type': 'application/json' } }))

  assert.deepEqual(await apiRequest('/api/v1/health'), { status: 'ok' })
  assert.equal(globalThis.fetch.mock.calls[0].arguments[0], '/api/v1/health')
})

test('apiRequest throws structured API errors', async (t) => {
  t.mock.method(globalThis, 'fetch', async () => new Response(JSON.stringify({
    success: false,
    data: null,
    error: { code: 'not_found', message: 'Not Found', details: 'missing' },
  }), { status: 404, headers: { 'content-type': 'application/json' } }))

  await assert.rejects(
    apiRequest('/api/v1/missing'),
    (error) => error instanceof ApiError && error.status === 404 && error.code === 'not_found' && error.details === 'missing',
  )
})

test('apiRequest rejects invalid response bodies', async (t) => {
  t.mock.method(globalThis, 'fetch', async () => new Response('<html>bad gateway</html>', { status: 502 }))
  await assert.rejects(apiRequest('/api/v1/health'), { code: 'invalid_response', status: 502 })
})

test('apiRequest marks an expired desktop session', async (t) => {
  clearDesktopAuthenticationRequirement()
  t.after(clearDesktopAuthenticationRequirement)
  t.mock.method(globalThis, 'fetch', async () => new Response(JSON.stringify({
    success: false,
    data: null,
    error: {
      code: 'desktop_authentication_required',
      message: 'Desktop authentication required',
      details: 'Reopen Lumi',
    },
  }), { status: 401, headers: { 'content-type': 'application/json' } }))

  await assert.rejects(apiRequest('/api/v1/providers'), { code: 'desktop_authentication_required', status: 401 })
  assert.equal(desktopAuthenticationRequired(), true)
})

test('project_not_open recovery coalesces one open and safely retries each request once', async (t) => {
  const projectUuid = '01989abc-def0-7000-8000-000000000001'
  let openCalls = 0
  const resourceCalls = new Map()
  t.mock.method(globalThis, 'fetch', async (path) => {
    if (path === `/api/v1/open-projects/${projectUuid}`) {
      openCalls += 1
      await new Promise((resolve) => setTimeout(resolve, 5))
      return new Response(JSON.stringify({ success: true, data: { uuid: projectUuid, open: true } }), { status: 200 })
    }
    const count = (resourceCalls.get(path) || 0) + 1
    resourceCalls.set(path, count)
    if (count === 1) {
      return new Response(JSON.stringify({
        success: false,
        data: null,
        error: { code: 'project_not_open', message: 'not open', details: 'open first' },
      }), { status: 409 })
    }
    return new Response(JSON.stringify({ success: true, data: { path } }), { status: 200 })
  })

  const [story, thread] = await Promise.all([
    apiRequest(`/api/v1/projects/${projectUuid}/story-profile`),
    apiRequest(`/api/v1/projects/${projectUuid}/chat_threads/thread`),
  ])

  assert.equal(openCalls, 1)
  assert.equal(story.path, `/api/v1/projects/${projectUuid}/story-profile`)
  assert.equal(thread.path, `/api/v1/projects/${projectUuid}/chat_threads/thread`)
})
