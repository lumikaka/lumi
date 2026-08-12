const DEFAULT_JOIN_TIMEOUT = 8_000
const DEFAULT_HEARTBEAT_INTERVAL = 25_000
const DEFAULT_RECONNECT_DELAYS = [250, 500, 1_000, 2_000, 5_000]

class Push {
  constructor() {
    this.callbacks = new Map()
    this.result = null
    this.timer = null
  }

  receive(status, callback) {
    if (typeof callback !== 'function') return this
    const callbacks = this.callbacks.get(status) || new Set()
    callbacks.add(callback)
    this.callbacks.set(status, callbacks)
    if (this.result?.status === status) defer(() => callback(this.result.payload))
    return this
  }

  startTimeout(timeoutMs, onTimeout) {
    if (this.result || this.timer) return
    this.timer = setTimeout(() => {
      this.timer = null
      if (this.result) return
      this.trigger('timeout', { reason: 'timeout' })
      onTimeout?.()
    }, timeoutMs)
  }

  trigger(status, payload) {
    if (this.result) return
    if (this.timer) clearTimeout(this.timer)
    this.timer = null
    this.result = { status, payload }
    Array.from(this.callbacks.get(status) || []).forEach((callback) => callback(payload))
  }
}

class TopicChannel {
  constructor(socket, topic, params) {
    this.socket = socket
    this.topic = topic
    this.params = params
    this.handlers = new Map()
    this.refCount = 0
    this.status = 'closed'
    this.joinPush = null
    this.joinRef = null
    this.everJoined = false
    this.rejoining = false
  }

  lease() {
    return new ChannelLease(this)
  }

	retain() {
    this.refCount += 1
    if (this.status === 'joined') return settledPush('ok', {})
    if (this.joinPush && !this.joinPush.result) return this.joinPush

    this.joinPush = new Push()
    this.status = this.socket.isOpen() ? 'joining' : 'connecting'
    this.emit('phx_status', { status: this.status })
    this.socket.connect()
    if (this.socket.isOpen()) this.sendJoin(this.joinPush)
    return this.joinPush
	}

	retryJoin() {
		if (this.refCount === 0) return settledPush('error', { reason: 'not_joining' })
		if (this.status === 'joined') return settledPush('ok', {})
		if (this.joinPush && !this.joinPush.result) return this.joinPush
		this.joinPush = new Push()
		this.status = this.socket.isOpen() ? 'joining' : 'connecting'
		this.emit('phx_status', { status: this.status })
		this.socket.connect()
		if (this.socket.isOpen()) this.sendJoin(this.joinPush)
		return this.joinPush
	}

  release() {
    this.refCount = Math.max(0, this.refCount - 1)
    if (this.refCount > 0) return

    if (this.status === 'joined' && this.socket.isOpen()) {
      const ref = this.socket.makeRef()
      this.socket.send({
        topic: this.topic,
        event: 'phx_leave',
        payload: {},
        ref,
        join_ref: this.joinRef,
      }, new Push(), 'leave', this)
    }
    this.status = 'closed'
    this.joinPush = null
    this.joinRef = null
    this.handlers.clear()
    this.socket.removeChannel(this)
  }

  onSocketOpen() {
    if (this.refCount === 0) return
    const push = this.joinPush && !this.joinPush.result ? this.joinPush : new Push()
    this.joinPush = push
    this.rejoining = this.everJoined
    this.status = 'joining'
    this.emit('phx_status', { status: this.status })
    this.sendJoin(push)
  }

  sendJoin(push) {
    if (!this.socket.isOpen() || this.refCount === 0) return
    const ref = this.socket.makeRef()
    this.joinRef = ref
    this.socket.send({
      topic: this.topic,
      event: 'phx_join',
      payload: this.params,
      ref,
      join_ref: ref,
    }, push, 'join', this)
  }

