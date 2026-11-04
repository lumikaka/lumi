// Explicit browser integration harness; never imported by the product entry.
import React, { useState } from 'react'
import { createRoot } from 'react-dom/client'
import { CindyWorkspace } from './Workspace.jsx'
import { createCindySession } from './session.js'
import './workspace.scss'

function Fixture() {
  const [session, setSession] = useState(null), [error, setError] = useState('')
  const show = async (slot, view) => {
    try {
      const launch = await (await fetch(`/launch/${slot}/${view}`)).json()
      const next = createCindySession(launch.result, {
        async call(name, args, { signal, project }) {
          const response = await fetch(`/call/${launch.key}`, { method: 'POST', body: JSON.stringify({ name, args, project }), signal })
          const body = await response.json()
          if (body.transport_error) throw Object.assign(new Error(body.transport_error.message), body.transport_error)
          return body
        },
        async close() { await fetch(`/close/${launch.key}`, { method: 'POST', keepalive: true }) },
      })
      setSession(next); setError('')
    } catch (e) { setError(e.message) }
  }
  return <><aside aria-label="独立测试宿主" style={{ padding: 8 }}><strong>模拟页面授权 · 真实 Lumi CLI</strong>{['a', 'b'].map(slot => <span key={slot}>{['book', 'chapters', 'chapter', 'page', 'settings', 'setting', 'candidates'].map(view => <button key={view} onClick={() => show(slot, view)}>{slot} · {view}</button>)}</span>)}<button onClick={() => { setSession(null) }}>关闭页面</button></aside>{error && <p role="alert">{error}</p>}{session && <CindyWorkspace key={session.key} session={session} />}</>
}
createRoot(document.getElementById('root')).render(<Fixture />)
