const object = value => value !== null && typeof value === 'object' && !Array.isArray(value)

// One writer for the entire App: contexts replace each other in the host.
// Serialize requests and coalesce queued clicks so an older response never
// reports success for the latest selection or overtakes its context update.
export class ModelContextAction {
  constructor(bridge) {
    this.bridge = bridge
    this.listeners = new Set()
    this.state = { message: '', pending: false }
    this.sequence = 0
  }
  subscribe = fn => { this.listeners.add(fn); return () => this.listeners.delete(fn) }
  getSnapshot = () => this.state
  update(state) { this.state = state; this.listeners.forEach(fn => fn()) }
  select = context => {
    this.queued = { context, sequence: ++this.sequence }
    this.update({ pending: true, message: `正在同步引用：${context.title || '文本候选'}…` })
    if (!this.running) void this.drain()
  }
  async drain() {
    this.running = true
    while (this.queued) {
      const { context, sequence } = this.queued
      this.queued = null
      try {
        await this.bridge.updateModelContext(context)
        if (sequence === this.sequence) this.update({ pending: false, message: `宿主已接收引用：${context.title || '文本候选'}。请在当前对话中继续。` })
      } catch (error) {
        // On timeout the host may still apply the old request. Do not dispatch
        // queued selections and imply that their ordering can be guaranteed.
        if (error.kind === 'timeout') this.queued = null
        if (sequence === this.sequence || error.kind === 'timeout') {
          const reason = error.kind === 'unsupported' ? '宿主未声明上下文更新能力' : error.kind === 'not_ready' ? '宿主尚未完成连接' : error.kind === 'timeout' ? '宿主响应超时，无法确认当前引用，请重新点选后等待回执' : `宿主未接受引用：${error.message}`
          this.update({ pending: false, message: `${reason}。尚未确认最新选择已同步，请勿依赖之前的引用。` })
        }
      }
    }
    this.running = false
  }
}

export function modelContextParams(initialization, context) {
  if (!initialization) throw Object.assign(new Error('宿主尚未完成连接'), { kind: 'not_ready' })
  const capabilities = initialization.hostCapabilities?.updateModelContext
  if (!object(capabilities) || (!object(capabilities.text) && !object(capabilities.structuredContent))) throw Object.assign(new Error('宿主未声明上下文更新能力'), { kind: 'unsupported' })
  return {
    ...(object(capabilities.text) ? { content: [{ type: 'text', text: `用户在 Lumi 中选中了以下内容，供当前对话的下一条消息引用；这不是保存或采用操作。\n${JSON.stringify(context, null, 2)}` }] } : {}),
    ...(object(capabilities.structuredContent) ? { structuredContent: context } : {}),
  }
}
