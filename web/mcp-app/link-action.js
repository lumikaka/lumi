// MCP Apps open-link acknowledges a request; it cannot prove a visible window
// opened. Only the standard optional isError field has failure semantics.
const failure = (kind, message) => Object.assign(new Error(message), { kind })
const object = value => value !== null && typeof value === 'object' && !Array.isArray(value)

export function downloadURL(origin, href, project) {
  try {
    const base = new URL(origin), url = new URL(href, base)
    const prefix = `/media/projects/${project}/comic-exports/`
    if (base.protocol !== 'http:' || base.hostname !== '127.0.0.1' || !base.port || base.username || base.password ||
        typeof href !== 'string' || !href.startsWith(prefix) || /[%\\\u0000-\u0020]/.test(href) ||
        url.origin !== base.origin || !url.pathname.startsWith(prefix) || url.pathname === prefix || url.username || url.password) return null
    return url.href
  } catch { return null }
}

function failureMessage(error) {
  const reason = error.message ? `：${error.message}` : ''
  switch (error.kind) {
    case 'unsupported': return '宿主未声明打开链接能力，请复制链接到浏览器打开。'
    case 'not_ready': return '宿主尚未完成连接，请稍后重试或复制链接。'
    case 'invalid_url': return '下载链接未通过路径校验，请在 Lumi Web 页面继续。'
    case 'timeout': return '等待宿主响应超时，无法确认是否已打开。可复制链接到浏览器打开。'
    case 'transport_error': return `请求未能发出${reason}。请复制链接到浏览器打开。`
    case 'protocol_error': return `宿主链接协议不兼容或响应无效${reason}。请复制链接到浏览器打开。`
    case 'host_denied': return `宿主拒绝打开链接${reason}。请复制链接到浏览器打开。`
    case 'host_failed': return '宿主报告未能打开链接，未提供具体原因。请复制链接到浏览器打开。'
    case 'cancelled': return '打开请求已取消，无法确认是否已打开。'
    default: return `宿主请求异常${reason}。请复制链接到浏览器打开。`
  }
}

export class LinkAction {
  constructor(bridge, url, clipboard = globalThis.navigator?.clipboard) {
    this.bridge = bridge; this.url = url; this.clipboard = clipboard; this.listeners = new Set()
    this.state = { opening: false, copying: false, openMessage: '', copyMessage: '', manualCopy: false, diagnostics: null }
  }
  subscribe = listener => { this.listeners.add(listener); return () => this.listeners.delete(listener) }
  getSnapshot = () => this.state
  update(patch) { this.state = { ...this.state, ...patch }; this.listeners.forEach(listener => listener()) }
  async open() {
    if (this.state.opening) return
    const init = this.bridge.initialization
    const diagnostics = { protocol_version: init?.protocolVersion, host_info: init?.hostInfo, host_capabilities: init?.hostCapabilities, exchanges: [] }
    const exchanges = []
    this.update({ opening: true, openMessage: '正在请求打开…', diagnostics })
    try {
      if (!this.url) throw failure('invalid_url')
      if (!init) throw failure('not_ready')
      if (!object(init.hostCapabilities?.openLinks)) throw failure('unsupported')
      const result = await this.bridge.request('ui/open-link', { url: this.url }, {
        timeoutMs: 10000,
        onExchange: event => { exchanges.push(event); this.update({ diagnostics: { ...diagnostics, exchanges: [...exchanges] } }) },
      })
      if (!object(result) || (result.isError !== undefined && typeof result.isError !== 'boolean')) throw failure('protocol_error', 'ui/open-link 结果格式无效')
      if (result.isError === true) throw failure('host_failed')
      this.update({ openMessage: '已请求打开，无法确认外部页面是否已显示；若无跳转，请复制链接到浏览器打开。' })
    } catch (error) {
      this.update({ openMessage: failureMessage(error), diagnostics: { ...this.state.diagnostics, failure: { kind: error.kind || 'host_error', code: error.code, message: error.message } } })
    } finally { this.update({ opening: false }) }
  }
  async copy() {
    if (this.state.copying || !this.url) return
    this.update({ copying: true, copyMessage: '正在复制…', manualCopy: false })
    let timer
    try {
      if (!this.clipboard?.writeText) throw new Error('Clipboard unavailable')
      await Promise.race([
        this.clipboard.writeText(this.url),
        new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Clipboard timed out')), 5000) }),
      ])
      this.update({ copyMessage: '链接已复制，请粘贴到浏览器打开。' })
    } catch {
      this.update({ copyMessage: '复制失败，请选择下方完整地址后手动复制到浏览器。', manualCopy: true })
    } finally { clearTimeout(timer); this.update({ copying: false }) }
  }
}
