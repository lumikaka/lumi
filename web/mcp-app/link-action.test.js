import test from 'node:test'
import assert from 'node:assert/strict'
import { AppBridge } from './bridge.js'
import { LinkAction, downloadURL } from './link-action.js'

const project = '01989abc-def0-7000-8000-000000000001'
const url = `http://127.0.0.1:49123/projects/${project}/chapters?panel=candidates`
function fixture() {
  const sent = []; let receive
  const surface = { parent: { postMessage: message => sent.push(message) }, addEventListener: (_type, fn) => { receive = fn }, removeEventListener() {} }
  const bridge = new AppBridge(surface)
  bridge.initialization = { protocolVersion: '2026-01-26', hostInfo: { name: 'Test host' }, hostCapabilities: { openLinks: {} } }
  const action = new LinkAction(bridge, url)
  const respond = body => receive({ source: surface.parent, data: { jsonrpc: '2.0', id: sent.findLast(message => message.method === 'ui/open-link').id, ...body } })
  return { bridge, action, sent, respond, surface }
}

test('standard empty result acknowledges without claiming a window opened; exact wire response is retained', async () => {
  for (const result of [{}, { isError: false }, { vendor_extension: 'preserved' }]) {
    const { bridge, action, sent, respond } = fixture()
    const opening = action.open()
    assert.equal(action.state.opening, true)
    assert.match(action.state.openMessage, /正在请求打开/)
    assert.deepEqual(sent[0], { jsonrpc: '2.0', id: 1, method: 'ui/open-link', params: { url } })
    respond({ result }); await opening
    assert.equal(action.state.opening, false)
    assert.match(action.state.openMessage, /^已请求打开，无法确认/)
    assert.doesNotMatch(action.state.openMessage, /已成功|桌面客户端/)
    assert.deepEqual(action.state.diagnostics.exchanges.find(event => event.stage === 'response').response, { jsonrpc: '2.0', id: 1, result })
    bridge.dispose()
  }
})

test('rapid repeated clicks issue one request and do not replay after acknowledgement', async () => {
  const { bridge, action, sent, respond } = fixture()
  const opening = action.open()
  await Promise.all([action.open(), action.open()])
  assert.equal(sent.length, 1)
  respond({ result: {} }); await opening
  assert.equal(sent.length, 1)
  const retry = action.open(); assert.equal(sent.length, 2)
  respond({ result: {} }); await retry
  bridge.dispose()
})

test('explicit isError failure, JSON-RPC denial, unsupported method and invalid results remain distinct', async () => {
  for (const [body, kind, message] of [
    [{ result: { isError: true } }, 'host_failed', /未能打开.*未提供具体原因/],
    [{ error: { code: -32000, message: 'Policy denied this URL' } }, 'host_denied', /宿主拒绝.*Policy denied/],
    [{ error: { code: -32000, message: 'Access Denied' } }, 'host_denied', /宿主拒绝.*Access Denied/],
    [{ error: { code: -32700, message: 'Parse error' } }, 'protocol_error', /协议不兼容/],
    [{ error: { code: -32600, message: 'Invalid Request' } }, 'protocol_error', /协议不兼容/],
    [{ error: { code: -32601, message: 'Method not found' } }, 'protocol_error', /协议不兼容.*Method not found/],
    [{ error: { code: -32602, message: 'Invalid params' } }, 'protocol_error', /协议不兼容/],
    [{ error: { code: -32603, message: 'Internal error' } }, 'host_error', /宿主请求异常.*Internal error/],
    [{ result: null }, 'protocol_error', /结果格式无效/],
    [{ result: { isError: 'false' } }, 'protocol_error', /结果格式无效/],
    [{ result: {}, error: { code: -32000, message: 'Invalid envelope' } }, 'protocol_error', /JSON-RPC/],
    [{}, 'protocol_error', /JSON-RPC/],
  ]) {
    const { bridge, action, respond } = fixture()
    const opening = action.open(); respond(body); await opening
    assert.equal(action.state.diagnostics.failure.kind, kind)
    assert.match(action.state.openMessage, message)
    assert.equal(action.state.opening, false)
    assert.equal(bridge.pending.size, 0)
    bridge.dispose()
  }
})

