import { useEffect, useId, useRef, useState } from 'react'
import {
  Bot,
  ChevronDown,
  CircleHelp,
  FolderKanban,
  LayoutDashboard,
  LibraryBig,
  PanelTop,
  Settings,
  X,
} from 'lucide-react'
import { Link, useLocation } from 'react-router-dom'

import { WORKSPACE_SECTIONS, workspaceRoute } from './workspaceNavigation.js'
import AccountMenu from './AccountMenu.jsx'
import { useI18n } from '../i18n/useI18n.js'

const GLOBAL_ITEMS = [
  { labelKey: 'projects.title', to: '/', icon: LibraryBig },
  { labelKey: 'settings.ai', to: '/settings/providers', icon: Settings },
  { labelKey: 'settings.about', to: '/about', icon: CircleHelp },
  { labelKey: 'settings.admin', to: '/admin', icon: PanelTop, external: true },
]

const SECTION_ICONS = {
  overview: LayoutDashboard,
  premise: Bot,
  chapters: FolderKanban,
}

export default function GlobalTopbar({
  title,
  projectUuid,
  activeSection,
  actions,
  recentProjects = [],
  onSwitchProject,
  switchingProjectUuid = '',
}) {
  const { t } = useI18n()
  const location = useLocation()
  const drawerTitleId = useId()
  const drawerCloseRef = useRef(null)
  const switcherRef = useRef(null)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [switcherOpen, setSwitcherOpen] = useState(false)
  const availableProjects = recentProjects.filter((project) => project.available)

  useEffect(() => {
    setDrawerOpen(false)
    setSwitcherOpen(false)
  }, [location.pathname])

  useEffect(() => {
    if (!drawerOpen) return undefined
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    drawerCloseRef.current?.focus()
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') setDrawerOpen(false)
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      document.body.style.overflow = previousOverflow
      window.removeEventListener('keydown', handleKeyDown)
    }
  }, [drawerOpen])

  useEffect(() => {
    if (!switcherOpen) return undefined
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') setSwitcherOpen(false)
    }
    const handlePointerDown = (event) => {
      if (!switcherRef.current?.contains(event.target)) setSwitcherOpen(false)
    }
    window.addEventListener('keydown', handleKeyDown)
    document.addEventListener('mousedown', handlePointerDown)
    return () => {
      window.removeEventListener('keydown', handleKeyDown)
      document.removeEventListener('mousedown', handlePointerDown)
    }
  }, [switcherOpen])

  return (
    <>
      <header className={`project-topbar ${projectUuid ? '' : 'project-topbar--index'}`}>
        <div className="project-topbar__context">
          <button
            className="project-logo"
            type="button"
            aria-label={t('common.navigation.open_lumi')}
            aria-expanded={drawerOpen}
            onClick={() => setDrawerOpen(true)}
          >
            <span aria-hidden="true">L</span>
          </button>
          <div className="project-topbar__title" ref={switcherRef}>
            {projectUuid && availableProjects.length ? (
              <h1>
                <button
                  className="project-switcher-button"
                  type="button"
                  aria-haspopup="dialog"
                  aria-expanded={switcherOpen}
                  onClick={() => setSwitcherOpen((open) => !open)}
                >
                  <span data-no-i18n>{title}</span>
                  <ChevronDown size={16} aria-hidden="true" />
                </button>
              </h1>
            ) : projectUuid ? <h1 data-no-i18n>{title}</h1> : <h1>{title}</h1>}
            {switcherOpen ? (
              <div className="project-switcher-popover" role="dialog" aria-label={t('projects.open_switcher')}>
                {availableProjects.map((project) => (
                  <button
                    className={`project-switcher-option ${project.uuid === projectUuid ? 'is-active' : ''}`}
                    type="button"
                    key={project.uuid}
                    disabled={Boolean(switchingProjectUuid) || project.uuid === projectUuid}
                    onClick={() => onSwitchProject?.(project.uuid)}
                  >
                    <FolderKanban size={17} aria-hidden="true" />
                    <span className="project-switcher-option__copy">
                      <span className="project-switcher-option__title" data-no-i18n>{project.name}</span>
					  <span className="project-switcher-option__meta"><span>{t(project.open ? 'projects.open' : 'projects.recent_used')}</span> · <span data-no-i18n>{project.root_path}</span></span>
                    </span>
                  </button>
                ))}
              </div>
            ) : null}
          </div>
        </div>

        {projectUuid ? (
          <nav className="project-topbar__nav" aria-label={t('projects.navigation')}>
            {WORKSPACE_SECTIONS.map((section) => (
              <Link
                className={activeSection === section.key ? 'is-active' : ''}
                key={section.key}
                to={workspaceRoute(projectUuid, section.route, location.search)}
              >
                {t(section.labelKey)}
              </Link>
            ))}
          </nav>
        ) : null}

        <div className="project-topbar__actions">{actions}<AccountMenu /></div>
      </header>

      {drawerOpen ? (
        <div className="project-menu-drawer" role="dialog" aria-modal="true" aria-labelledby={drawerTitleId}>
          <button className="project-menu-drawer__backdrop" type="button" aria-label={t('common.navigation.close')} onClick={() => setDrawerOpen(false)} />
          <aside className="project-menu-drawer__panel">
            <header className="project-menu-drawer__header">
              <div className="project-menu-drawer__brand"><span aria-hidden="true">L</span><div><p>Lumi</p><h2 id={drawerTitleId}>{t('common.navigation.title')}</h2></div></div>
              <button ref={drawerCloseRef} className="project-menu-drawer__close" type="button" aria-label={t('common.navigation.close')} onClick={() => setDrawerOpen(false)}><X size={18} /></button>
            </header>
            <nav className="project-menu-drawer__nav" aria-label={t('common.navigation.main')}>
              {GLOBAL_ITEMS.map((item) => <DrawerLink key={item.to} item={item} label={t(item.labelKey)} active={isActivePath(location.pathname, item.to)} />)}
            </nav>
            {projectUuid ? (
              <section className="project-menu-drawer__section">
                <h3>{t('projects.current')}</h3>
                <nav className="project-menu-drawer__nav" aria-label={t('projects.group_navigation', { section: t('projects.current') })}>
                  {WORKSPACE_SECTIONS.map((section) => {
                    const Icon = SECTION_ICONS[section.key]
                    return <Link className={`project-menu-drawer__link ${activeSection === section.key ? 'is-active' : ''}`} key={section.key} to={workspaceRoute(projectUuid, section.route, location.search)}><Icon size={17} /><span>{t(section.labelKey)}</span></Link>
                  })}
                </nav>
              </section>
            ) : null}
            {recentProjects.length ? (
              <section className="project-menu-drawer__section">
                <h3>{t('projects.recent')}</h3>
                <nav className="project-menu-drawer__nav" aria-label={t('projects.recent')}>
                  {recentProjects.slice(0, 5).map((project) => (
                    <RecentProjectShortcut
                      key={project.uuid}
                      project={project}
                      onSwitchProject={onSwitchProject}
                      t={t}
                    />
                  ))}
                </nav>
              </section>
            ) : null}
          </aside>
        </div>
      ) : null}
    </>
  )
}

