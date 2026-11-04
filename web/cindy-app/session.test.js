import assert from 'node:assert/strict'
import test from 'node:test'
import { createCindySession } from './session.js'
import { themeCSS } from './theme.js'

const project = '01989abc-def0-7000-8000-000000000001'
const other = '01989abc-def0-7000-8000-000000000002'
const launch = (write = true) => ({ structuredContent: { success: true, data: { project_uuid: project, target_uuid: project, revision: 0, view: 'book', open_url: `http://127.0.0.1:3333/projects/${project}/`, request: { name: 'review_book', arguments: {} }, permissions: { can_write: write } } } })
test('theme mapping applies only to the Cindy build and preserves on-accent contrast and existing host variables', () => {
  const result = themeCSS('.cindy-app{color:#20231f;background:#fff;border-color:rgba(47,107,85,.24)}.button{color:#fff;background:#2f6b55}.notice{color:var(--text-primary,#20231f)}')
  assert.match(result, /color:var\(--text-primary,#20231f\)/)
  assert.match(result, /background:var\(--surface,#ffffff\)/)
  assert.match(result, /color:var\(--text-on-accent,#ffffff\)/)
  assert.match(result, /color-mix\(in srgb,var\(--accent,#2f6b55\) 24%,transparent\)/)
  assert.ok(result.endsWith('.notice{color:var(--text-primary,#20231f)}'))
  assert.match(themeCSS('.badge{background:#ffffffeb}'), /color-mix\(in srgb,var\(--surface,#ffffff\) 92\.16%,transparent\)/)
})
test('Cindy uses the shared REST API contract and never guesses a sender from page arguments', async () => {
  const calls = []
  const session = createCindySession(launch(), { call: async (...args) => { calls.push(args); return { success: true, data: { uuid: project } } } })
  assert.equal((await session.api.getStoryProject(project)).uuid, project)
  await session.api.updateStoryProject(project, { name: 'Saved', expected_revision: 2 })
  assert.equal(calls[0][0], 'read_app_resource')
  assert.deepEqual(calls[1][1], { resource: 'project', method: 'PATCH', body: { name: 'Saved', expected_revision: 2 } })
  await assert.rejects(session.api.getStoryProject(other), { code: 'app_project_mismatch' })
  await assert.rejects(session.bridge.call('call_tool', { name: 'project_init' }), { code: 'workspace_capability_unavailable' })
  await session.close()
})
test('missing permission, files, generation, export and downloads cannot claim a successful submission', async () => {
  let calls = 0
  const session = createCindySession(launch(false), { call: async () => { calls++; return { success: true, data: {} } } })
  await assert.rejects(session.api.updateStoryProject(project, { name: 'Denied', expected_revision: 0 }), { code: 'project_read_only' })
  await assert.rejects(session.api.createComicExport(project, { scope: 'book', format: 'pdf' }), { code: 'workspace_capability_unavailable' })
  await assert.rejects(session.api.generateSectionImage(project, other, other, {}), { code: 'workspace_capability_unavailable' })
  await assert.rejects(session.api.createAssetUpload(project, { file: new File(['x'], 'image.png') }), { code: 'workspace_capability_unavailable' })
  assert.equal(calls, 0)
  await session.close()
})
test('same-project task switches get independent queries/previews and discard a late result after close', async () => {
  let finish
  const first = createCindySession(launch(), { call: () => new Promise(resolve => { finish = resolve }) })
  const second = createCindySession(launch(), { call: async () => ({ success: true, data: { name: 'Second task' } }) })
  assert.notEqual(first.key, second.key)
  first.client.setQueryData(['story-project', project], { name: 'First task' })
  assert.equal(second.client.getQueryData(['story-project', project]), undefined)
  const pending = first.api.getStoryProject(project)
  await first.close()
  finish({ success: true, data: { name: 'Late first task' } })
  await assert.rejects(pending, { code: 'workspace_revoked' })
  assert.equal((await second.api.getStoryProject(project)).name, 'Second task')
  await second.close()
})
test('pagehide and component teardown share one close receipt and clear only their own session', async () => {
  let closes = 0
  const session = createCindySession(launch(), { call: async () => ({ success: true, data: {} }), close: async () => { closes++ } })
  const first = session.close(), second = session.close()
  assert.equal(first, second)
  await Promise.all([first, second])
  assert.equal(closes, 1)
  assert.equal(session.client.getQueryCache().getAll().length, 0)
})
