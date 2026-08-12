import assert from 'node:assert/strict'
import test from 'node:test'

import { rewriteAdminRequest } from './vite.config.js'

function rewrite(url, { method = 'GET', accept = 'text/html' } = {}) {
  const request = { url, method, headers: { accept } }
  let nextCalled = false
  rewriteAdminRequest(request, {}, () => { nextCalled = true })
  assert.equal(nextCalled, true)
  return request.url
}

test('admin HTML navigation falls back to the admin entry', () => {
  assert.equal(rewrite('/admin/settings?tab=general'), '/admin.html?tab=general')
  assert.equal(rewrite('/admin'), '/admin.html')
})

test('admin fallback ignores assets, non-HTML requests and writes', () => {
  assert.equal(rewrite('/admin/logo.svg'), '/admin/logo.svg')
  assert.equal(rewrite('/admin/settings', { accept: 'application/json' }), '/admin/settings')
  assert.equal(rewrite('/admin/settings', { method: 'POST' }), '/admin/settings')
})
