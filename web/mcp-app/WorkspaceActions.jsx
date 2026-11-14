import { useEffect, useRef } from 'react'
import { BookOpen, Ellipsis, House, RefreshCw, Shapes } from 'lucide-react'
import { NavLink, useLocation } from 'react-router-dom'
import { HostLink } from './HostLink.jsx'
import { disconnectedMessage } from './sync.js'

export default function WorkspaceActions({ base, status, bridge, url, onReconnect, confirmLeave }) {
  const details = useRef(null)
  const trigger = useRef(null)
  const location = useLocation()

  useEffect(() => {
    const panel = details.current
    const closeOutside = event => {
      if (panel.open && !panel.contains(event.target)) panel.open = false
    }
    const escape = event => {
      if (event.key === 'Escape' && panel.open) {
        event.preventDefault()
        panel.open = false
        trigger.current.focus()
      }
    }
    document.addEventListener('pointerdown', closeOutside)
    document.addEventListener('focusin', closeOutside)
    document.addEventListener('keydown', escape)
    return () => {
      document.removeEventListener('pointerdown', closeOutside)
      document.removeEventListener('focusin', closeOutside)
      document.removeEventListener('keydown', escape)
    }
  }, [])

  useEffect(() => {
    if (!details.current.open) return
    const restoreFocus = details.current.contains(document.activeElement)
    details.current.open = false
    if (restoreFocus) trigger.current.focus()
  }, [location.key])

  const disconnected = status === disconnectedMessage
  return <details className="mcp-more" ref={details}>
    <summary ref={trigger} aria-label={disconnected ? '更多操作，实时连接中断' : '更多操作'} title={disconnected ? status : '更多操作'}>
      <Ellipsis size={18} aria-hidden="true" />
      {disconnected && <span className="mcp-more__alert" aria-hidden="true">!</span>}
    </summary>
    <div className="mcp-more__panel">
      <nav aria-label="作品工作区">
        {[[base + '/', '作品', House], [base + '/chapters', '章节', BookOpen], [base + '/premise', '设定', Shapes]].map(([to, label, Icon]) =>
          <NavLink key={to} to={to} end={to === base + '/'} onClick={event => {
            if (!confirmLeave()) event.preventDefault()
          }}>
            <Icon size={16} aria-hidden="true" />{label}
          </NavLink>
        )}
      </nav>
      <HostLink bridge={bridge} url={url} showOpen={false} />
      <div className="mcp-more__sync">
        <span role="status">{status}</span>
        <button type="button" className="mcp-more__reconnect" onClick={onReconnect}><RefreshCw size={16} aria-hidden="true" />重新连接</button>
      </div>
    </div>
  </details>
}
