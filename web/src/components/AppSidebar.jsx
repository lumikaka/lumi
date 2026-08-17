import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useLocation, useNavigate } from 'react-router-dom'

import expandIcon from '../assets/figma/workspace/expand.svg'
import folderIcon from '../assets/figma/workspace/folder.svg'
import libraryIcon from '../assets/figma/workspace/library.svg'
import logoAsset from '../assets/figma/workspace/lumi-logo.svg'
import moreIcon from '../assets/figma/workspace/more.svg'
import newChatIcon from '../assets/figma/workspace/new-chat.svg'
import projectAddIcon from '../assets/figma/workspace/project-add.svg'
import searchIcon from '../assets/figma/workspace/search.svg'
import sidebarCollapseIcon from '../assets/figma/workspace/sidebar-collapse.svg'
import { listProviders } from '../api/ai.js'
import { archiveChatThread, listChatThreads } from '../api/chat.js'
import { forgetRecentProject, listRecentProjects, openRecentProjectFolder } from '../api/projects.js'
import { projectQueryKeys } from '../api/projectQueryKeys.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import {
  matchingThreads,
  newConversationPath,
  orderedProjects,
  orderedThreads,
  providerConnectionState,
  shouldShowThreadToggle,
  visibleThreads,
} from './sidebarNavigation.js'
import { ConversationRow, SidebarAccountAction, SidebarActionRow, SidebarProviderAction } from './SidebarRows.jsx'
import FigmaIcon from './FigmaIcon.jsx'
import { markThreadRead, readPinnedProjects, readPinnedThreads, readThreadReadAt, threadReadState, writePinnedProjects, writePinnedThreads } from './sidebarPreferences.js'

const SETTINGS_ITEMS = [
  { labelKey: 'settings.account', to: '/settings/account' },
  { labelKey: 'settings.llm_management', to: '/settings/providers' },
  { labelKey: 'settings.llm_logs', to: '/settings/llm-logs' },
  { labelKey: 'settings.about', to: '/about' },
  { labelKey: 'settings.admin', to: '/admin', external: true },
]

