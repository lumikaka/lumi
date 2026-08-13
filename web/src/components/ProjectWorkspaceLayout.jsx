import { useEffect, useId, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { MessageCircle } from 'lucide-react'
import { Link, useLocation, useNavigate } from 'react-router-dom'

import { listRecentProjects } from '../api/projects.js'
import { projectQueryKeys } from '../api/projectQueryKeys.js'
import { useProjectRealtimeSync } from '../realtime/useProjectRealtimeSync.js'
import ChatArea from './ChatArea.jsx'
import GlobalTopbar from './GlobalTopbar.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { projectChatSearchWithoutLegacyScope } from '../pages/chatAreaPresentation.js'

const CHAT_COLLAPSED_KEY = 'lumi.projectChatAreaCollapsed'
const COMPACT_CHAT_QUERY = '(max-width: 980px)'

export default function ProjectWorkspaceLayout({ project, projectUuid, activeSection, children, hideChat = false }) {
  const { t } = useI18n()
  const location = useLocation()
  const navigate = useNavigate()
  const chatOverlayId = useId()
  const [compact, setCompact] = useState(readCompact)
	const [collapsed, setCollapsed] = useState(() => readCollapsed(projectUuid))
  const [overlayOpen, setOverlayOpen] = useState(false)
  const recentQuery = useQuery({ queryKey: projectQueryKeys.recent(), queryFn: listRecentProjects })
  useProjectRealtimeSync(projectUuid)

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return undefined
    const media = window.matchMedia(COMPACT_CHAT_QUERY)
    const handleChange = (event) => setCompact(event.matches)
    setCompact(media.matches)
    media.addEventListener?.('change', handleChange)
    return () => media.removeEventListener?.('change', handleChange)
  }, [])

  useEffect(() => {
		writeCollapsed(projectUuid, collapsed)
	}, [collapsed, projectUuid])

  useEffect(() => {
    if (!hasChatQuery(location.search)) return
    if (compact) setOverlayOpen(true)
    else setCollapsed(false)
  }, [compact, location.search])

  useEffect(() => {
    if (!new URLSearchParams(location.search).has('chat_scope')) return
    const params = projectChatSearchWithoutLegacyScope(location.search)
    navigate({ pathname: location.pathname, search: params.toString() ? `?${params}` : '', hash: location.hash }, { replace: true })
  }, [location.hash, location.pathname, location.search, navigate])

  useEffect(() => {
    if (!overlayOpen) return undefined
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') setOverlayOpen(false)
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      document.body.style.overflow = previousOverflow
      window.removeEventListener('keydown', handleKeyDown)
    }
  }, [overlayOpen])

  useEffect(() => {
    if (!compact && overlayOpen) setOverlayOpen(false)
  }, [compact, overlayOpen])

  return (
    <main className="project-shell">
      <GlobalTopbar
        title={project?.name || t('projects.fallback_name')}
        projectUuid={projectUuid}
        activeSection={activeSection}
        recentProjects={recentQuery.data?.items || []}
        onSwitchProject={(uuid) => navigate(`/projects/${encodeURIComponent(uuid)}/overview/summary`)}
        actions={compact && !hideChat ? <button className={`project-topbar__icon-button project-topbar__chat-button ${overlayOpen ? 'is-active' : ''}`} type="button" aria-label={t('chat.open')} aria-haspopup="dialog" aria-expanded={overlayOpen} aria-controls={overlayOpen ? chatOverlayId : undefined} title={t('chat.open')} onClick={() => setOverlayOpen(true)}><MessageCircle size={18} aria-hidden="true" /></button> : <Link className="project-topbar__action" to="/"><span>{t('projects.all')}</span></Link>}
      />
      <div className={`project-workbench ${hideChat ? 'project-workbench--solo' : !compact && collapsed ? 'project-workbench--chat-collapsed' : ''}`}>
        <section className="project-workbench__content" aria-label={t('projects.workspace')}>
          {children}
        </section>
        {!compact && !hideChat ? <ChatArea projectUuid={projectUuid} expanded={!collapsed} onToggle={() => setCollapsed((value) => !value)} /> : null}
      </div>
      {compact && !hideChat && overlayOpen ? (
        <div className="project-chat-overlay" id={chatOverlayId} role="dialog" aria-modal="true" aria-label={t('chat.project')}>
          <button className="project-chat-overlay__backdrop" type="button" aria-label={t('chat.close')} onClick={() => setOverlayOpen(false)} />
          <div className="project-chat-overlay__panel"><ChatArea projectUuid={projectUuid} expanded onToggle={() => setOverlayOpen(false)} overlay /></div>
        </div>
      ) : null}
    </main>
  )
}

function readCompact() {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function' && window.matchMedia(COMPACT_CHAT_QUERY).matches
}

function readCollapsed(projectUuid) {
	if (typeof window === 'undefined') return false
	try { return window.localStorage.getItem(`${CHAT_COLLAPSED_KEY}.${projectUuid}`) === 'true' } catch { return false }
}

function writeCollapsed(projectUuid, value) {
	if (typeof window === 'undefined') return
	try { window.localStorage.setItem(`${CHAT_COLLAPSED_KEY}.${projectUuid}`, value ? 'true' : 'false') } catch { /* restricted browser */ }
}

function hasChatQuery(search) {
  const params = new URLSearchParams(search)
  return Boolean(params.get('chat_thread_uuid') || params.get('workflow_uuid') || params.get('chat_new') === '1')
}
