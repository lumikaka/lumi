import { useEffect, useId, useRef, useState } from 'react'
import {
  ChevronLeft,
  ChevronRight,
  CircleHelp,
  FolderKanban,
  Home,
  PanelTop,
  Settings,
  X,
} from 'lucide-react'
import { Link, useLocation } from 'react-router-dom'

import { useI18n } from '../i18n/useI18n.js'

export const GLOBAL_SIDEBAR_COLLAPSED_KEY = 'lumi.globalSidebarCollapsed'

const PRIMARY_ITEMS = [
  { labelKey: 'projects.title', to: '/', icon: Home },
]

const UTILITY_ITEMS = [
  { labelKey: 'settings.ai', to: '/settings/providers', icon: Settings },
  { labelKey: 'settings.about', to: '/about', icon: CircleHelp },
  { labelKey: 'settings.admin', to: '/admin', icon: PanelTop, external: true },
]

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
  onSwitchProject,
}) {
  const { t } = useI18n()
  const location = useLocation()
  const titleId = useId()
  const closeRef = useRef(null)

  useEffect(() => {
    onClose?.()
  }, [location.pathname]) // eslint-disable-line react-hooks/exhaustive-deps

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
          <Link className="global-sidebar__brand" to="/" title="Lumi">
            <img src="/favicon.png" alt="" aria-hidden="true" />
            <span id={titleId}>Lumi</span>
          </Link>
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
          {PRIMARY_ITEMS.map((item) => (
            <SidebarLink key={item.to} item={item} label={t(item.labelKey)} active={isActivePath(location.pathname, item.to)} />
          ))}
        </nav>

        <section className="global-sidebar__recent" aria-labelledby={`${titleId}-recent`}>
          <h2 id={`${titleId}-recent`}>{t('projects.recent')}</h2>
          <nav aria-label={t('projects.recent')}>
            {recentProjects.slice(0, 6).map((project) => (
              <RecentProjectLink key={project.uuid} project={project} onSwitchProject={onSwitchProject} t={t} />
            ))}
          </nav>
        </section>

        <nav className="global-sidebar__nav global-sidebar__nav--utility" aria-label={t('common.navigation.main')}>
          {UTILITY_ITEMS.map((item) => (
            <SidebarLink key={item.to} item={item} label={t(item.labelKey)} active={isActivePath(location.pathname, item.to)} />
          ))}
        </nav>

        <button
          className="global-sidebar__collapse"
          type="button"
          aria-label={collapsed ? t('common.navigation.open_lumi') : t('common.navigation.close')}
          aria-pressed={collapsed}
          onClick={onToggleCollapsed}
        >
          {collapsed ? <ChevronRight size={17} aria-hidden="true" /> : <ChevronLeft size={17} aria-hidden="true" />}
          <span>{t('common.navigation.close')}</span>
        </button>
      </aside>
    </>
  )
}

function SidebarLink({ item, label, active }) {
  const Icon = item.icon
  const className = `global-sidebar__link ${active ? 'is-active' : ''}`
  const content = <><Icon size={17} aria-hidden="true" /><span>{label}</span></>
  if (item.external) return <a className={className} href={item.to} title={label}>{content}</a>
  return <Link className={className} to={item.to} title={label}>{content}</Link>
}

function RecentProjectLink({ project, onSwitchProject, t }) {
  const content = (
    <>
      <FolderKanban size={16} aria-hidden="true" />
      <span className="global-sidebar__project-copy">
        <span data-no-i18n>{project.name}</span>
        <small>{t(project.open ? 'projects.open' : project.available ? 'projects.recent_used' : 'projects.relocate_required')}</small>
      </span>
    </>
  )
  if (!project.available) {
    return <Link className="global-sidebar__link global-sidebar__project" to="/" title={project.root_path}>{content}</Link>
  }
  return (
    <button
      className="global-sidebar__link global-sidebar__project"
      type="button"
      title={project.root_path}
      onClick={() => onSwitchProject?.(project.uuid)}
    >
      {content}
    </button>
  )
}

function isActivePath(pathname, target) {
  if (target === '/') return pathname === '/' || pathname.startsWith('/projects/')
  return pathname === target || pathname.startsWith(`${target}/`)
}

function readCollapsed() {
  if (typeof window === 'undefined') return false
  try {
    return window.localStorage.getItem(GLOBAL_SIDEBAR_COLLAPSED_KEY) === 'true'
  } catch {
    return false
  }
}
