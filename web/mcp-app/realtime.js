import { success, uuidV7 } from './contracts.js'

function hint(result, project, subscription) {
  const value = success(result)
  if (value.project_uuid !== project || !uuidV7.test(value.subscription_uuid) ||
      (subscription && value.subscription_uuid !== subscription) ||
      !['joined', 'changed', 'heartbeat'].includes(value.state) ||
      typeof value.cursor !== 'string' || !/^(0|[1-9][0-9]{0,19})$/.test(value.cursor) ||
      Object.keys(value).some(key => !['project_uuid', 'subscription_uuid', 'cursor', 'state', 'event'].includes(key)) ||
      (value.state === 'changed' && (typeof value.event !== 'string' || !value.event || value.event.length > 128))) throw new Error('实时订阅结果无效')
  return value
}

// Each next call waits for an actual event on the SAME CLI WebSocket. There is
// no timer issuing business reads. WS pings finish waits without invalidating
// Query; a persistent cursor preserves changes between two tool calls.
export class AppRealtime {
  constructor(bridge, project, { startupTimeout = 15000, retryDelays = [250, 500, 1000, 2000, 5000] } = {}) {
    Object.assign(this, { bridge, project, startupTimeout, retryDelays })
    this.handlers = new Map(); this.messages = new Set(); this.disposed = false; this.attempt = 0
  }
  channel(topic) {
    if (topic !== `project:${this.project}`) throw new Error('实时目标不匹配')
    return {
      on: (event, fn) => { const set = this.handlers.get(event) ?? new Set(); set.add(fn); this.handlers.set(event, set); return () => set.delete(fn) },
      onMessage: fn => { this.messages.add(fn); return () => this.messages.delete(fn) },
      join: () => this.start(), leave: () => this.disconnect(),
    }
  }
  emit(event, value) { this.handlers.get(event)?.forEach(fn => fn(value)) }
  request(action, args = {}, options = {}) {
    return this.bridge.request('tools/call', { name: 'watch_app_changes', arguments: { action, project_uuid: this.project, ...args } }, options)
  }
  close(subscription) {
    if (subscription) void this.request('close', { subscription_uuid: subscription }, { timeoutMs: 5000 }).catch(() => {})
  }
  async start() {
    if (this.disposed || this.controller) return
    const controller = new AbortController(); this.controller = controller
    const active = () => !this.disposed && this.controller === controller
    this.emit('phx_status', { status: 'connecting' })
    let subscription
    try {
      const opened = hint(await this.request('open', {}, { timeoutMs: this.startupTimeout, signal: controller.signal }), this.project)
      subscription = opened.subscription_uuid
      if (!active()) { this.close(subscription); return }
      if (opened.state !== 'joined') throw new Error('实时订阅未建立')
      this.subscription = subscription; this.everJoined = true
      let cursor = opened.cursor
      this.emit('phx_status', { status: 'joined' }); this.emit('phx_joined', {})
      while (active()) {
        const data = hint(await this.request('next', { subscription_uuid: subscription, cursor }, { timeoutMs: 0, signal: controller.signal }), this.project, subscription)
        if (!active()) return
        if (data.state === 'joined' || BigInt(data.cursor) <= BigInt(cursor)) throw new Error('实时游标无效')
        cursor = data.cursor
        this.attempt = 0
        if (data.state === 'changed') this.messages.forEach(fn => fn(data.event, { project_uuid: this.project }))
      }
    } catch (error) {
      if (!active()) return
      this.close(subscription); this.subscription = null; this.controller = null
      this.emit('phx_status', { status: 'disconnected' })
      // Only a confirmed lost CLI subscription reconnects automatically. Host
      // refusal/timeout/protocol failure requires manual retry, avoiding loops.
      if (this.everJoined && this.attempt < this.retryDelays.length && ['app_watch_expired', 'app_watch_disconnected', 'app_watch_cancelled'].includes(error.code)) {
        const delay = this.retryDelays[this.attempt++]
        this.retry = setTimeout(() => this.start(), delay)
      }
    }
  }
  disconnect() {
    if (this.disposed) return
    this.disposed = true; clearTimeout(this.retry); this.controller?.abort(); this.controller = null
    this.close(this.subscription); this.subscription = null
    this.handlers.clear(); this.messages.clear()
  }
}
