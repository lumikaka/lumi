import { useEffect, useId, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'

import canvasCollapseIcon from '../assets/figma/workspace/canvas-collapse.svg'
import canvasFullscreenIcon from '../assets/figma/workspace/canvas-fullscreen.svg'
import panelMenuIcon from '../assets/figma/workspace/panel-menu.svg'
import { useProjectRealtimeSync } from '../realtime/useProjectRealtimeSync.js'
import ChatArea from './ChatArea.jsx'
import FigmaIcon from './FigmaIcon.jsx'
import ProjectCanvasNavigation from './ProjectCanvasNavigation.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { projectChatSearchWithoutLegacyScope } from '../pages/chatAreaPresentation.js'

const CHAT_COLLAPSED_KEY = 'lumi.projectChatAreaCollapsed'
const CANVAS_FULLSCREEN_KEY = 'lumi.projectCanvasFullscreen'
const COMPACT_CHAT_QUERY = '(max-width: 1179px)'

export default function ProjectWorkspaceLayout({ project, projectUuid, children, hideChat = false, composerDraftRequest = null }) {
  const { t } = useI18n()
  const location = useLocation()
  const navigate = useNavigate()
  const chatOverlayId = useId()
  const [compact, setCompact] = useState(readCompact)
	const [collapsed, setCollapsed] = useState(() => readCollapsed(projectUuid))
  const [canvasFullscreen, setCanvasFullscreen] = useState(readCanvasFullscreen)
  const [canvasCollapsed, setCanvasCollapsed] = useState(false)
  const [overlayOpen, setOverlayOpen] = useState(false)
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
    try { window.localStorage.setItem(CANVAS_FULLSCREEN_KEY, canvasFullscreen ? 'true' : 'false') } catch { /* restricted browser */ }
  }, [canvasFullscreen])

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

  useEffect(() => {
    setCanvasCollapsed(false)
  }, [location.pathname])

  useEffect(() => {
    if (!composerDraftRequest?.id || hideChat) return
    setCanvasFullscreen(false)
    setCanvasCollapsed(false)
    if (compact) setOverlayOpen(true)
    else setCollapsed(false)
  }, [compact, composerDraftRequest?.id, hideChat])

  const chatHidden = hideChat || canvasFullscreen
  const chatMounted = !hideChat
  const workbenchClass = [
    'project-workbench',
    chatHidden ? 'project-workbench--solo' : '',
    !compact && !chatHidden && collapsed ? 'project-workbench--chat-collapsed' : '',
    !compact && !chatHidden && canvasCollapsed ? 'project-workbench--canvas-collapsed' : '',
    compact && overlayOpen ? 'project-workbench--chat-overlay-open' : '',
  ].filter(Boolean).join(' ')

  return (
    <main className="project-shell">
      <div className={workbenchClass}>
        {chatMounted ? (
          <div className={`project-chat-host${compact && overlayOpen ? ' is-overlay' : ''}`} id={compact && overlayOpen ? chatOverlayId : undefined} role={compact && overlayOpen ? 'dialog' : undefined} aria-modal={compact && overlayOpen ? 'true' : undefined} aria-label={compact && overlayOpen ? t('chat.project') : undefined}>
            <ChatArea projectUuid={projectUuid} expanded={compact || !collapsed || canvasCollapsed} onToggle={compact ? () => setOverlayOpen(false) : () => setCollapsed((value) => !value)} overlay={compact} composerDraftRequest={composerDraftRequest} />
          </div>
        ) : null}
        {canvasCollapsed && !compact && !chatHidden ? (
          <button className="project-canvas-rail" type="button" onClick={() => setCanvasCollapsed(false)} aria-label={t('projects.canvas.expand')} title={t('projects.canvas.expand')}>
            <FigmaIcon src={canvasCollapseIcon} size={16} />
            <span>{t('projects.canvas.title')}</span>
          </button>
        ) : (
          <section className="project-canvas" aria-label={t('projects.workspace')}>
            <header className="project-canvas__header">
              <div className="project-canvas__heading">
                <h1>{t('projects.title')}</h1>
                <span className="project-canvas__menu-icon"><FigmaIcon src={panelMenuIcon} size={16} /></span>
              </div>
              <div className="project-canvas__actions">
                {compact && !hideChat ? <button className="project-canvas__text-action" type="button" aria-label={t('chat.open')} aria-haspopup="dialog" aria-expanded={overlayOpen} aria-controls={overlayOpen ? chatOverlayId : undefined} onClick={() => setOverlayOpen(true)}>{t('chat.title')}</button> : null}
                {!compact && !hideChat ? (
                  <button type="button" onClick={() => { setCanvasFullscreen((value) => !value); setCanvasCollapsed(false) }} aria-label={t(canvasFullscreen ? 'projects.canvas.exit_fullscreen' : 'projects.canvas.fullscreen')} title={t(canvasFullscreen ? 'projects.canvas.exit_fullscreen' : 'projects.canvas.fullscreen')} aria-pressed={canvasFullscreen}>
                    <FigmaIcon src={canvasFullscreenIcon} size={16} />
                  </button>
                ) : null}
                {!compact && !hideChat && !canvasFullscreen ? <button type="button" onClick={() => setCanvasCollapsed(true)} aria-label={t('projects.canvas.collapse')} title={t('projects.canvas.collapse')}><FigmaIcon src={canvasCollapseIcon} size={16} /></button> : null}
              </div>
            </header>
            <div className="project-workbench__content">{children}</div>
            <ProjectCanvasNavigation projectUuid={projectUuid} />
          </section>
        )}
      </div>
      {compact && !hideChat && overlayOpen ? (
        <div className="project-chat-overlay" aria-hidden="true">
          <button className="project-chat-overlay__backdrop" type="button" aria-label={t('chat.close')} onClick={() => setOverlayOpen(false)} />
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

function readCanvasFullscreen() {
  if (typeof window === 'undefined') return false
  try { return window.localStorage.getItem(CANVAS_FULLSCREEN_KEY) === 'true' } catch { return false }
}

function hasChatQuery(search) {
  const params = new URLSearchParams(search)
  return Boolean(params.get('chat_thread_uuid') || params.get('workflow_uuid') || params.get('chat_new') === '1')
}
