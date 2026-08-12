import { useSyncExternalStore } from 'react'

import {
  desktopAuthenticationRequired,
  subscribeToDesktopAuthentication,
} from '../desktopSession.js'
import { useI18n } from '../i18n/useI18n.js'

export function DesktopAuthenticationBlock({ expired = false }) {
  const { t } = useI18n()
  return (
    <main className="desktop-session-block" role="alert">
      <section>
        <span>{t('common.desktop_auth.eyebrow')}</span>
        <h1>{t(expired ? 'common.desktop_auth.expired_title' : 'common.desktop_auth.failed_title')}</h1>
        <p>{t(expired ? 'common.desktop_auth.expired_body' : 'common.desktop_auth.failed_body')}</p>
        <strong>{t('common.desktop_auth.reopen')}</strong>
      </section>
    </main>
  )
}

export default function DesktopSessionGate({ children }) {
  const authenticationRequired = useSyncExternalStore(
    subscribeToDesktopAuthentication,
    desktopAuthenticationRequired,
    desktopAuthenticationRequired,
  )
  return authenticationRequired ? <DesktopAuthenticationBlock expired /> : children
}
