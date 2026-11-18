import React, { useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { CindyWorkspace } from './Workspace.jsx'
import { createCindySession } from './session.js'
import { applyHostTheme, createHostTransport } from './host.js'
import './workspace.scss'
export { CindyWorkspace } from './Workspace.jsx'
export { createCindySession } from './session.js'

function App() {
  const [session, setSession] = useState(null), [error, setError] = useState('')
  useEffect(() => {
    let app, current, disposed = false
    const start = async () => {
      const { App: HostApp } = await import('@modelcontextprotocol/ext-apps')
      if (disposed) return
      app = new HostApp({ name: 'Lumi 作品工作区', version: '0.5.0' }, {}, { autoResize: true })
      // Register before connect: Cindy replays the original result when a card
      // reopens. Each card/session owns its QueryClient and media caches.
      app.ontoolresult = result => {
        if (disposed) return
        try {
          const next = createCindySession(result, createHostTransport(app))
          void current?.close()
          current = next
          setSession(next); setError('')
        } catch (failure) {
          void current?.close(); current = null
          setSession(null); setError(failure.message)
        }
      }
      app.onhostcontextchanged = () => applyHostTheme(app.getHostContext())
      app.onteardown = async () => { disposed = true; await current?.close(); setSession(null); return {} }
      await app.connect()
      applyHostTheme(app.getHostContext())
    }
    start().catch(failure => setError(`工作区连接失败：${failure.message}`))
    return () => { disposed = true; void current?.close(); void app?.close() }
  }, [])
  return session ? <CindyWorkspace key={session.key} session={session} /> : <main className="cindy-workspace__blocked"><h1>Lumi 作品工作区</h1><p role={error ? 'alert' : 'status'}>{error || '正在连接 Cindy，等待当前工具结果…'}</p></main>
}
createRoot(document.getElementById('root')).render(<App />)
