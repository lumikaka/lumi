import test from 'node:test'
import assert from 'node:assert/strict'
import { AppBridge } from './bridge.js'
import { ModelContextAction, modelContextParams } from './model-context.js'
const init = hostCapabilities => ({ hostCapabilities })
const context = { project_uuid: '01989abc-def0-7000-8000-000000000001', resource_type: 'text_candidate', number: 2, title: '候选二', text: '完整正文' }
const tick = () => new Promise(resolve => setTimeout(resolve, 0))
const deferred = () => { let resolve, reject; const promise = new Promise((yes,no) => { resolve=yes; reject=no }); return { promise, resolve, reject } }

test('uses only declared standard text/structured capabilities; no experimental fallback', () => {
  for (const value of [undefined, init({}), init({ experimental: { 'openai/modelContext': {} } }), init({updateModelContext:{image:{}}})]) assert.throws(() => modelContextParams(value, context))
  const text = modelContextParams(init({updateModelContext:{text:{}}}), context)
  assert.equal(text.structuredContent, undefined)
  assert.ok(text.content[0].text.includes('完整正文'))
  const structured = modelContextParams(init({updateModelContext:{structuredContent:{}}}), context)
  assert.equal(structured.content, undefined)
  assert.deepEqual(structured.structuredContent, context)
})

test('AppBridge sends standard context request without message/tools calls, validates acknowledgement and denial', async () => {
  let receive; const sent=[]
  const surface = { parent:{postMessage: m => sent.push(m)}, addEventListener:(_,fn)=>receive=fn, removeEventListener:()=>{} }
  const bridge = new AppBridge(surface)
  bridge.initialization = init({updateModelContext:{text:{},structuredContent:{}}})
  for (const [response, outcome] of [[{result:{}}, 'ok'], [{result:{isError:true}}, 'host_failed'], [{result:null}, 'protocol_error'], [{result:{isError:'true'}}, 'protocol_error'], [{error:{code:'permission_denied',message:'拒绝'}}, 'host_denied']]) {
    const pending = bridge.updateModelContext(context)
    const request = sent.at(-1)
    assert.equal(request.method, 'ui/update-model-context')
    assert.deepEqual(request.params.structuredContent, context)
    receive({source:surface.parent,data:{jsonrpc:'2.0',id:request.id,...response}})
    if (outcome === 'ok') await pending
    else await assert.rejects(pending, error=>error.kind===outcome)
  }
  assert.ok(sent.every(m=>m.method==='ui/update-model-context'))
  bridge.dispose()
})

test('quick selections serialize globally, coalesce intermediate clicks and report only latest acknowledgement', async () => {
  const calls=[]
  const action = new ModelContextAction({ updateModelContext: value => { const d=deferred(); calls.push({...d,value}); return d.promise } })
  action.select({...context,title:'A'})
  action.select({...context,title:'B'})
  action.select({...context,title:'C'})
  assert.equal(calls.length,1)
  calls[0].resolve({}); await tick()
  assert.equal(calls.length,2)
  assert.equal(calls[1].value.title,'C')
  assert.equal(action.state.pending,true)
  assert.ok(!action.state.message.includes('已接收'))
  calls[1].resolve({}); await tick()
  assert.match(action.state.message,/已接收引用：C/)
})

test('rejected latest selection never reports previous success; timeout stops queued updates', async () => {
  for (const kind of ['host_denied','unsupported','timeout','protocol_error','transport_error']) {
    const d=deferred(); let calls=0
    const action=new ModelContextAction({updateModelContext:()=>{calls++; return d.promise}})
    action.select(context)
    if(kind==='timeout') action.select({...context,title:'new'})
    d.reject(Object.assign(new Error('test failure'),{kind})); await tick()
    assert.equal(action.state.pending,false)
    assert.match(action.state.message,/尚未确认最新选择已同步/)
    assert.equal(calls,1)
  }
})
