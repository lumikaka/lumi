import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'

import { listRecentProjects } from '../api/projects.js'
import { projectQueryKeys } from '../api/projectQueryKeys.js'
import GlobalSidebar, { useGlobalSidebarState } from './GlobalSidebar.jsx'
import GlobalTopbar from './GlobalTopbar.jsx'

export default function AppPageShell({ title, subtitle, actions, children, className = '', showAccount = true }) {
  const navigate = useNavigate()
  const [sidebarCollapsed, setSidebarCollapsed] = useGlobalSidebarState()
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const recentQuery = useQuery({ queryKey: projectQueryKeys.recent(), queryFn: listRecentProjects })
  const switchProject = (uuid) => navigate(`/projects/${encodeURIComponent(uuid)}/overview/summary`)

  return (
    <main className={`app-route-shell ${sidebarCollapsed ? 'global-sidebar-collapsed' : ''} ${className}`.trim()}>
      <GlobalSidebar
        collapsed={sidebarCollapsed}
        mobileOpen={sidebarOpen}
        onClose={() => setSidebarOpen(false)}
        onToggleCollapsed={() => setSidebarCollapsed((value) => !value)}
        recentProjects={recentQuery.data?.items || []}
        onSwitchProject={switchProject}
      />
      <GlobalTopbar title={title} subtitle={subtitle} actions={actions} showAccount={showAccount} onOpenNavigation={() => setSidebarOpen(true)} />
      <div className="app-page-content">{children}</div>
    </main>
  )
}
