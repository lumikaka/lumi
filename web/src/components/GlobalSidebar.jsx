import { useEffect, useId, useRef, useState } from 'react'
import {
  Activity,
  Cpu,
  House,
  Languages,
  MessageSquareWarning,
  PanelLeftClose,
  Plus,
  Search,
  Settings,
  User,
  X,
} from 'lucide-react'
import { Link, useLocation } from 'react-router-dom'

import { useI18n } from '../i18n/useI18n.js'
import ProjectSearchDialog from './ProjectSearchDialog.jsx'
import { mergeSidebarProjectOrder, orderSidebarProjects } from './sidebarProjectOrder.js'

export const GLOBAL_SIDEBAR_COLLAPSED_KEY = 'lumi.globalSidebarCollapsed'
export const GLOBAL_SIDEBAR_PROJECT_ORDER_KEY = 'lumi.globalSidebarProjectOrder'

const SETTINGS_ITEMS = [
  { labelKey: 'settings.user_account', to: '/settings/account', icon: User },
  { labelKey: 'settings.language_preference', to: '/settings/account#language', icon: Languages },
  { labelKey: 'settings.model_configuration', to: '/settings/providers', icon: Cpu },
  { labelKey: 'settings.llm_calls', to: '/settings/llm-logs', icon: Activity },
  { labelKey: 'settings.feedback', to: '/about', icon: MessageSquareWarning },
]

const PROJECT_TONES = ['warm', 'accent', 'muted', 'warm', 'accent', 'muted']

export function useGlobalSidebarState() {
  const [collapsed, setCollapsed] = useState(readCollapsed)

  useEffect(() => {
    try {
      window.localStorage.setItem(GLOBAL_SIDEBAR_COLLAPSED_KEY, collapsed ? 'true' : 'false')
    } catch {
      // Storage may be unavailable in restricted browsers; the visual state still works.
    }
  }, [collapsed])

  return [collapsed, setCollapsed]
}

export default function GlobalSidebar({
  collapsed,
  mobileOpen,
  onClose,
  onToggleCollapsed,
  recentProjects = [],
  recentProjectsLoading = false,
  onSwitchProject,
}) {
  const { t } = useI18n()
  const location = useLocation()
  const titleId = useId()
  const searchDialogId = useId()
  const settingsMenuId = useId()
  const closeRef = useRef(null)
  const searchTriggerRef = useRef(null)
  const settingsRef = useRef(null)
  const settingsTriggerRef = useRef(null)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const stableRecentProjects = useStableRecentProjects(recentProjects)

  useEffect(() => {
    setSettingsOpen(false)
    setSearchOpen(false)
    onClose?.()
  }, [location.hash, location.pathname, location.search]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!settingsOpen) return undefined
    const handlePointerDown = (event) => {
      if (!settingsRef.current?.contains(event.target)) setSettingsOpen(false)
    }
    const handleKeyDown = (event) => {
      if (event.key !== 'Escape') return
      setSettingsOpen(false)
      settingsTriggerRef.current?.focus()
    }
    document.addEventListener('pointerdown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [settingsOpen])

  useEffect(() => {
    if (!mobileOpen) return undefined
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    closeRef.current?.focus()
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') onClose?.()
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      document.body.style.overflow = previousOverflow
      window.removeEventListener('keydown', handleKeyDown)
    }
  }, [mobileOpen, onClose])

  const settingsActive = location.pathname.startsWith('/settings/') || location.pathname === '/about'
  const currentProjectUuid = stableRecentProjects.find((project) => isProjectActive(location.pathname, project.uuid))?.uuid || ''

  const closeSearch = () => {
    setSearchOpen(false)
    window.requestAnimationFrame(() => searchTriggerRef.current?.focus())
  }

  const openSearch = () => {
    setSettingsOpen(false)
    setSearchOpen(true)
    onClose?.()
  }

  return (
    <>
      {mobileOpen ? (
        <button
          className="global-sidebar__backdrop"
          type="button"
          aria-label={t('common.navigation.close')}
          onClick={onClose}
        />
      ) : null}
      <aside
        className={`global-sidebar ${collapsed ? 'is-collapsed' : ''} ${mobileOpen ? 'is-mobile-open' : ''}`}
        aria-labelledby={titleId}
      >
        <header className="global-sidebar__header">
          <button
            className="global-sidebar__brand"
            type="button"
            aria-label={collapsed ? t('common.navigation.open_lumi') : t('common.navigation.close')}
            aria-pressed={collapsed}
            title={collapsed ? t('common.navigation.open_lumi') : t('common.navigation.close')}
            onClick={onToggleCollapsed}
          >
            <img className="global-sidebar__logo" src="/favicon.png" alt="" aria-hidden="true" />
            <span className="global-sidebar__brand-name" id={titleId}>Lumi</span>
          </button>
          <button
            ref={closeRef}
            className="global-sidebar__mobile-close"
            type="button"
            aria-label={t('common.navigation.close')}
            onClick={onClose}
          >
            <X size={18} aria-hidden="true" />
          </button>
        </header>

        <nav className="global-sidebar__nav" aria-label={t('common.navigation.main')}>
          <SidebarLink
            to="/"
            label={t('common.navigation.home')}
            icon={House}
            active={location.pathname === '/'}
          />
          <button
            ref={searchTriggerRef}
            className={`global-sidebar__link global-sidebar__search ${searchOpen ? 'is-active' : ''}`}
            type="button"
            aria-haspopup="dialog"
            aria-expanded={searchOpen}
            aria-controls={searchOpen ? searchDialogId : undefined}
            title={t('common.action.search')}
            onClick={openSearch}
          >
            <Search size={16} aria-hidden="true" />
            <span>{t('common.action.search')}</span>
          </button>
        </nav>

        <section className="global-sidebar__recent" aria-labelledby={`${titleId}-projects`}>
          <header className="global-sidebar__section-header">
            <h2 id={`${titleId}-projects`}>{t('projects.title')}</h2>
            <Link className="global-sidebar__add-project" to="/?create_project=1" aria-label={t('projects.action.new')}>
              <Plus size={16} aria-hidden="true" />
            </Link>
          </header>
          <nav aria-label={t('projects.title')}>
            {stableRecentProjects.slice(0, 6).map((project, index) => (
              <RecentProjectLink
                key={project.uuid}
                active={isProjectActive(location.pathname, project.uuid)}
                project={project}
                tone={PROJECT_TONES[index]}
                onSwitchProject={onSwitchProject}
              />
            ))}
          </nav>
        </section>

        <footer className="global-sidebar__footer" ref={settingsRef}>
          <button
            ref={settingsTriggerRef}
            className={`global-sidebar__link global-sidebar__settings ${settingsActive ? 'is-active' : ''}`}
            type="button"
            aria-haspopup="menu"
            aria-expanded={settingsOpen}
            aria-controls={settingsOpen ? settingsMenuId : undefined}
            title={t('settings.title')}
            onClick={() => setSettingsOpen((current) => !current)}
          >
            <Settings size={16} aria-hidden="true" />
            <span>{t('settings.title')}</span>
          </button>

          {settingsOpen ? (
            <nav className="global-sidebar__settings-menu" id={settingsMenuId} role="menu" aria-label={t('settings.menu')}>
              {SETTINGS_ITEMS.map((item) => {
                const Icon = item.icon
                return (
                  <Link className="global-sidebar__menu-item" key={`${item.to}-${item.labelKey}`} role="menuitem" to={item.to}>
                    <Icon size={16} aria-hidden="true" />
                    <span>{t(item.labelKey)}</span>
                  </Link>
                )
              })}
            </nav>
          ) : null}

          <button
            className="global-sidebar__collapse"
            type="button"
            aria-label={collapsed ? t('common.navigation.open_lumi') : t('common.navigation.close')}
            aria-pressed={collapsed}
            title={collapsed ? t('common.navigation.open_lumi') : t('common.navigation.close')}
            onClick={onToggleCollapsed}
          >
            <PanelLeftClose size={16} aria-hidden="true" />
            <span>{t('common.navigation.collapse')}</span>
          </button>
        </footer>
      </aside>
      {searchOpen ? (
        <ProjectSearchDialog
          id={searchDialogId}
          currentProjectUuid={currentProjectUuid}
          loading={recentProjectsLoading}
          projects={stableRecentProjects}
          onClose={closeSearch}
          onSwitchProject={onSwitchProject}
        />
      ) : null}
    </>
  )
}