function RecentProjectShortcut({ project, onSwitchProject, t }) {
  const content = (
    <>
      <FolderKanban size={17} aria-hidden="true" />
      <span className="project-menu-drawer__project-copy">
        <span className="project-menu-drawer__project-title" data-no-i18n>{project.name}</span>
        <span className="project-menu-drawer__project-meta">
		  {t(project.open ? 'projects.open' : project.available ? 'projects.recent_used' : 'projects.relocate_required')}
        </span>
      </span>
    </>
  )

  if (!project.available) {
    return <Link className="project-menu-drawer__link project-menu-drawer__project-link" to="/" title={project.root_path}>{content}</Link>
  }

  return (
    <button
      className="project-menu-drawer__link project-menu-drawer__project-link"
      type="button"
      title={project.root_path}
      onClick={() => onSwitchProject?.(project.uuid)}
    >
      {content}
    </button>
  )
}

function DrawerLink({ item, label, active }) {
  const Icon = item.icon
  const className = `project-menu-drawer__link ${active ? 'is-active' : ''}`
  if (item.external) return <a className={className} href={item.to}><Icon size={17} /><span>{label}</span></a>
  return <Link className={className} to={item.to}><Icon size={17} /><span>{label}</span></Link>
}

function isActivePath(pathname, target) {
  if (target === '/') return pathname === '/' || pathname.startsWith('/projects/')
  return pathname === target || pathname.startsWith(`${target}/`)
}
