import test from 'node:test'
import assert from 'node:assert/strict'
import { QueryClient, QueryObserver } from '@tanstack/react-query'
import { AppBridge } from './bridge.js'
import { envelope, reviewContext, success, imageData, failureKind, acceptReviewResult } from './contracts.js'
import { PreviewCache, PreviewRequest } from './images.js'
import { bindReviewSync } from './sync.js'
import { AppRealtime } from './realtime.js'
const project = '01989abc-def0-7000-8000-000000000001'
const asset = '01989abc-def0-7000-8000-000000000002'
const wrap = (data) => ({ structuredContent: { success: true, data } })
const preview = (uuid = asset) => wrap({ asset_uuid: uuid, mime_type: 'image/jpeg', width: 256, height: 128, data_uri: 'data:image/jpeg;base64,/9j/2Q==' })
const context = () => ({ view: 'book', project_uuid: project, target_uuid: project, revision: 0, title: '星星', open_url: `http://127.0.0.1:1234/projects/${project}/chapters`, request: { name: 'review_book', arguments: {} }, items: [] })
const tick = () => new Promise((r) => setTimeout(r, 0))
function deferred() { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no }); return { resolve, reject, promise } }
function events() {
  const listeners = new Map()
  return { listeners, visibilityState: 'visible', addEventListener: (key, fn) => listeners.set(key, fn), removeEventListener: (key) => listeners.delete(key), emit: (key, value) => listeners.get(key)?.(value) }
}

