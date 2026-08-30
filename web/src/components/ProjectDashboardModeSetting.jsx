import { useI18n } from '../i18n/useI18n.js'
import {
  PROJECT_DASHBOARD_MODE_EXPERT,
  PROJECT_DASHBOARD_MODE_SIMPLE,
} from '../projectDashboardMode.js'
import { useProjectDashboardMode } from './ProjectDashboardModeContext.jsx'

const MODES = Object.freeze([
  {
    value: PROJECT_DASHBOARD_MODE_SIMPLE,
    labelKey: 'simple.mode.simple',
    descriptionKey: 'projects.dashboard_mode.simple_body',
  },
  {
    value: PROJECT_DASHBOARD_MODE_EXPERT,
    labelKey: 'simple.mode.expert',
    descriptionKey: 'projects.dashboard_mode.expert_body',
  },
])

export default function ProjectDashboardModeSetting({ disabled = false, dirty = false }) {
  const { t } = useI18n()
  const { preferredMode, selectMode: updateMode } = useProjectDashboardMode()

  const selectMode = (mode) => {
    if (mode === preferredMode) return
    if (dirty && !window.confirm(t('projects.dashboard_mode.unsaved_warning'))) return
    updateMode(mode)
  }

  return (
    <fieldset className="project-dashboard-mode-setting">
      <legend>{t('projects.dashboard_mode.title')}</legend>
      <p>{t('projects.dashboard_mode.body')}</p>
      <div className="project-dashboard-mode-setting__options">
        {MODES.map((mode) => (
          <button
            key={mode.value}
            type="button"
            aria-pressed={preferredMode === mode.value}
            disabled={disabled}
            onClick={() => selectMode(mode.value)}
          >
            <strong>{t(mode.labelKey)}</strong>
            <span>{t(mode.descriptionKey)}</span>
          </button>
        ))}
      </div>
    </fieldset>
  )
}
