import { useEffect, useRef, useState } from 'react'
import { ChevronDown, FolderKanban, Menu } from 'lucide-react'
import { Link, useLocation } from 'react-router-dom'

import { WORKSPACE_SECTIONS, workspaceRoute } from './workspaceNavigation.js'
import AccountMenu from './AccountMenu.jsx'
import { useI18n } from '../i18n/useI18n.js'

export default function GlobalTopbar({
  title,
  projectUuid,
  activeSection,
  actions,
  recentProjects = [],
  onSwitchProject,
  onOpenNavigation,
  switchingProjectUuid = '',
}) {
  const { t } = useI18n()
  const location = useLocation()
  const switcherRef = useRef(null)
  const [switcherOpen, setSwitcherOpen] = useState(false)
  const availableProjects = recentProjects.filter((project) => project.available)

  useEffect(() => setSwitcherOpen(false), [location.pathname])

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
    <header className={`project-topbar ${projectUuid ? '' : 'project-topbar--index'}`}>
      <div className="project-topbar__context">
        <button
          className="project-logo"
          type="button"
          aria-label={t('common.navigation.open_lumi')}
          onClick={onOpenNavigation}
        >
          <Menu size={19} aria-hidden="true" />
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
                <ChevronDown size={15} aria-hidden="true" />
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
  )
}