export default function AppSidebar({ collapsed, onToggle }) {
  const { t } = useI18n()
  const location = useLocation()
  const settingsRef = useRef(null)
  const searchInputRef = useRef(null)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [pinnedProjects, setPinnedProjects] = useState(readPinnedProjects)
  const projectsQuery = useQuery({ queryKey: projectQueryKeys.recent(), queryFn: listRecentProjects, retry: false })
  const providersQuery = useQuery({ queryKey: ['providers'], queryFn: listProviders, retry: false })
  const activeProjectUuid = projectUuidFromPath(location.pathname)
  const recentProjects = projectsQuery.data?.items || []
  const projects = useMemo(() => orderedProjects(recentProjects, pinnedProjects), [pinnedProjects, recentProjects])
  const newThreadPath = newConversationPath(projects, activeProjectUuid)
  const libraryPath = activeProjectUuid ? `/projects/${encodeURIComponent(activeProjectUuid)}/comic` : '/'
  const providerState = providersQuery.isLoading ? 'ready' : providerConnectionState(providersQuery.data?.items)

  useEffect(() => setSettingsOpen(false), [location.hash, location.pathname])

  useEffect(() => {
    if (!searchOpen || collapsed) return
    searchInputRef.current?.focus()
  }, [collapsed, searchOpen])

  useEffect(() => {
    if (!settingsOpen) return undefined
    const close = (event) => {
      if (event.key === 'Escape' || (event.type === 'pointerdown' && !settingsRef.current?.contains(event.target))) setSettingsOpen(false)
    }
    document.addEventListener('pointerdown', close)
    document.addEventListener('keydown', close)
    return () => {
      document.removeEventListener('pointerdown', close)
      document.removeEventListener('keydown', close)
    }
  }, [settingsOpen])

  const openSearch = () => {
    setSearchOpen(true)
    if (collapsed) onToggle()
  }
  const closeSearch = () => {
    setSearch('')
    setSearchOpen(false)
  }
  const togglePinnedProject = (projectUuid) => {
    const next = pinnedProjects.includes(projectUuid) ? pinnedProjects.filter((uuid) => uuid !== projectUuid) : [projectUuid, ...pinnedProjects]
    setPinnedProjects(next)
    writePinnedProjects(next)
  }
  const forgetPinnedProject = (projectUuid) => {
    if (!pinnedProjects.includes(projectUuid)) return
    const next = pinnedProjects.filter((uuid) => uuid !== projectUuid)
    setPinnedProjects(next)
    writePinnedProjects(next)
  }

  return (
    <aside className="app-sidebar" aria-label={t('sidebar.navigation')}>
      <header className="app-sidebar__header">
        <Link className="app-sidebar__brand" to="/" aria-label="Lumi" title="Lumi">
          <img src={logoAsset} alt="Lumi" width="53.143" height="20" />
        </Link>
        <button className="app-sidebar__toggle" type="button" onClick={onToggle} aria-label={t(collapsed ? 'sidebar.expand' : 'sidebar.collapse')} aria-expanded={!collapsed} title={t(collapsed ? 'sidebar.expand' : 'sidebar.collapse')}>
          <FigmaIcon className={collapsed ? 'is-reversed' : ''} src={sidebarCollapseIcon} size={16} />
        </button>
      </header>

      <div className="app-sidebar__body">
        {collapsed ? (
          <>
            <Link className="app-sidebar__compact-action" to={newThreadPath} aria-label={t('sidebar.new_conversation')} title={t('sidebar.new_conversation')}><FigmaIcon src={newChatIcon} size={16} /></Link>
            <Link className="app-sidebar__compact-action" to={libraryPath} aria-label={t('sidebar.library')} title={t('sidebar.library')}><FigmaIcon src={libraryIcon} size={16} /></Link>
            <button className="app-sidebar__compact-action" type="button" aria-label={t('sidebar.search.label')} title={t('sidebar.search.label')} onClick={openSearch}><FigmaIcon src={searchIcon} size={16} /></button>
          </>
        ) : (
          <>
            {searchOpen ? (
              <label className="app-sidebar-search">
                <FigmaIcon src={searchIcon} size={16} />
                <input ref={searchInputRef} type="search" value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t('sidebar.search.placeholder')} aria-label={t('sidebar.search.label')} />
                <button type="button" aria-label={t('sidebar.search.close')} onClick={closeSearch}>{t('common.action.cancel')}</button>
              </label>
            ) : (
              <div className="app-sidebar__primary-actions">
                <SidebarActionRow iconSrc={newChatIcon} label={t('sidebar.new_conversation')} to={newThreadPath} />
                <SidebarActionRow iconSrc={libraryIcon} label={t('sidebar.library')} to={libraryPath} />
                <SidebarActionRow className="app-sidebar__search-row" iconSrc={searchIcon} label={t('sidebar.search.label')} onClick={openSearch} />
              </div>
            )}

            <section className="app-sidebar__projects" aria-labelledby="sidebar-projects-title">
              <div className="app-sidebar__section-heading">
                <h2 id="sidebar-projects-title">{t('sidebar.projects')}</h2>
                <Link className="app-sidebar__create-project" to="/?create_project=1" aria-label={t('sidebar.project.create')} title={t('sidebar.project.create')}><FigmaIcon src={projectAddIcon} size={16} /></Link>
              </div>
              {projectsQuery.isLoading ? <p className="app-sidebar__message">{t('projects.loading.index')}</p> : null}
              {!projectsQuery.isLoading && projects.length === 0 ? <p className="app-sidebar__message">{t('sidebar.projects.empty')}</p> : null}
              {projects.map((project, index) => (
                <ProjectTreeGroup
                  key={project.uuid}
                  project={project}
                  projectCount={projects.length}
                  initiallyExpanded={projects.length === 1 || project.uuid === activeProjectUuid || index === 0}
                  active={project.uuid === activeProjectUuid}
                  location={location}
                  search={search}
                  pinnedProject={pinnedProjects.includes(project.uuid)}
                  onTogglePinnedProject={() => togglePinnedProject(project.uuid)}
                  onForgetProject={() => forgetPinnedProject(project.uuid)}
                  t={t}
                />
              ))}
            </section>
          </>
        )}
      </div>

      <div className="app-sidebar__footer" ref={settingsRef}>
        {!collapsed ? <SidebarProviderAction state={providerState} t={t} /> : null}
        {settingsOpen ? (
          <div className="app-sidebar-settings" role="menu" aria-label={t('settings.account_and_settings')}>
            <div className="app-sidebar-settings__identity"><strong>{t('settings.local_account')}</strong></div>
            {SETTINGS_ITEMS.map((item) => <SettingsLink key={item.to} item={item} t={t} />)}
          </div>
        ) : null}
        <SidebarAccountAction collapsed={collapsed} open={settingsOpen} onClick={() => setSettingsOpen((value) => !value)} t={t} />
      </div>
    </aside>
  )
}

