import { failureKind } from './contracts.js'
export class AppBridge {
  constructor(surface = window, timeout = 30000) {
    this.surface = surface; this.timeout = timeout; this.serial = 0; this.pending = new Map(); this.listeners = new Set()
    this.receive = ({ source, data }) => {
      if (source !== surface.parent || data?.jsonrpc !== '2.0') return
      if (data.method === 'ui/resource-teardown' && data.id !== undefined) {
        this.listeners.forEach((fn) => fn(data.method, data.params))
        this.surface.parent.postMessage({ jsonrpc: '2.0', id: data.id, result: {} }, '*')
        return
      }
      if (data.method) { this.listeners.forEach((fn) => fn(data.method, data.params)); return }
      const pending = this.pending.get(data.id)
      if (pending) {
        this.pending.delete(data.id); pending.cleanup()
        if (data.error) pending.reject(Object.assign(new Error(data.error.message ?? '宿主请求失败'), data.error, { kind: failureKind(data.error) }))
        else pending.resolve(data.result)
      }
    }
    surface.addEventListener('message', this.receive)
  }
  request(method, params, { timeoutMs = this.timeout, signal } = {}) {
    const id = ++this.serial
    return new Promise((resolve, reject) => {
      let timer
      const stop = (kind, message) => {
        if (!this.pending.has(id)) return
        this.pending.delete(id); cleanup()
        this.notify('notifications/cancelled', { requestId: id, reason: message })
        reject(Object.assign(new Error(message), { kind }))
      }
      const abort = () => stop('cancelled', '审阅界面已切换或关闭')
      const cleanup = () => { clearTimeout(timer); signal?.removeEventListener('abort', abort) }
      if (timeoutMs > 0) timer = setTimeout(() => stop('timeout', '请求超时，可手动重试'), timeoutMs)
      this.pending.set(id, { resolve, reject, cleanup })
      signal?.addEventListener('abort', abort, { once: true })
      this.surface.parent.postMessage({ jsonrpc: '2.0', id, method, params }, '*')
      if (signal?.aborted) abort()
    })
  }
  notify(method, params = {}) { this.surface.parent.postMessage({ jsonrpc: '2.0', method, params }, '*') }
  call(name, args) { return this.request('tools/call', { name, arguments: args }) }
  async initialize() {
    const result = await this.request('ui/initialize', { appInfo: { name: 'Lumi review', version: '1.0.2' }, appCapabilities: {}, protocolVersion: '2026-01-26' })
    this.notify('ui/notifications/initialized'); return result
  }
  onNotification(fn) { this.listeners.add(fn); return () => this.listeners.delete(fn) }
  dispose() {
    this.surface.removeEventListener('message', this.receive)
    for (const [id, p] of this.pending) { p.cleanup(); this.notify('notifications/cancelled', { requestId: id, reason: 'View closed' }); p.reject(Object.assign(new Error('界面已关闭'), { kind: 'cancelled' })) }
    this.pending.clear(); this.listeners.clear()
  }
}
