import assert from 'node:assert/strict'
import test from 'node:test'
import { createHostTransport, applyHostTheme } from './host.js'
import { createCindySession } from './session.js'
import { ModelContextAction } from '../mcp-app/model-context.js'

const project = '01989abc-def0-7000-8000-000000000001'
const page = '01989abc-def0-7000-8000-000000000002'
const envelope = { success: true, data: { uuid: project, title: 'Private complete text' } }
const launch = { structuredContent: { success: true, data: { project_uuid: project, target_uuid: project, revision: 0, view: 'book', open_url: `http://127.0.0.1:3333/projects/${project}/`, request: { name: 'review_book', arguments: {} }, permissions: { can_write: true } } } }
function sdk() {
  const calls = [], contexts = []
  return { calls, contexts, getHostCapabilities: () => ({ updateModelContext: { text: {}, structuredContent: {} }, openLinks: {} }),
    async callServerTool(request, options) { calls.push({ request, options }); return { _meta: { 'lumi/app_response': envelope } } },
    async updateModelContext(params, options) { contexts.push({ params, options }); return {} },
    async openLink() { return {} },
  }
}
test('official App calls pass the displayed UUID only as a constraint and require UI-only metadata', async () => {
  const app = sdk(), transport = createHostTransport(app)
  assert.deepEqual(await transport.call('read_app_resource', { resource: 'project' }, { project }), envelope)
  assert.deepEqual(app.calls[0].request, { name: 'read_app_resource', arguments: { resource: 'project', expected_project_uuid: project } })
  assert.equal('session_context' in app.calls[0].request.arguments, false)
  assert.ok(app.calls[0].options.signal instanceof AbortSignal)
  app.callServerTool = async () => ({ structuredContent: envelope })
  await assert.rejects(transport.call('read_app_resource', { resource: 'project' }, { project }), { code: 'app_meta_missing' })
  transport.close()
})
test('page selection uses standard updateModelContext without sending a message, adopting or saving', async () => {
  const app = sdk(), session = createCindySession(launch, createHostTransport(app)), action = new ModelContextAction(session.bridge)
  action.select({ kind: 'page', project_uuid: project, page_uuid: page, title: 'Identity page two', summary: 'Short page summary' })
  while (action.running) await new Promise(resolve => setImmediate(resolve))
  assert.equal(app.calls.length, 0, 'selection never calls a business tool')
  assert.equal(app.contexts.length, 1)
  assert.equal(app.contexts[0].params.structuredContent.page_uuid, page)
  assert.match(app.contexts[0].params.content[0].text, /Identity page two/)
  assert.equal(app.sendMessage, undefined)
  assert.match(action.getSnapshot().message, /宿主已接收引用/)
  await session.close()
  assert.equal(app.contexts[0].options.signal.aborted, true)
  await assert.rejects(session.bridge.updateModelContext({ page_uuid: page }), { code: 'workspace_capability_unavailable' })
})
test('uncertain SDK context timeout stops queued selections rather than overwriting the next context', async () => {
  const app = sdk(); let reject
  app.updateModelContext = params => { app.contexts.push(params); return new Promise((_, fail) => { reject = fail }) }
  const session = createCindySession(launch, createHostTransport(app)), action = new ModelContextAction(session.bridge)
  action.select({ project_uuid: project, page_uuid: page, title: 'First' })
  action.select({ project_uuid: project, page_uuid: project, title: 'Queued second' })
  reject(Object.assign(new Error('Request timed out'), { code: -32001 }))
  while (action.running) await new Promise(resolve => setImmediate(resolve))
  assert.equal(app.contexts.length, 1)
  assert.equal(action.getSnapshot().pending, false)
  assert.match(action.getSnapshot().message, /宿主响应超时/)
  await session.close()
})

test('Host dark/light variables map only within the Cindy page and preserve explicit legacy roles', () => {
  const values = new Map(), document = { documentElement: { style: { setProperty: (key, value) => values.set(key, value) } } }
  applyHostTheme({ theme: 'dark', styles: { variables: { '--color-text-primary': '#eeeeee', '--color-background-primary': '#111111', '--accent': '#abcdef' } } }, document)
  assert.equal(document.documentElement.style.colorScheme, 'dark')
  assert.equal(values.get('--text-primary'), 'var(--color-text-primary)')
  assert.equal(values.get('--surface'), 'var(--color-background-primary)')
  assert.equal(values.get('--accent'), '#abcdef')
})