test('production envelope and exactly one JSON text wrapper preserve complete context', () => {
  const d = context()
  for (const r of [wrap(d), { content: [{ type: 'text', text: JSON.stringify({ success: true, data: d }) }] }, { content: [{ type: 'text', text: JSON.stringify({ content: [], structuredContent: { success: true, data: d } }) }] }]) assert.deepEqual(reviewContext(r), d)
  for (const data of [{ ...d, view: 'guess' }, { ...d, revision: -1 }, { ...d, project_uuid: '1' }, { ...d, request: { name: 'review_page' } }, { ...d, open_url: 'https://evil.test' }]) assert.throws(() => reviewContext(wrap(data)))
  assert.throws(() => reviewContext({ content: [{ type: 'text', text: JSON.stringify({ content: [{ type: 'text', text: JSON.stringify(wrap(d)) }] }) }] }))
  assert.throws(() => success({ isError: true, ...wrap(d) }))
  assert.throws(() => success(wrap(null)))
  assert.throws(() => success({ structuredContent: { success: false, data: null, error: { code: 'asset_not_found', message: 'Missing', details: 'retry' } } }), (e) => e.code === 'asset_not_found' && e.kind === 'resource_error')
  assert.throws(() => envelope({ content: [{ type: 'text', text: 'not json' }] }))
  const badPage = { ...d, view: 'page', request: { name: 'review_page', arguments: { chapter_uuid: project, page_uuid: asset } }, items: [{ uuid: project, revision: 0, title: '', text: '', scene: '', kind: '', chapter_uuid: project, open_url: d.open_url }] }
  assert.throws(() => reviewContext(wrap(badPage)))
})
test('image uses nested production data, identity, dimensions and 120 KiB bound', () => {
  assert.equal(imageData(preview(), asset, 384).width, 256)
  assert.throws(() => imageData({ structuredContent: preview().structuredContent.data }, asset, 384))
  assert.throws(() => imageData(preview(project), asset, 384))
  assert.throws(() => imageData(preview(), asset, 128))
  assert.throws(() => imageData(wrap({ ...preview().structuredContent.data, data_uri: 'data:image/jpeg;base64,' + 'x'.repeat(163844) }), asset, 384))
  assert.equal(failureKind({ kind: 'timeout', message: 'rejected' }), 'timeout')
  assert.equal(failureKind({ code: 'permission_denied' }), 'host_denied')
  assert.equal(failureKind({ message: 'connection closed' }), 'host_error')
})
test('a fresh host review replaces existing query data even when the request key is unchanged', () => {
  const queryClient = new QueryClient()
  const first = context()
  acceptReviewResult(queryClient, wrap(first))
  const latest = { ...first, title: '最新项目标题', revision: 2 }
  acceptReviewResult(queryClient, { content: [{ type: 'text', text: JSON.stringify(wrap(latest)) }] })
  assert.equal(queryClient.getQueryData(['app-review', project, first.request]).title, '最新项目标题')
  assert.throws(() => acceptReviewResult(queryClient, wrap({ ...latest, target_uuid: 'bad' })))
  assert.equal(queryClient.getQueryData(['app-review', project, first.request]).revision, 2)
  queryClient.clear()
})
test('switching away cancels queued previews before they reach the host', async () => {
  const first = deferred(); let calls = 0
  const cache = new PreviewCache(() => { calls++; return first.promise }, { concurrency: 1 })
  const active = cache.load(project, asset, 384)
  const view = new PreviewRequest(cache, () => {})
  view.show(project, project, 384, true)
  assert.equal(cache.queue.length, 1)
  view.cancel(); assert.equal(cache.queue.length, 0)
  first.resolve(preview()); await active; await tick(); assert.equal(calls, 1)
})
test('bridge verifies parent source, distinguishes denial/timeout and ignores late responses', async () => {
  const surface = events(); const sent = []; surface.parent = { postMessage: (m) => sent.push(m) }
  const bridge = new AppBridge(surface, 20)
  const first = bridge.call('read_app_image', { asset_uuid: asset })
  surface.emit('message', { source: {}, data: { jsonrpc: '2.0', id: 1, result: preview() } })
  assert.equal(bridge.pending.size, 1)
  surface.emit('message', { source: surface.parent, data: { jsonrpc: '2.0', id: 1, error: { code: 'user_rejected', message: 'Rejected' } } })
  await assert.rejects(first, (e) => e.kind === 'host_denied')
  await assert.rejects(bridge.call('read_app_image', {}), (e) => e.kind === 'timeout')
  surface.emit('message', { source: surface.parent, data: { jsonrpc: '2.0', id: 2, result: preview() } })
  assert.equal(bridge.pending.size, 0); assert.equal(sent.filter(m => m.method === 'tools/call').length, 2); bridge.dispose()
})
test('visible-only requests, three in flight, deduplication, bounded cache and identity keys', async () => {
  const jobs = []; const cache = new PreviewCache((_name, args) => { const d = deferred(); jobs.push({ ...d, args }); return d.promise }, { capacity: 2 })
  const states = []; const consumer = new PreviewRequest(cache, (p) => states.push(p))
  consumer.show(project, asset, 384, false); await tick(); assert.equal(jobs.length, 0)
  consumer.show(project, undefined, 384, true); assert.equal(states.at(-1).state, 'missing')
  const loads = [asset, project, '01989abc-def0-7000-8000-000000000003', '01989abc-def0-7000-8000-000000000004'].map((uuid) => cache.load(project, uuid, 384))
  assert.equal(cache.load(project, asset, 384), loads[0]); await tick(); assert.equal(jobs.length, 3)
  jobs[0].resolve(preview(jobs[0].args.asset_uuid)); await tick(); assert.equal(jobs.length, 4)
  for (const job of jobs.slice(1)) job.resolve(preview(job.args.asset_uuid)); await Promise.all(loads); await tick()
  assert.equal(cache.cache.size, 2)
  const differentSize = cache.load(project, asset, 1024); await tick(); assert.equal(jobs.length, 5); jobs[4].resolve(preview()); await differentSize; await tick()
  const otherProject = cache.load(asset, asset, 1024); await tick(); assert.equal(jobs.length, 6); jobs[5].resolve(preview()); await otherProject
})
test('single failure remains local, does not retry, and obsolete page response is discarded', async () => {
  const old = deferred(); const latest = deferred(); const states = []; let calls = 0
  const cache = new PreviewCache(() => (++calls === 1 ? old.promise : latest.promise))
  const view = new PreviewRequest(cache, (s) => states.push(s))
  view.show(project, asset, 384, true); view.show(project, project, 384, true); await tick()
  latest.resolve(preview(project)); await tick(); assert.equal(states.at(-1).data.asset_uuid, project)
  old.reject(Object.assign(new Error('missing'), { kind: 'resource_error' })); await tick()
  assert.equal(states.at(-1).data.asset_uuid, project); assert.equal(calls, 2)
  const failure = new PreviewRequest(new PreviewCache(async () => { throw { kind: 'timeout' } }), (s) => states.push(s))
  failure.show(project, asset, 384, true); await tick(); assert.equal(states.at(-1).error.kind, 'timeout')
  await tick(); assert.equal(calls, 2)
})
test('WS join/rejoin/hint/focus/visibility invalidate TanStack Query and reread through REST bridge', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  let reads = 0
  const observer = new QueryObserver(client, { queryKey: ['app-review', project, 'book'], queryFn: async () => ++reads })
  const unsubscribe = observer.subscribe(() => {}); await tick(); assert.equal(reads, 1)
  const callbacks = {}; let message; let left = false; let disconnected = false
  const channel = { on: (key, fn) => { callbacks[key] = fn; return () => delete callbacks[key] }, onMessage: (fn) => { message = fn; return () => { message = null } }, join() {}, leave() { left = true } }
  const surface = events(); const doc = events()
  const stop = bindReviewSync({ socket: { channel: (topic) => { assert.equal(topic, `project:${project}`); return channel }, disconnect: () => { disconnected = true } }, project, queryClient: client, surface, documentSurface: doc })
  for (const action of [() => callbacks.phx_joined(), () => message('comic:section_changed', {}), () => callbacks.phx_joined(), () => surface.emit('focus'), () => doc.emit('visibilitychange')]) { const before = reads; action(); await tick(); assert.equal(reads, before + 1) }
  const before = reads; await new Promise((r) => setTimeout(r, 30)); assert.equal(reads, before)
  stop(); assert.ok(left && disconnected); assert.equal(surface.listeners.size, 0); unsubscribe(); client.clear()
})
test('focus still reconciles when the host connection callback is unavailable', async () => {
  const client = new QueryClient(); const surface = events(); const doc = events(); let reads = 0
  const observer = new QueryObserver(client, { queryKey: ['app-review', project], queryFn: async () => ++reads, staleTime: Infinity })
  const off = observer.subscribe(() => {}); await tick()
  const stop = bindReviewSync({ project, queryClient: client, surface, documentSurface: doc })
  surface.emit('focus'); await tick(); assert.equal(reads, 2)
  stop(); off(); client.clear()
})