function SidebarLink({ active, icon: Icon, label, to }) {
  return (
    <Link className={`global-sidebar__link ${active ? 'is-active' : ''}`} to={to} title={label} aria-current={active ? 'page' : undefined}>
      <Icon size={16} aria-hidden="true" />
      <span>{label}</span>
    </Link>
  )
}

function RecentProjectLink({ active, project, onSwitchProject, tone }) {
  const content = (
    <>
      <span className={`global-sidebar__project-mark is-${tone}`} aria-hidden="true">{project.name?.trim().charAt(0) || '·'}</span>
      <span className="global-sidebar__project-copy" data-no-i18n>{project.name}</span>
    </>
  )
  const className = `global-sidebar__link global-sidebar__project ${active ? 'is-active' : ''}`
  if (!project.available) {
    return <Link className={className} to="/" title={project.name} aria-current={active ? 'page' : undefined}>{content}</Link>
  }
  return (
    <button
      className={className}
      type="button"
      title={project.name}
      aria-current={active ? 'page' : undefined}
      onClick={() => onSwitchProject?.(project.uuid)}
    >
      {content}
    </button>
  )
}

function isProjectActive(pathname, projectUuid) {
  return pathname === `/projects/${encodeURIComponent(projectUuid)}` || pathname.startsWith(`/projects/${encodeURIComponent(projectUuid)}/`)
}

function useStableRecentProjects(recentProjects) {
  const [projectOrder, setProjectOrder] = useState(readProjectOrder)
  const nextProjectOrder = mergeSidebarProjectOrder(projectOrder, recentProjects)

  useEffect(() => {
    if (nextProjectOrder === projectOrder) return
    setProjectOrder(nextProjectOrder)
    try {
      window.sessionStorage.setItem(GLOBAL_SIDEBAR_PROJECT_ORDER_KEY, JSON.stringify(nextProjectOrder))
    } catch {
      // Storage may be unavailable; the order still remains stable until unmount.
    }
  }, [nextProjectOrder, projectOrder])

  return orderSidebarProjects(recentProjects, nextProjectOrder)
}

function readProjectOrder() {
  if (typeof window === 'undefined') return []
  try {
    const value = JSON.parse(window.sessionStorage.getItem(GLOBAL_SIDEBAR_PROJECT_ORDER_KEY) || '[]')
    if (!Array.isArray(value)) return []
    return [...new Set(value.filter((uuid) => typeof uuid === 'string' && uuid))]
  } catch {
    return []
  }
}

function readCollapsed() {
  if (typeof window === 'undefined') return false
  try {
    return window.localStorage.getItem(GLOBAL_SIDEBAR_COLLAPSED_KEY) === 'true'
  } catch {
    return false
  }
}
