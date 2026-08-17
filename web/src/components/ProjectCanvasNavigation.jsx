import { NavLink, useLocation } from 'react-router-dom'

import chaptersIcon from '../assets/figma/workspace/canvas-tab-chapters.svg'
import premiseIcon from '../assets/figma/workspace/canvas-tab-premise.svg'
import worksIcon from '../assets/figma/workspace/canvas-tab-works.svg'
import { useI18n } from '../i18n/useI18n.js'
import FigmaIcon from './FigmaIcon.jsx'
import { canvasModeForPath, workspaceRoute } from './workspaceNavigation.js'

const CANVAS_MODES = [
  { key: 'premise', labelKey: 'projects.canvas.premise', route: 'premise', icon: premiseIcon },
  { key: 'chapters', labelKey: 'projects.section.chapters', route: 'chapters', icon: chaptersIcon },
  { key: 'works', labelKey: 'projects.canvas.works', route: 'comic', icon: worksIcon },
]

export default function ProjectCanvasNavigation({ projectUuid }) {
  const { t } = useI18n()
  const location = useLocation()
  const activeMode = canvasModeForPath(location.pathname)

  return (
    <nav className="project-canvas-nav" aria-label={t('projects.canvas.navigation')}>
      {CANVAS_MODES.map((mode) => (
        <NavLink
          className={activeMode === mode.key ? 'is-active' : ''}
          key={mode.key}
          to={workspaceRoute(projectUuid, mode.route, location.search)}
          aria-current={activeMode === mode.key ? 'page' : undefined}
        >
          {activeMode === mode.key ? <FigmaIcon src={mode.icon} size={20} /> : null}
          <span>{t(mode.labelKey)}</span>
        </NavLink>
      ))}
    </nav>
  )
}

export { CANVAS_MODES }
