import assert from 'node:assert/strict'
import test from 'node:test'

import {
  bootstrapDesktopSession,
  clearDesktopAuthenticationRequirement,
  desktopAuthenticationRequired,
  requireDesktopAuthentication,
  subscribeToDesktopAuthentication,
} from './desktopSession.js'

test('desktop bootstrap removes the fragment before exchanging the token', async () => {
  const calls = []
  const result = await bootstrapDesktopSession({
    location: {
      hash: '#desktop_token=runtime-token',
      pathname: '/admin/settings',
      search: '?tab=runtime',
    },
    history: {
      state: { navigation: true },
      replaceState(state, title, url) {
        calls.push({ kind: 'history', state, title, url })
      },
    },
    async fetchImpl(path, options) {
      calls.push({ kind: 'fetch', path, options })
      return new Response(JSON.stringify({ success: true, data: null }), {
        status: 201,
        headers: { 'content-type': 'application/json' },
      })
    },
  })

  assert.deepEqual(result, { ok: true, attempted: true })
  assert.equal(calls[0].kind, 'history')
  assert.equal(calls[0].url, '/admin/settings?tab=runtime')
  assert.equal(calls[1].kind, 'fetch')
  assert.equal(calls[1].path, '/api/v1/desktop-sessions')
  assert.equal(calls[1].options.credentials, 'same-origin')
  assert.deepEqual(JSON.parse(calls[1].options.body), { token: 'runtime-token' })
})

test('desktop bootstrap leaves ordinary web loads unchanged', async () => {
  let fetched = false
  const result = await bootstrapDesktopSession({
    location: { hash: '', pathname: '/', search: '' },
    history: { replaceState() { throw new Error('unexpected history change') } },
    fetchImpl: async () => { fetched = true },
  })

  assert.deepEqual(result, { ok: true, attempted: false })
  assert.equal(fetched, false)
})

test('desktop bootstrap fails closed for an invalid token', async () => {
  const result = await bootstrapDesktopSession({
    location: { hash: '#desktop_token=invalid', pathname: '/', search: '' },
    history: { state: null, replaceState() {} },
    fetchImpl: async () => new Response(JSON.stringify({
      success: false,
      data: null,
      error: { code: 'desktop_authentication_failed' },
    }), { status: 401, headers: { 'content-type': 'application/json' } }),
  })

  assert.deepEqual(result, { ok: false, attempted: true })
})

test('desktop authentication requirement notifies subscribers once per transition', () => {
  clearDesktopAuthenticationRequirement()
  let notifications = 0
  const unsubscribe = subscribeToDesktopAuthentication(() => { notifications += 1 })

  requireDesktopAuthentication()
  requireDesktopAuthentication()
  assert.equal(desktopAuthenticationRequired(), true)
  assert.equal(notifications, 1)

  clearDesktopAuthenticationRequirement()
  assert.equal(desktopAuthenticationRequired(), false)
  assert.equal(notifications, 2)
  unsubscribe()
})
