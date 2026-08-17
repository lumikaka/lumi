import { useEffect, useState } from 'react'

import AppSidebar from './AppSidebar.jsx'

const SIDEBAR_COLLAPSED_KEY = 'lumi.appSidebarCollapsed'

export default function AppFrame({ children }) {
  const [collapsed, setCollapsed] = useState(readCollapsed)

  useEffect(() => {
    try { window.localStorage.setItem(SIDEBAR_COLLAPSED_KEY, collapsed ? 'true' : 'false') } catch { /* restricted browser */ }
  }, [collapsed])

  return (
    <div className={`app-frame${collapsed ? ' app-frame--sidebar-collapsed' : ''}`}>
      <AppSidebar collapsed={collapsed} onToggle={() => setCollapsed((value) => !value)} />
      <div className="app-frame__main">{children}</div>
    </div>
  )
}

function readCollapsed() {
  if (typeof window === 'undefined') return false
  try { return window.localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === 'true' } catch { return false }
}
