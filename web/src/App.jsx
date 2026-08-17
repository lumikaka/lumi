import { Route, Routes, useLocation } from 'react-router-dom'

import HomePage from './pages/HomePage.jsx'
import AboutPage from './pages/AboutPage.jsx'
import AccountSettingsPage from './pages/AccountSettingsPage.jsx'
import LLMLogsPage from './pages/LLMLogsPage.jsx'
import NotFoundPage from './pages/NotFoundPage.jsx'
import ProviderSettingsPage from './pages/ProviderSettingsPage.jsx'
import StoryWorkspacePage from './pages/StoryWorkspacePage.jsx'
import ChatComposerAcceptancePage from './pages/ChatComposerAcceptancePage.jsx'
import AppFrame from './components/AppFrame.jsx'
import ProjectActivationGate from './components/ProjectActivationGate.jsx'
import { useSiteSettingsRealtime } from './realtime/useSiteSettingsRealtime.js'

export default function App() {
  useSiteSettingsRealtime()
  const location = useLocation()
  if (location.pathname === '/acceptance/chat-composer') {
    return <Routes><Route path="/acceptance/chat-composer" element={<ChatComposerAcceptancePage />} /></Routes>
  }
  return (
    <AppFrame>
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route path="/about" element={<AboutPage />} />
        <Route path="/setup/*" element={<ProviderSettingsPage onboarding />} />
        <Route path="/settings/providers" element={<ProviderSettingsPage />} />
        <Route path="/settings/account" element={<AccountSettingsPage />} />
        <Route path="/settings/llm-logs" element={<LLMLogsPage />} />
        <Route path="/projects/:projectUuid/*" element={<ProjectActivationGate><StoryWorkspacePage /></ProjectActivationGate>} />
        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </AppFrame>
  )
}