test('timeout releases the button, ignores late response and does not send another open request', async t => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const { bridge, action, sent, respond } = fixture()
  const opening = action.open()
  t.mock.timers.tick(10000); await opening
  assert.match(action.state.openMessage, /响应超时，无法确认/)
  assert.equal(action.state.diagnostics.failure.kind, 'timeout')
  assert.equal(action.state.opening, false)
  respond({ result: {} })
  assert.equal(action.state.diagnostics.failure.kind, 'timeout')
  assert.equal(sent.filter(message => message.method === 'ui/open-link').length, 1)
  assert.equal(bridge.pending.size, 0)
  bridge.dispose()
})

test('dispatch exception cleans pending state immediately, without waiting for a second timeout', async () => {
  const { bridge, action, surface } = fixture()
  surface.parent.postMessage = () => { throw new Error('DataCloneError') }
  await action.open()
  assert.match(action.state.openMessage, /请求未能发出.*DataCloneError/)
  assert.equal(action.state.diagnostics.failure.kind, 'transport_error')
  assert.equal(bridge.pending.size, 0)
  bridge.dispose()
})

test('absent host capability or initialization does not dispatch; copy still works', async () => {
  const { bridge, action, sent } = fixture()
  for (const initialization of [{ hostCapabilities: {} }, undefined]) {
    bridge.initialization = initialization
    await action.open()
    assert.match(action.state.openMessage, /未声明|尚未完成/)
  }
  assert.equal(sent.length, 0)
  let copied
  action.clipboard = { writeText: async text => { copied = text } }
  await action.copy()
  assert.equal(copied, url)
  assert.match(action.state.copyMessage, /链接已复制/)
  bridge.dispose()
})

test('copy remains usable during open, deduplicates pending copies, reports success and preserves the exact URL', async () => {
  const { bridge, action, respond } = fixture()
  let resolve, copies = 0, copied
  action.clipboard = { writeText: text => { copies++; copied = text; return new Promise(done => { resolve = done }) } }
  const opening = action.open(), copying = action.copy()
  await action.copy()
  assert.equal(copies, 1); assert.equal(copied, url)
  assert.equal(action.state.copying, true)
  resolve(); await copying
  assert.match(action.state.copyMessage, /已复制/)
  assert.equal(action.state.opening, true)
  respond({ result: {} }); await opening; bridge.dispose()
})

test('unavailable, rejected and throwing clipboard expose full selectable URL without affecting opening state', async () => {
  for (const clipboard of [undefined, { writeText: async () => { throw new Error('NotAllowedError') } }, { writeText: () => { throw new Error('Permission denied') } }]) {
    const { bridge, action } = fixture()
    action.clipboard = clipboard
    await action.copy()
    assert.equal(action.state.manualCopy, true)
    assert.equal(action.url, url)
    assert.match(action.state.copyMessage, /复制失败.*手动复制/)
    assert.equal(action.state.copying, false)
    bridge.dispose()
  }
})

test('stalled clipboard falls back to manual copy and cannot leave the copy button disabled', async t => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const { bridge, action } = fixture()
  action.clipboard = { writeText: () => new Promise(() => {}) }
  const copying = action.copy()
  t.mock.timers.tick(5000); await copying
  assert.equal(action.state.manualCopy, true)
  assert.equal(action.state.copying, false)
  bridge.dispose()
})

test('download URL preserves current service port and exact project export boundary', () => {
  const path = `/media/projects/${project}/comic-exports/export.pdf`
  assert.equal(downloadURL('http://127.0.0.1:49234', path, project), `http://127.0.0.1:49234${path}`)
  for (const href of [undefined, '', 'https://example.com/a', '//evil.test/a', `${path}/../../../../other`, path.replace(project, 'other'), path.replace('export.pdf', '%2e%2e/secret'), path.replace('export.pdf', '\\secret'), path.replace('export.pdf', ''), `${path}\n`]) {
    assert.equal(downloadURL('http://127.0.0.1:49234', href, project), null, String(href))
  }
  assert.equal(downloadURL('https://example.com', path, project), null)
})

test('initialize stores the negotiated host capabilities before the initialized notification', async () => {
  const { bridge, surface, sent } = fixture()
  let onMessage
  bridge.dispose()
  surface.addEventListener = (_type, fn) => { onMessage = fn }
  const fresh = new AppBridge(surface)
  const initialized = fresh.initialize()
  const result = { protocolVersion: '2026-01-26', hostCapabilities: { openLinks: {} } }
  onMessage({ source: surface.parent, data: { jsonrpc: '2.0', id: 1, result } })
  await initialized
  assert.deepEqual(fresh.initialization, result)
  assert.equal(sent.at(-1).method, 'ui/notifications/initialized')
  fresh.dispose()
})