  onJoinResult(status, payload) {
    if (status === 'ok') {
      const wasRejoining = this.rejoining
      this.status = 'joined'
      this.everJoined = true
      this.rejoining = false
      this.emit('phx_status', { status: 'joined' })
      this.emit('phx_joined', payload)
      if (wasRejoining) this.emit('phx_reconnected', payload)
      return
    }
    this.status = status === 'timeout' ? 'timeout' : 'error'
    this.rejoining = false
    this.emit('phx_status', { status: this.status, error: payload })
    this.emit('phx_join_error', payload)
  }

  onSocketClose() {
    if (this.refCount === 0) return
    this.joinPush = null
    this.status = 'reconnecting'
    this.rejoining = this.everJoined
    this.emit('phx_status', { status: this.status })
    this.emit('phx_disconnected', { reason: 'disconnected' })
  }

  dispatch(event, payload, joinRef) {
    if (joinRef && this.joinRef && joinRef !== this.joinRef) return
    this.emit(event, payload)
  }

  addHandler(event, handler) {
    const handlers = this.handlers.get(event) || new Set()
    handlers.add(handler)
    this.handlers.set(event, handlers)
  }

  removeHandler(event, handler) {
    const handlers = this.handlers.get(event)
    handlers?.delete(handler)
    if (handlers?.size === 0) this.handlers.delete(event)
  }

  emit(event, payload) {
    Array.from(this.handlers.get(event) || []).forEach((handler) => handler(payload))
  }
}

class ChannelLease {
  constructor(entry) {
    this.entry = entry
    this.active = false
    this.handlers = []
  }

  on(event, handler) {
    if (typeof handler !== 'function') return () => {}
    const guarded = (payload) => {
      if (this.active) handler(payload)
    }
    this.handlers.push([event, guarded])
    this.entry.addHandler(event, guarded)
    return () => this.entry.removeHandler(event, guarded)
  }

	join() {
		if (this.active) {
			return this.entry.status === 'joined'
				? settledPush('ok', {})
				: this.entry.retryJoin()
    }
    this.active = true
    return this.entry.retain()
  }

  leave() {
    if (!this.active) return settledPush('ok', {})
    this.active = false
    this.handlers.forEach(([event, handler]) => this.entry.removeHandler(event, handler))
    this.handlers = []
    this.entry.release()
    return settledPush('ok', {})
  }
}

export class RealtimeSocket {
  constructor(endpoint = defaultEndpoint(), options = {}) {
    this.endpoint = endpoint
    this.webSocketFactory = options.webSocketFactory || ((url) => new WebSocket(url))
    this.joinTimeout = options.joinTimeout || DEFAULT_JOIN_TIMEOUT
    this.heartbeatInterval = options.heartbeatInterval || DEFAULT_HEARTBEAT_INTERVAL
    this.reconnectDelays = options.reconnectDelays || DEFAULT_RECONNECT_DELAYS
    this.channels = new Map()
    this.pending = new Map()
    this.ref = 0
    this.connection = null
    this.reconnectTimer = null
    this.heartbeatTimer = null
    this.reconnectAttempt = 0
    this.closed = false
  }

  channel(topic, params = {}) {
    let entry = this.channels.get(topic)
    if (!entry) {
      entry = new TopicChannel(this, topic, params)
      this.channels.set(topic, entry)
    }
    return entry.lease()
  }

  connect() {
    if (this.closed || this.connection?.readyState === 0 || this.isOpen()) return
    clearTimeout(this.reconnectTimer)
    this.reconnectTimer = null
    const connection = this.webSocketFactory(this.endpoint)
    this.connection = connection
    addSocketListener(connection, 'open', () => this.onOpen(connection))
    addSocketListener(connection, 'message', (event) => this.onMessage(event.data))
    addSocketListener(connection, 'close', () => this.onClose(connection))
    addSocketListener(connection, 'error', () => {})
  }

  onOpen(connection) {
    if (connection !== this.connection) return
    this.reconnectAttempt = 0
    this.startHeartbeat()
    Array.from(this.channels.values()).forEach((channel) => channel.onSocketOpen())
  }