test('blocking tool results relay real WS hints without timed state reads and explicitly release subscriptions', async () => {
 const surface = events(); const sent = []; surface.parent = { postMessage: m => sent.push(m) }
 const bridge = new AppBridge(surface); const client = new QueryClient(); let reads = 0
 const observer = new QueryObserver(client, { queryKey: ['app-review', project], queryFn: async () => ++reads, staleTime: Infinity })
 const off = observer.subscribe(() => {}); await tick()
 const socket = new AppRealtime(bridge, project, { startupTimeout: 200, retryDelays: [1] })
 const stop = bindReviewSync({ socket, project, queryClient: client, surface, documentSurface: events() })
 const calls = action => sent.filter(m => m.method === 'tools/call' && m.params.arguments.action === action)
 const send = (request, data) => surface.emit('message', { source: surface.parent, data: { jsonrpc: '2.0', id: request.id, result: { structuredContent: { success: true, data } } } })
 const subscription = '01989abc-def0-7000-8000-000000000099'
 const value = (state, cursor, uuid = subscription) => ({ project_uuid: project, subscription_uuid: uuid, state, cursor, ...(state === 'changed' ? { event: 'comic:section_changed' } : {}) })
 send(calls('open')[0], value('joined','0')); await tick(); assert.equal(reads, 2)
 assert.equal(calls('next')[0].params.arguments.cursor, '0')
 send(calls('next').at(-1), value('changed','1')); await tick(); assert.equal(reads, 3)
 send(calls('next').at(-1), value('heartbeat','2')); await tick(); assert.equal(reads, 3)
 assert.equal(calls('next').at(-1).params.arguments.cursor, '2')
 await new Promise(r => setTimeout(r, 25)); assert.equal(calls('next').length, 3)
 const waiting = calls('next').at(-1)
 surface.emit('message', { source: surface.parent, data: { jsonrpc: '2.0', id: waiting.id, result: { isError: true, structuredContent: { success: false, data: null, error: { code: 'app_watch_disconnected', message: 'closed' } } } } })
 await new Promise(r => setTimeout(r, 10)); assert.equal(calls('open').length, 2)
 send(calls('close')[0], {}); send(calls('open')[1], value('joined','0')); await tick(); assert.equal(reads, 4)
 surface.emit('focus'); await tick(); assert.equal(reads, 5)
 const lastWait = calls('next').at(-1)
 stop(); send(calls('close').at(-1), {}); await tick()
 assert.equal(calls('close').length, 2); assert.equal(bridge.pending.size, 0)
 assert.ok(sent.some(m => m.method === 'notifications/cancelled' && m.params.requestId === lastWait.id))
 send(lastWait, value('changed','3')); await tick(); assert.equal(reads, 5)
 off(); client.clear(); bridge.dispose()
})
test('a stalled or rejecting host stops once without periodic state queries', async () => {
 const surface = events(); const sent = []; surface.parent = { postMessage: m => sent.push(m) }
 const bridge = new AppBridge(surface); const socket = new AppRealtime(bridge, project, { startupTimeout: 5, retryDelays: [1] })
 socket.channel(`project:${project}`).join()
 await new Promise(r => setTimeout(r, 30))
 assert.equal(sent.filter(m => m.method === 'tools/call').length, 1)
 assert.equal(bridge.pending.size, 0); socket.disconnect(); bridge.dispose()
})
test('invalid event identities/cursors and host rejection stop without reconnecting or applying changes', async () => {
 const subscription = '01989abc-def0-7000-8000-000000000099'
 for (const invalid of [
  { project_uuid: asset }, { subscription_uuid: asset }, { cursor: '0' }, { state: 'joined' }, null,
 ]) {
  const surface = events(); const sent = []; surface.parent = { postMessage: m => sent.push(m) }
  const bridge = new AppBridge(surface); const socket = new AppRealtime(bridge, project, { retryDelays: [1] }); let changes = 0
  const channel = socket.channel(`project:${project}`); channel.onMessage(() => changes++); channel.join()
  const send = (id, rest) => surface.emit('message', { source: surface.parent, data: { jsonrpc: '2.0', id, ...rest } })
  const base = { project_uuid: project, subscription_uuid: subscription, cursor: '0', state: 'joined' }
  send(sent[0].id, { result: wrap(base) }); await tick()
  const next = sent.find(m => m.params?.arguments?.action === 'next')
  send(next.id, invalid ? { result: wrap({ ...base, state: 'changed', cursor: '1', event: 'comic:section_changed', ...invalid }) } : { error: { code: -32000, message: 'permission denied' } })
  await new Promise(r => setTimeout(r, 10))
  assert.equal(changes, 0)
  assert.equal(sent.filter(m => m.params?.arguments?.action === 'open').length, 1)
  assert.equal(sent.filter(m => m.params?.arguments?.action === 'next').length, 1)
  socket.disconnect(); bridge.dispose()
 }
})
test('host teardown requests are handled even when their id equals a pending view request', async () => {
  const surface = events(); const sent = []; surface.parent = { postMessage: m => sent.push(m) }
  const bridge = new AppBridge(surface)
  const pending = bridge.request('tools/call', { name: 'watch_app_changes', arguments: {} }, { timeoutMs: 0 })
  bridge.onNotification(method => { if (method === 'ui/resource-teardown') bridge.dispose() })
  surface.emit('message', { source: surface.parent, data: { jsonrpc: '2.0', id: 1, method: 'ui/resource-teardown', params: {} } })
  await assert.rejects(pending, e => e.kind === 'cancelled')
  assert.deepEqual(sent.at(-1), { jsonrpc: '2.0', id: 1, result: {} })
})
