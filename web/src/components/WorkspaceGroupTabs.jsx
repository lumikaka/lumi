import { NavLink, useLocation } from 'react-router-dom'

import { WORKSPACE_GROUP_ITEMS, workspaceRoute } from './workspaceNavigation.js'
import { useI18n } from '../i18n/useI18n.js'

export default function WorkspaceGroupTabs({ projectUuid, activeSection }) {
  const { t } = useI18n()
  const location = useLocation()
  if (activeSection === 'premise' && location.pathname.endsWith('/premise')) return null
  if (activeSection === 'chapters' && (location.pathname.endsWith('/chapters') || /\/chapters\/[^/]+(?:\/preview)?$/.test(location.pathname))) return null
  const items = WORKSPACE_GROUP_ITEMS[activeSection] || WORKSPACE_GROUP_ITEMS.overview
  const overviewTabs = activeSection === 'overview'

  return (
    <nav className={`workspace-group-nav ${overviewTabs ? 'workspace-group-nav--overview' : ''}`} aria-label={overviewTabs ? t('projects.overview_navigation') : t('projects.group_navigation', { section: t(`projects.section.${activeSection}`) })}>
      <div className="workspace-group-tabs" role={overviewTabs ? 'tablist' : undefined}>
        {items.map((item) => (
          <NavLink
            className={({ isActive }) => isActive ? 'is-active' : ''}
            end={item.end}
            key={item.key}
            role={overviewTabs ? 'tab' : undefined}
            id={overviewTabs ? `overview-tab-${item.key}` : undefined}
            aria-controls={overviewTabs ? `overview-panel-${item.key}` : undefined}
            aria-selected={overviewTabs ? location.pathname.endsWith(`/${item.route}`) : undefined}
            to={workspaceRoute(projectUuid, item.route, location.search)}
          >
            {t(item.labelKey)}
          </NavLink>
        ))}
      </div>
    </nav>
  )
}
