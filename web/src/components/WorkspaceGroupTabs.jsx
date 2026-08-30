import { NavLink, useLocation } from 'react-router-dom'

import { WORKSPACE_GROUP_ITEMS, workspaceRoute } from './workspaceNavigation.js'
import { useI18n } from '../i18n/useI18n.js'
import { formatTerminologyKey } from '../pages/pictureBookProfile.js'

export default function WorkspaceGroupTabs({ projectUuid, activeSection, pictureBook, hidden = false }) {
  const { t } = useI18n()
  const location = useLocation()
  if (hidden) return null
  if (activeSection === 'premise' && location.pathname.endsWith('/premise')) return null
  if (activeSection === 'chapters' && (location.pathname.endsWith('/chapters') || /\/chapters\/[^/]+(?:\/preview)?$/.test(location.pathname))) return null
  const items = WORKSPACE_GROUP_ITEMS[activeSection] || WORKSPACE_GROUP_ITEMS.overview
  const overviewTabs = activeSection === 'overview'

  return (
    <nav className={`workspace-group-nav ${overviewTabs ? 'workspace-group-nav--overview' : ''}`} aria-label={overviewTabs ? t('projects.overview_navigation') : t('projects.group_navigation', { section: t(`projects.section.${activeSection}`) })}>
      <div className="workspace-group-tabs" role={overviewTabs ? 'tablist' : undefined}>
        {items.map((item) => (
          <NavLink
            className={workspaceItemActive(location, workspaceRoute(projectUuid, item.route, location.search), item) ? 'is-active' : ''}
            end={item.end}
            key={item.key}
            role={overviewTabs ? 'tab' : undefined}
            id={overviewTabs ? `overview-tab-${item.key}` : undefined}
            aria-controls={overviewTabs ? `overview-panel-${item.key}` : undefined}
            aria-selected={overviewTabs ? workspaceItemActive(location, workspaceRoute(projectUuid, item.route, location.search), item) : undefined}
            to={workspaceRoute(projectUuid, item.route, location.search)}
          >
            {t(item.key === 'chapters'
              ? formatTerminologyKey(pictureBook, 'projects.section.picture_books', item.labelKey)
              : item.labelKey)}
          </NavLink>
        ))}
      </div>
    </nav>
  )
}

function workspaceItemActive(location, target, item) {
  if (location.pathname !== target.pathname) return false
  const current = new URLSearchParams(location.search)
  const expected = new URLSearchParams(target.search)
  if (item.key === 'trash') return current.get('state') === 'trashed'
  if (item.key === 'chapters') return current.get('state') !== 'trashed'
  for (const [key, value] of expected) {
    if (current.get(key) !== value) return false
  }
  return true
}
