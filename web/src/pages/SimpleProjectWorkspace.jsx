import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Activity,
  Download,
  LayoutDashboard,
  Menu,
  MessageCircle,
  MoreHorizontal,
  Settings,
} from 'lucide-react'
import { Link, Route, Routes, useLocation, useNavigate } from 'react-router-dom'

import { listRecentProjects } from '../api/projects.js'
import { getChapter } from '../api/story.js'
import { getPremiseAsset, listComicSections } from '../api/production.js'
import { projectQueryKeys } from '../api/projectQueryKeys.js'
import { projectRoute } from '../projectRoutes.js'
import { useProjectRealtimeSync } from '../realtime/useProjectRealtimeSync.js'
import ChatArea from '../components/ChatArea.jsx'
import DraftProjectWorkspace from '../components/DraftProjectWorkspace.jsx'
import GlobalSidebar, { useGlobalSidebarState } from '../components/GlobalSidebar.jsx'
import { useProjectDashboardMode } from '../components/ProjectDashboardModeContext.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { PROJECT_DASHBOARD_MODE_EXPERT } from '../projectDashboardMode.js'
import ProjectLLMLogsPanel from './ProjectLLMLogsPanel.jsx'
import { OverviewExportsPanel } from './ProjectOverviewPanels.jsx'
import {
  SimpleBookView,
  SimpleBooksPage,
  SimpleHomePage,
  SimpleNotFound,
  SimplePageView,
  SimpleSettingDetailPage,
  SimpleSettingsPage,
  SimpleStoryPage,
} from './SimpleProjectPages.jsx'
import { orderedSimplePages, simpleProjectChatReference, simpleProjectRouteState } from './simpleProjectState.js'

const SIMPLE_CHAT_KEY = 'lumi.simpleProjectChatCollapsed'
const COMPACT_QUERY = '(max-width: 1199px)'

