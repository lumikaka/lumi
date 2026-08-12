import { useQuery } from '@tanstack/react-query'
import { Navigate, Route, Routes, useLocation } from 'react-router-dom'

import HomePage from './pages/HomePage.jsx'
import AboutPage from './pages/AboutPage.jsx'
import AccountSettingsPage from './pages/AccountSettingsPage.jsx'
import LLMLogsPage from './pages/LLMLogsPage.jsx'
import NotFoundPage from './pages/NotFoundPage.jsx'
import ProviderSettingsPage from './pages/ProviderSettingsPage.jsx'
import StoryWorkspacePage from './pages/StoryWorkspacePage.jsx'
import ProjectActivationGate from './components/ProjectActivationGate.jsx'
import { getActiveProvider } from './api/ai.js'
import { useSiteSettingsRealtime } from './realtime/useSiteSettingsRealtime.js'
import { useI18n } from './i18n/useI18n.js'

function ProviderGate({ children }) {
  const { t } = useI18n()
  const location = useLocation()
  const activeQuery = useQuery({ queryKey: ['active-provider'], queryFn: getActiveProvider, retry: false })
  if (activeQuery.isLoading) return <p className="workspace-loading">{t('settings.provider.checking')}</p>
  if (activeQuery.isError || !activeQuery.data?.ready) {
    const from = `${location.pathname}${location.search}${location.hash}`
    return <Navigate replace to="/setup/" state={{ from }} />
  }
  return children
}

export default function App() {
  useSiteSettingsRealtime()
  return (
    <Routes>
      <Route path="/" element={<ProviderGate><HomePage /></ProviderGate>} />
      <Route path="/about" element={<ProviderGate><AboutPage /></ProviderGate>} />
      <Route path="/setup/*" element={<ProviderSettingsPage onboarding />} />
      <Route path="/settings/providers" element={<ProviderSettingsPage />} />
      <Route path="/settings/account" element={<AccountSettingsPage />} />
      <Route path="/settings/llm-logs" element={<LLMLogsPage />} />
      <Route path="/projects/:projectUuid/*" element={<ProviderGate><ProjectActivationGate><StoryWorkspacePage /></ProjectActivationGate></ProviderGate>} />
      <Route path="*" element={<ProviderGate><NotFoundPage /></ProviderGate>} />
    </Routes>
  )
}
