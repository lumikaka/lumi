import assert from 'node:assert/strict'
import test from 'node:test'

import { RealtimeSocket, getRealtimeSocket, resetRealtimeSocket } from './realtime.js'

class FakeWebSocket {
  constructor(url) {
    this.url = url
    this.readyState = 0
    this.listeners = new Map()
    this.sent = []
  }

  addEventListener(event, handler) {
    const handlers = this.listeners.get(event) || []
    handlers.push(handler)
    this.listeners.set(event, handlers)
  }

  emit(event, payload = {}) {
    for (const handler of this.listeners.get(event) || []) handler(payload)
  }

  open() {
    this.readyState = 1
    this.emit('open')
  }

  send(message) {
    this.sent.push(JSON.parse(message))
  }

  serverFrame(frame) {
    this.emit('message', { data: JSON.stringify(frame) })
  }

  close() {
    if (this.readyState === 3) return
    this.readyState = 3
    this.emit('close')
  }
}

function harness(options = {}) {
  const connections = []
  const socket = new RealtimeSocket('ws://test/api/v1/ws', {
    heartbeatInterval: 60_000,
    joinTimeout: 50,
    reconnectDelays: [0],
    webSocketFactory: (url) => {
      const connection = new FakeWebSocket(url)
      connections.push(connection)
      return connection
    },
    ...options,
  })
  return { socket, connections }
}

function reply(connection, frame, status = 'ok', response = {}) {
  connection.serverFrame({
    topic: frame.topic,
    event: 'phx_reply',
    payload: { status, response },
    ref: frame.ref,
    join_ref: frame.join_ref,
  })
}

test('application realtime socket is a singleton', () => {
  resetRealtimeSocket()
  assert.equal(getRealtimeSocket(), getRealtimeSocket())
  resetRealtimeSocket()
})

test('system subscribers share one join and receive broadcasts', () => {
  const { socket, connections } = harness()
  const first = socket.channel('system')
  const second = socket.channel('system')
  const received = []
  first.on('system:status_changed', (payload) => received.push(['first', payload.status]))
  second.on('system:status_changed', (payload) => received.push(['second', payload.status]))
  first.join()
  second.join()

  connections[0].open()
  const join = connections[0].sent.find((frame) => frame.event === 'phx_join')
  reply(connections[0], join)
  connections[0].serverFrame({
    topic: 'system',
    event: 'system:status_changed',
    payload: { status: 'ok' },
    ref: null,
    join_ref: join.join_ref,
  })
  assert.deepEqual(received, [['first', 'ok'], ['second', 'ok']])

  first.leave()
  assert.equal(connections[0].sent.filter((frame) => frame.event === 'phx_leave').length, 0)
  second.leave()
  assert.equal(connections[0].sent.filter((frame) => frame.event === 'phx_leave').length, 1)
  socket.disconnect()
})

test('active channel reconnects and rejoins with a new join_ref', async () => {
  const { socket, connections } = harness()
  const channel = socket.channel('system')
  let reconnected = 0
  channel.on('phx_reconnected', () => { reconnected += 1 })
  channel.join()
  connections[0].open()
  const firstJoin = connections[0].sent.find((frame) => frame.event === 'phx_join')
  reply(connections[0], firstJoin)

  connections[0].close()
  await waitFor(() => connections.length === 2)
  connections[1].open()
  const secondJoin = connections[1].sent.find((frame) => frame.event === 'phx_join')
  assert.notEqual(secondJoin.join_ref, firstJoin.join_ref)
  reply(connections[1], secondJoin)
  assert.equal(reconnected, 1)
  socket.disconnect()
})

test('an active project channel can retry its join after the project is reopened', () => {
  const { socket, connections } = harness()
  const channel = socket.channel('project:01989abc-def0-7000-8000-000000000001')
  channel.on('phx_join_error', (error) => {
    if (error.reason === 'project_not_open') channel.join()
  })
  channel.join()
  connections[0].open()
  const firstJoin = connections[0].sent.find((frame) => frame.event === 'phx_join')
  reply(connections[0], firstJoin, 'error', { reason: 'project_not_open' })
  const joins = connections[0].sent.filter((frame) => frame.event === 'phx_join')
  assert.equal(joins.length, 2)
  assert.notEqual(joins[1].join_ref, firstJoin.join_ref)
  reply(connections[0], joins[1])
  channel.leave()
  socket.disconnect()
})

async function waitFor(condition) {
  const deadline = Date.now() + 500
  while (Date.now() < deadline) {
    if (condition()) return
    await new Promise((resolve) => setTimeout(resolve, 2))
  }
  assert.fail('condition was not satisfied')
}
