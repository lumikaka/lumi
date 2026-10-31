import { imageData } from './contracts.js'

export class PreviewCache {
  constructor(call, { concurrency = 3, capacity = 32, maxBytes = 6 * 1024 * 1024 } = {}) {
    this.call = call; this.concurrency = concurrency; this.capacity = capacity; this.maxBytes = maxBytes
    this.cache = new Map(); this.pending = new Map(); this.queue = []; this.active = 0; this.bytes = 0
  }
  load(project, asset, edge, signal) {
    const key = `${project}:${asset}:${edge}`
    if (this.cache.has(key)) { const hit = this.cache.get(key); this.cache.delete(key); this.cache.set(key, hit); return Promise.resolve(hit) }
    if (this.pending.has(key)) return this.subscribe(this.pending.get(key), signal)
    const job = { key, asset, edge, signals: new Set(), keep: false }
    job.promise = new Promise((resolve, reject) => { Object.assign(job, { resolve, reject }) })
    this.pending.set(key, job); this.queue.push(job)
    const result = this.subscribe(job, signal); this.pump(); return result
  }
  subscribe(job, signal) {
    if (!signal) { job.keep = true; return job.promise }
    job.signals.add(signal)
    return new Promise((resolve, reject) => {
      const aborted = () => {
        job.signals.delete(signal)
        const error = Object.assign(new Error('预览已切换'), { kind: 'cancelled' })
        reject(error)
        if (!job.keep && !job.signals.size && this.queue.includes(job)) {
          this.queue = this.queue.filter((v) => v !== job); this.pending.delete(job.key); job.reject(error)
        }
      }
      signal.addEventListener('abort', aborted, { once: true })
      job.promise.then(resolve, reject).finally(() => { signal.removeEventListener('abort', aborted); job.signals.delete(signal) })
      if (signal.aborted) aborted()
    })
  }
  pump() {
    while (this.active < this.concurrency && this.queue.length) {
      const job = this.queue.shift(); this.active++
      Promise.resolve().then(() => this.call('read_app_image', { asset_uuid: job.asset, max_edge: job.edge }))
        .then((result) => {
          const data = imageData(result, job.asset, job.edge)
          this.cache.set(job.key, data); this.bytes += data.data_uri.length * 2
          while (this.cache.size > this.capacity || this.bytes > this.maxBytes) {
            const key = this.cache.keys().next().value
            this.bytes -= this.cache.get(key).data_uri.length * 2; this.cache.delete(key)
          }
          job.resolve(data)
        })
        .catch(job.reject).finally(() => { this.pending.delete(job.key); this.active--; this.pump() })
    }
  }
}
// Only visible previews request data. Each consumer discards older responses,
// including a previous asset resolving after a rapid page/version switch.
export class PreviewRequest {
  constructor(cache, publish) { this.cache = cache; this.publish = publish; this.generation = 0 }
  show(project, asset, edge, visible) {
    this.cancel()
    const generation = ++this.generation
    if (!asset) { this.publish({ state: 'missing' }); return }
    if (!visible) { this.publish({ state: 'waiting' }); return }
    this.publish({ state: 'loading' })
    this.controller = new AbortController()
    this.cache.load(project, asset, edge, this.controller.signal).then(
      (data) => { if (this.generation === generation) this.publish({ state: 'ready', data }) },
      (error) => { if (this.generation === generation) this.publish({ state: 'error', error }) },
    )
  }
  cancel() { this.generation++; this.controller?.abort() }
}
