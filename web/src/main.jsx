import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter } from 'react-router-dom'

import App from './App.jsx'
import DesktopSessionGate, { DesktopAuthenticationBlock } from './components/DesktopSessionGate.jsx'
import { bootstrapDesktopSession } from './desktopSession.js'
import { I18nProvider } from './i18n/I18nProvider.jsx'
import './styles/workspaces.sass'
import './styles/common.sass'
import './styles/shell.sass'
import './styles/projects.sass'
import './styles/settings.sass'
import './styles/chat.sass'
import './styles/trajectory.sass'
import './styles/desktop-session.sass'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, staleTime: 30_000 },
  },
})

async function mount() {
  const bootstrap = await bootstrapDesktopSession()
  const root = createRoot(document.getElementById('root'))
  if (!bootstrap.ok) {
    root.render(<I18nProvider><DesktopAuthenticationBlock /></I18nProvider>)
    return
  }
  root.render(
    <StrictMode>
      <I18nProvider>
        <DesktopSessionGate>
          <QueryClientProvider client={queryClient}>
            <BrowserRouter>
              <App />
            </BrowserRouter>
          </QueryClientProvider>
        </DesktopSessionGate>
      </I18nProvider>
    </StrictMode>,
  )
}

void mount()