export default function SimpleProjectWorkspace({ projectUuid, projectQuery }) {
  const { t } = useI18n()
  const location = useLocation()
  const navigate = useNavigate()
  const { selectMode } = useProjectDashboardMode()
  const routeState = useMemo(() => simpleProjectRouteState(location.pathname, projectUuid), [location.pathname, projectUuid])
  const [sidebarCollapsed, setSidebarCollapsed] = useGlobalSidebarState()
  const [mobileNavigationOpen, setMobileNavigationOpen] = useState(false)
  const [compact, setCompact] = useState(readCompact)
  const [chatCollapsed, setChatCollapsed] = useState(() => readChatCollapsed(projectUuid))
  const [chatOverlayOpen, setChatOverlayOpen] = useState(false)
  const recentQuery = useQuery({ queryKey: projectQueryKeys.recent(), queryFn: listRecentProjects })
  const referenceAssetQuery = useQuery({
    queryKey: ['premise-asset', projectUuid, routeState.assetUuid],
    queryFn: () => getPremiseAsset(projectUuid, routeState.assetUuid),
    enabled: routeState.key === 'setting' && Boolean(routeState.assetUuid),
  })
  const referenceChapterQuery = useQuery({
    queryKey: ['story-chapter', projectUuid, routeState.chapterUuid],
    queryFn: () => getChapter(projectUuid, routeState.chapterUuid),
    enabled: ['chapter', 'pages', 'page', 'book'].includes(routeState.key) && Boolean(routeState.chapterUuid),
  })
  const referenceSectionsQuery = useQuery({
    queryKey: ['comic-sections', projectUuid, routeState.chapterUuid],
    queryFn: () => listComicSections(projectUuid, routeState.chapterUuid),
    enabled: ['page', 'pages'].includes(routeState.key) && Boolean(routeState.chapterUuid),
  })
  const referenceSections = orderedSimplePages(referenceSectionsQuery.data?.items || [])
  const referenceSection = routeState.sectionUuid
    ? referenceSections.find((item) => item.uuid === routeState.sectionUuid)
    : referenceSections[0]
  const chatReference = simpleProjectChatReference(routeState, { asset: referenceAssetQuery.data, chapter: referenceChapterQuery.data, section: referenceSection })
  const project = projectQuery.data
  const pageTitle = t({ home: 'simple.shell.page.home', story: 'simple.shell.page.story', llm_logs: 'settings.llm_logs', exports: 'projects.tab.exports', settings: 'simple.shell.page.settings', setting: 'simple.shell.page.settings', books: 'simple.shell.page.books', chapter: 'simple.shell.page.pages', pages: 'simple.shell.page.pages', page: 'simple.shell.page.page', book: 'simple.shell.page.book' }[routeState.key] || 'simple.shell.page.home')
  const chatOpen = compact ? chatOverlayOpen : !chatCollapsed
  const pageEditorActive = project?.setup_status !== 'draft' && ['page', 'pages'].includes(routeState.key)

  useProjectRealtimeSync(projectUuid)

  useEffect(() => {
    try { window.localStorage.setItem(`${SIMPLE_CHAT_KEY}.${projectUuid}`, chatCollapsed ? 'true' : 'false') } catch { /* restricted browser */ }
  }, [chatCollapsed, projectUuid])

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return undefined
    const media = window.matchMedia(COMPACT_QUERY)
    const handleChange = (event) => setCompact(event.matches)
    setCompact(media.matches)
    media.addEventListener?.('change', handleChange)
    return () => media.removeEventListener?.('change', handleChange)
  }, [])

  useEffect(() => {
    if (!hasChatControl(location.search)) return
    if (compact) setChatOverlayOpen(true)
    else setChatCollapsed(false)
  }, [compact, location.search])

  useEffect(() => {
    if (!chatOverlayOpen) return undefined
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') setChatOverlayOpen(false)
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      document.body.style.overflow = previousOverflow
      window.removeEventListener('keydown', handleKeyDown)
    }
  }, [chatOverlayOpen])

  const switchProject = (uuid) => {
    navigate(`/projects/${encodeURIComponent(uuid)}`)
  }

  const toggleChat = () => {
    if (compact) {
      setChatOverlayOpen((open) => !open)
      return
    }
    setChatCollapsed((collapsed) => !collapsed)
  }

  const openProjectConfiguration = () => {
    navigate(projectRoute(projectUuid, '', location.search), { state: { openProjectConfiguration: true } })
  }

  return (
    <main className={`simple-project-shell${sidebarCollapsed ? ' global-sidebar-collapsed' : ''}`}>
      <GlobalSidebar collapsed={sidebarCollapsed} mobileOpen={mobileNavigationOpen} onClose={() => setMobileNavigationOpen(false)} onToggleCollapsed={() => setSidebarCollapsed((value) => !value)} recentProjects={recentQuery.data?.items || []} recentProjectsLoading={recentQuery.isLoading} onSwitchProject={switchProject} />
      <SimpleProjectTopbar
        pageTitle={pageTitle}
        project={project}
        projectUuid={projectUuid}
        search={location.search}
        chatOpen={chatOpen}
        onToggleChat={toggleChat}
        onOpenProjectConfiguration={openProjectConfiguration}
        onSwitchWorkspaceMode={() => selectMode(PROJECT_DASHBOARD_MODE_EXPERT)}
        onOpenNavigation={() => setMobileNavigationOpen(true)}
      />
      <div className={`simple-project-workbench${!compact && chatCollapsed ? ' is-chat-collapsed' : ''}`}>
        <section className={`simple-project-content${pageEditorActive ? ' simple-project-content--page-editor' : ''}`} aria-label={t('projects.workspace')}>
          {project?.setup_status === 'draft' ? <DraftProjectWorkspace /> : (
            <Routes>
              <Route index element={<SimpleHomePage project={project} projectUuid={projectUuid} projectQuery={projectQuery} />} />
              <Route path="story" element={<SimpleStoryPage project={project} projectUuid={projectUuid} />} />
              <Route path="llm-logs" element={<div className="simple-project-page simple-llm-logs-page"><ProjectLLMLogsPanel projectUuid={projectUuid} standalone /></div>} />
              <Route path="exports" element={<div className="simple-project-page simple-exports-page"><OverviewExportsPanel projectUuid={projectUuid} standalone /></div>} />
              <Route path="premise" element={<SimpleSettingsPage projectUuid={projectUuid} />} />
              <Route path="premise/assets/:assetUuid" element={<SimpleSettingDetailPage projectUuid={projectUuid} />} />
              <Route path="chapters" element={<SimpleBooksPage projectUuid={projectUuid} />} />
              <Route path="chapters/:chapterUuid" element={<SimplePageView project={project} projectUuid={projectUuid} />} />
              <Route path="chapters/:chapterUuid/sections/:sectionUuid" element={<SimplePageView project={project} projectUuid={projectUuid} />} />
              <Route path="chapters/:chapterUuid/preview" element={<SimpleBookView projectUuid={projectUuid} />} />
              <Route path="*" element={<SimpleNotFound projectUuid={projectUuid} />} />
            </Routes>
          )}
        </section>
        {!compact ? <ChatArea projectUuid={projectUuid} pictureBook={project?.picture_book} expanded={!chatCollapsed} onToggle={() => setChatCollapsed((value) => !value)} newThreadReference={chatReference} /> : null}
      </div>
      {compact && chatOverlayOpen ? <div className="simple-project-overlay" role="dialog" aria-modal="true" aria-label={t('chat.project')}><button className="simple-project-overlay__backdrop" type="button" aria-label={t('simple.shell.close_chat')} onClick={() => setChatOverlayOpen(false)} /><div className="simple-project-overlay__panel"><ChatArea projectUuid={projectUuid} pictureBook={project?.picture_book} expanded overlay onToggle={() => setChatOverlayOpen(false)} newThreadReference={chatReference} /></div></div> : null}
    </main>
  )
}

