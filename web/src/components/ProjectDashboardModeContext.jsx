import { createContext, useCallback, useContext, useMemo, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'

import {
  PROJECT_DASHBOARD_MODE_EXPERT,
  PROJECT_DASHBOARD_MODE_SIMPLE,
  normalizeProjectDashboardMode,
  readProjectDashboardMode,
  writeProjectDashboardMode,
} from '../projectDashboardMode.js'
import {
  projectModeOverride,
  projectRouteRequiresExpert,
  withoutProjectModeOverride,
} from '../projectRoutes.js'

const ProjectDashboardModeContext = createContext(null)

export function ProjectDashboardModeProvider({ children, projectUuid }) {
  const location = useLocation()
  const navigate = useNavigate()
  const [preferredMode, setPreferredMode] = useState(() => readProjectDashboardMode(
    typeof window === 'undefined' ? null : window.localStorage,
    projectUuid,
  ))
  const overrideMode = projectModeOverride(location.search)
  const requiresExpert = projectRouteRequiresExpert(location.pathname, projectUuid)
  const effectiveMode = requiresExpert
    ? PROJECT_DASHBOARD_MODE_EXPERT
    : normalizeProjectDashboardMode(overrideMode || preferredMode)
  const forcedExpert = requiresExpert && preferredMode !== PROJECT_DASHBOARD_MODE_EXPERT

  const selectMode = useCallback((mode) => {
    const normalized = writeProjectDashboardMode(
      typeof window === 'undefined' ? null : window.localStorage,
      projectUuid,
      mode,
    )
    setPreferredMode(normalized)
    if (projectModeOverride(location.search)) {
      navigate({
        pathname: location.pathname,
        search: withoutProjectModeOverride(location.search),
        hash: location.hash,
      }, { replace: true })
    }
    return normalized
  }, [location.hash, location.pathname, location.search, navigate, projectUuid])

  const value = useMemo(() => ({
    effectiveMode,
    forcedExpert,
    preferredMode,
    selectMode,
    simple: effectiveMode === PROJECT_DASHBOARD_MODE_SIMPLE,
  }), [effectiveMode, forcedExpert, preferredMode, selectMode])

  return <ProjectDashboardModeContext.Provider value={value}>{children}</ProjectDashboardModeContext.Provider>
}

export function useProjectDashboardMode() {
  const value = useContext(ProjectDashboardModeContext)
  if (!value) throw new Error('useProjectDashboardMode must be used inside ProjectDashboardModeProvider')
  return value
}