  onClose(connection) {
    if (connection !== this.connection) return
    this.connection = null
    this.stopHeartbeat()
    for (const pending of this.pending.values()) {
      pending.push.trigger('error', { reason: 'disconnected' })
      if (pending.kind === 'join') pending.channel.onJoinResult('error', { reason: 'disconnected' })
    }
    this.pending.clear()
    Array.from(this.channels.values()).forEach((channel) => channel.onSocketClose())
    if (!this.closed && Array.from(this.channels.values()).some((channel) => channel.refCount > 0)) {
      this.scheduleReconnect()
    }
  }

  onMessage(raw) {
    let frame
    try {
      frame = JSON.parse(raw)
    } catch {
      return
    }
    if (frame.event === 'phx_reply' && frame.ref) {
      const pending = this.pending.get(String(frame.ref))
      if (!pending) return
      this.pending.delete(String(frame.ref))
      const status = frame.payload?.status || 'error'
      const response = frame.payload?.response || {}
      pending.push.trigger(status, response)
      if (pending.kind === 'join') pending.channel.onJoinResult(status, response)
      return
    }
    const channel = this.channels.get(frame.topic)
    if (!channel) return
    channel.dispatch(frame.event, frame.payload || {}, frame.join_ref)
  }

  send(frame, push, kind, channel) {
    if (!this.isOpen()) return false
    const ref = String(frame.ref)
    this.pending.set(ref, { push, kind, channel })
    push.startTimeout(this.joinTimeout, () => {
      this.pending.delete(ref)
      if (kind === 'join') channel.onJoinResult('timeout', { reason: 'timeout' })
    })
    this.connection.send(JSON.stringify(frame))
    return true
  }

  makeRef() {
    this.ref += 1
    return String(this.ref)
  }

  isOpen() {
    return this.connection?.readyState === 1
  }

  removeChannel(channel) {
    if (this.channels.get(channel.topic) === channel && channel.refCount === 0) {
      this.channels.delete(channel.topic)
    }
  }

  startHeartbeat() {
    this.stopHeartbeat()
    this.heartbeatTimer = setInterval(() => {
      if (!this.isOpen()) return
      const ref = this.makeRef()
      this.send({
        topic: 'phoenix', event: 'heartbeat', payload: {}, ref, join_ref: null,
      }, new Push(), 'heartbeat', null)
    }, this.heartbeatInterval)
  }

  stopHeartbeat() {
    clearInterval(this.heartbeatTimer)
    this.heartbeatTimer = null
  }

  scheduleReconnect() {
    if (this.reconnectTimer) return
    const index = Math.min(this.reconnectAttempt, this.reconnectDelays.length - 1)
    const delay = this.reconnectDelays[index]
    this.reconnectAttempt += 1
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null
      this.connect()
    }, delay)
  }

  disconnect() {
    this.closed = true
    clearTimeout(this.reconnectTimer)
    this.reconnectTimer = null
    this.stopHeartbeat()
    this.connection?.close()
    this.connection = null
    this.channels.clear()
    for (const pending of this.pending.values()) pending.push.trigger('error', { reason: 'closed' })
    this.pending.clear()
  }
}

function settledPush(status, payload) {
  const push = new Push()
  push.trigger(status, payload)
  return push
}

function defaultEndpoint() {
  if (typeof window === 'undefined') return 'ws://localhost/api/v1/ws'
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${window.location.host}/api/v1/ws`
}

function addSocketListener(socket, event, handler) {
  if (typeof socket.addEventListener === 'function') socket.addEventListener(event, handler)
  else socket[`on${event}`] = handler
}

function defer(callback) {
  if (typeof queueMicrotask === 'function') queueMicrotask(callback)
  else Promise.resolve().then(callback)
}

let sharedSocket

export function getRealtimeSocket() {
  if (!sharedSocket) sharedSocket = new RealtimeSocket()
  return sharedSocket
}

export function resetRealtimeSocket() {
  sharedSocket?.disconnect()
  sharedSocket = undefined
}

export const __realtimeTest = {
  reset: resetRealtimeSocket,
  setSocket(socket) { sharedSocket = socket },
}