function SimpleProjectTopbar({ pageTitle, project, projectUuid, search, chatOpen, onToggleChat, onOpenProjectConfiguration, onSwitchWorkspaceMode, onOpenNavigation }) {
  const { t } = useI18n()
  const menuId = useId()
  const menuRef = useRef(null)
  const triggerRef = useRef(null)
  const [projectActionsOpen, setProjectActionsOpen] = useState(false)

  useEffect(() => {
    if (!projectActionsOpen) return undefined
    const focusFrame = window.requestAnimationFrame(() => menuRef.current?.querySelector('[role="menuitem"]')?.focus())
    const handlePointerDown = (event) => {
      if (!menuRef.current?.contains(event.target)) setProjectActionsOpen(false)
    }
    const handleKeyDown = (event) => {
      if (event.key !== 'Escape') return
      setProjectActionsOpen(false)
      triggerRef.current?.focus()
    }
    document.addEventListener('pointerdown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      window.cancelAnimationFrame(focusFrame)
      document.removeEventListener('pointerdown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [projectActionsOpen])

  const closeAndRun = (action) => {
    setProjectActionsOpen(false)
    action()
  }

  const handleMenuKeyDown = (event) => {
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    const items = [...event.currentTarget.querySelectorAll('[role="menuitem"]')]
    if (!items.length) return
    event.preventDefault()
    const currentIndex = items.indexOf(document.activeElement)
    if (event.key === 'Home') items[0].focus()
    else if (event.key === 'End') items.at(-1).focus()
    else if (event.key === 'ArrowDown') items[(currentIndex + 1 + items.length) % items.length].focus()
    else items[(currentIndex - 1 + items.length) % items.length].focus()
  }

  return (
    <header className="simple-project-topbar">
      <button type="button" className="simple-project-topbar__menu" aria-label={t('common.navigation.open')} onClick={onOpenNavigation}>
        <Menu size={16} strokeWidth={1.6} aria-hidden="true" />
      </button>
      <div className="simple-project-topbar__context">
        <strong data-no-i18n>{project?.name || t('projects.fallback_name')}</strong>
        <span>{pageTitle}</span>
      </div>
      <div className="simple-project-topbar__actions">
        <button type="button" aria-label={t(chatOpen ? 'simple.shell.close_chat' : 'simple.shell.open_chat')} aria-expanded={chatOpen} onClick={onToggleChat}>
          <MessageCircle size={16} strokeWidth={1.6} aria-hidden="true" />
        </button>
        <div className="simple-project-topbar__more" ref={menuRef}>
          <button
            ref={triggerRef}
            type="button"
            aria-label={t('simple.shell.more_project_actions')}
            aria-haspopup="menu"
            aria-expanded={projectActionsOpen}
            aria-controls={projectActionsOpen ? menuId : undefined}
            onClick={() => setProjectActionsOpen((open) => !open)}
          >
            <MoreHorizontal size={16} strokeWidth={1.6} aria-hidden="true" />
          </button>
          {projectActionsOpen ? (
            <div className="simple-project-topbar__dropdown" id={menuId} role="menu" aria-label={t('simple.shell.more_project_actions')} onKeyDown={handleMenuKeyDown}>
              <button type="button" role="menuitem" onClick={() => closeAndRun(onOpenProjectConfiguration)}><Settings size={16} aria-hidden="true" /><span>{t('projects.configuration')}</span></button>
              <button type="button" role="menuitem" onClick={() => closeAndRun(onSwitchWorkspaceMode)}><LayoutDashboard size={16} aria-hidden="true" /><span>{t('simple.shell.switch_workspace_mode')}</span></button>
              <Link role="menuitem" to={projectRoute(projectUuid, 'llm-logs', search)} onClick={() => setProjectActionsOpen(false)}><Activity size={16} aria-hidden="true" /><span>{t('settings.llm_logs')}</span></Link>
              <Link role="menuitem" to={projectRoute(projectUuid, 'exports', search)} onClick={() => setProjectActionsOpen(false)}><Download size={16} aria-hidden="true" /><span>{t('projects.tab.exports')}</span></Link>
            </div>
          ) : null}
        </div>
      </div>
    </header>
  )
}

function hasChatControl(search) {
  const params = new URLSearchParams(search)
  return Boolean(params.get('chat_thread_uuid') || params.get('workflow_uuid') || params.get('chat_new') === '1' || params.get('chat_reference_uuid'))
}

function readChatCollapsed(projectUuid) {
  try { return window.localStorage.getItem(`${SIMPLE_CHAT_KEY}.${projectUuid}`) === 'true' } catch { return false }
}

function readCompact() {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function' && window.matchMedia(COMPACT_QUERY).matches
}