export function ProjectTreeGroup({ project, projectCount, initiallyExpanded, active, location, search, pinnedProject, onTogglePinnedProject, onForgetProject, t }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const menuRef = useRef(null)
  const [menuOpen, setMenuOpen] = useState(false)
  const [expanded, setExpanded] = useState(initiallyExpanded)
  const [showAllThreads, setShowAllThreads] = useState(false)
  const [pinned, setPinned] = useState(readPinnedThreads)
  const [readAt, setReadAt] = useState(readThreadReadAt)
  const searching = Boolean(search.trim())
  const threadsQuery = useQuery({
    queryKey: ['chat-threads', project.uuid, 'project', 'sidebar'],
    queryFn: () => listChatThreads(project.uuid, { scope: 'project', page: 1, perPage: 30 }),
    enabled: Boolean(expanded || searching || active),
    retry: false,
  })
  const archiveThreadMutation = useMutation({
    mutationFn: (threadUuid) => archiveChatThread(project.uuid, threadUuid),
    onSuccess: (_, threadUuid) => {
      const nextPinned = pinned.filter((uuid) => uuid !== threadUuid)
      setPinned(nextPinned)
      writePinnedThreads(nextPinned)
      queryClient.invalidateQueries({ queryKey: ['chat-threads', project.uuid] })
      const activeThreadUuid = active ? new URLSearchParams(location.search).get('chat_thread_uuid') : ''
      if (activeThreadUuid === threadUuid) navigate(`/projects/${encodeURIComponent(project.uuid)}/premise`)
    },
  })
  const archiveProjectMutation = useMutation({
    mutationFn: () => forgetRecentProject(project.uuid),
    onSuccess: () => {
      setMenuOpen(false)
      onForgetProject?.()
      queryClient.invalidateQueries({ queryKey: projectQueryKeys.recent() })
      if (active) navigate('/')
    },
  })
  const openFolderMutation = useMutation({
    mutationFn: () => openRecentProjectFolder(project.uuid),
    onSuccess: () => setMenuOpen(false),
  })
  const projectBase = `/projects/${encodeURIComponent(project.uuid)}`
  const activeThreadUuid = active ? new URLSearchParams(location.search).get('chat_thread_uuid') : ''
  const needle = search.trim().toLocaleLowerCase()
  const threads = useMemo(() => matchingThreads(orderedThreads(threadsQuery.data?.items || [], pinned), search), [pinned, search, threadsQuery.data])
  const visible = !needle || threads.length > 0
  const treeOpen = expanded || searching
  const displayedThreads = visibleThreads(threads, { projectCount, expanded: showAllThreads, searching })
  const showThreadToggle = shouldShowThreadToggle(threads, { projectCount, searching })

  useEffect(() => {
    if (!activeThreadUuid) return
    setReadAt(markThreadRead(activeThreadUuid))
  }, [activeThreadUuid])

  useEffect(() => {
    if (active) setExpanded(true)
  }, [active])

  useEffect(() => {
    if (!menuOpen) return undefined
    const close = (event) => {
      if (event.key === 'Escape' || (event.type === 'pointerdown' && !menuRef.current?.contains(event.target))) setMenuOpen(false)
    }
    document.addEventListener('pointerdown', close)
    document.addEventListener('keydown', close)
    return () => {
      document.removeEventListener('pointerdown', close)
      document.removeEventListener('keydown', close)
    }
  }, [menuOpen])

  if (!visible) return null

  const togglePinned = (threadUuid) => {
    const next = pinned.includes(threadUuid) ? pinned.filter((uuid) => uuid !== threadUuid) : [threadUuid, ...pinned]
    setPinned(next)
    writePinnedThreads(next)
  }
  const toggleProject = () => {
    setExpanded((value) => !value)
    if (expanded) setShowAllThreads(false)
  }

  return (
    <div className={`app-sidebar-project${active ? ' is-active' : ''}`}>
      <div className="app-sidebar-project__row" ref={menuRef}>
        <button className="app-sidebar-project__main" type="button" aria-expanded={treeOpen} onClick={toggleProject} title={project.name}>
          <FigmaIcon src={folderIcon} size={16} /><span data-no-i18n>{project.name}</span>
        </button>
        <div className="app-sidebar-project__actions">
          <Link className="app-sidebar-project__new-chat" to={`${projectBase}/premise?chat_scope=project&chat_new=1`} aria-label={t('sidebar.project.new_conversation', { name: project.name })} title={t('sidebar.project.new_conversation', { name: project.name })}><FigmaIcon src={projectAddIcon} size={16} /></Link>
          <Link className="app-sidebar-project__enter" to={`${projectBase}/overview/summary`} aria-label={t('sidebar.project.enter', { name: project.name })}>{t('sidebar.project.enter_short')}</Link>
          <button type="button" aria-label={t('projects.details_for', { name: project.name })} title={t('common.action.more')} aria-haspopup="menu" aria-expanded={menuOpen} onClick={() => { openFolderMutation.reset(); setMenuOpen((value) => !value) }}><FigmaIcon src={moreIcon} size={16} /></button>
        </div>
        {menuOpen ? (
          <div className="app-sidebar-project__menu" role="menu" aria-label={t('projects.details_for', { name: project.name })}>
            <Link role="menuitem" to={`${projectBase}/overview/summary`} onClick={() => setMenuOpen(false)}><span>{t('projects.project_settings')}</span></Link>
            <button role="menuitem" type="button" disabled={openFolderMutation.isPending} onClick={() => openFolderMutation.mutate()}><span>{t(openFolderMutation.isPending ? 'sidebar.project.opening_folder' : 'sidebar.project.open_folder')}</span></button>
            <button role="menuitem" type="button" aria-pressed={pinnedProject} onClick={() => { onTogglePinnedProject?.(); setMenuOpen(false) }}><span>{t(pinnedProject ? 'sidebar.project.unpin' : 'sidebar.project.pin')}</span></button>
            <button className="is-danger" role="menuitem" type="button" disabled={archiveProjectMutation.isPending} onClick={() => window.confirm(t('sidebar.project.archive_confirm', { name: project.name })) && archiveProjectMutation.mutate()}><span>{t(archiveProjectMutation.isPending ? 'sidebar.project.archiving' : 'sidebar.project.archive')}</span></button>
            <LocalizedErrorMessage error={openFolderMutation.error} className="app-sidebar-project__menu-error" compact />
          </div>
        ) : null}
      </div>
      {treeOpen ? (
        <div className="app-sidebar-project__threads">
          {threadsQuery.isLoading ? <span className="app-sidebar-project__loading">{t('chat.thread.loading')}</span> : null}
          {displayedThreads.map((thread) => {
            const status = threadReadState(thread, readAt)
            const title = thread.title || t('chat.threads')
            return (
              <ConversationRow
                key={thread.uuid}
                active={activeThreadUuid === thread.uuid}
                href={`${projectBase}/premise?chat_scope=project&chat_thread_uuid=${encodeURIComponent(thread.uuid)}`}
                pinned={pinned.includes(thread.uuid)}
                status={status}
                statusLabel={t(`sidebar.thread.status.${status}`)}
                title={title}
                archiveDisabled={archiveThreadMutation.isPending}
                onTogglePinned={() => togglePinned(thread.uuid)}
                onArchive={() => archiveThreadMutation.mutate(thread.uuid)}
                t={t}
              />
            )
          })}
          {showThreadToggle ? (
            <button className="app-sidebar-project__thread-toggle" type="button" aria-expanded={showAllThreads} onClick={() => setShowAllThreads((value) => !value)}>
              {t(showAllThreads ? 'sidebar.threads.collapse' : 'sidebar.threads.expand')}<FigmaIcon className={showAllThreads ? 'is-reversed' : ''} src={expandIcon} size={12} />
            </button>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

function SettingsLink({ item, t }) {
  const content = <span>{t(item.labelKey)}</span>
  if (item.external) return <a href={item.to} role="menuitem">{content}</a>
  return <Link to={item.to} role="menuitem">{content}</Link>
}

function projectUuidFromPath(pathname) {
  const match = pathname.match(/^\/projects\/([^/]+)/)
  if (!match) return ''
  try { return decodeURIComponent(match[1]) } catch { return '' }
}
