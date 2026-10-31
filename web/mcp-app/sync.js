// WebSocket messages are hints only. Query functions call the MCP bridge,
// whose cwd-bound CLI forwards REST reads to SQLite's authoritative state.
export function bindReviewSync({ socket, project, queryClient, surface = window, documentSurface = document, onStatus = () => {}, calibrateOnFocus = true }) {
  const key = ['app-review', project]
  const calibrate = () => { void queryClient.invalidateQueries({ queryKey: key, refetchType: 'active' }) }
  const channel = socket?.channel(`project:${project}`)
  const cleanups = channel ? [channel.on('phx_joined', () => { onStatus('已同步'); calibrate() }),
    channel.on('phx_status', ({ status }) => { if (status !== 'joined') onStatus(status === 'connecting' ? '正在连接实时提示…' : '实时连接中断，当前为上次读取内容') }),
    channel.onMessage((event) => { if (!event.startsWith('phx_')) calibrate() })] : []
  const visible = () => { if (documentSurface.visibilityState === 'visible') calibrate() }
  if (calibrateOnFocus) { surface.addEventListener('focus', calibrate); documentSurface.addEventListener('visibilitychange', visible) }
  channel?.join()
  return () => { cleanups.forEach((fn) => fn()); channel?.leave(); socket?.disconnect(); if (calibrateOnFocus) { surface.removeEventListener('focus', calibrate); documentSurface.removeEventListener('visibilitychange', visible) } }
}
