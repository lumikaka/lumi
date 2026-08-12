import { Activity, Languages } from 'lucide-react'
import { NavLink } from 'react-router-dom'

import { useI18n } from '../i18n/useI18n.js'

export default function LocalAccountSettingsNav() {
  const { t } = useI18n()
  return (
    <nav className="local-account-nav" aria-label={t('settings.account')}>
      <NavLink to="/settings/account">
        <Languages size={16} aria-hidden="true" />
        <span>{t('settings.language')}</span>
      </NavLink>
      <NavLink to="/settings/llm-logs">
        <Activity size={16} aria-hidden="true" />
        <span>{t('settings.llm_logs')}</span>
      </NavLink>
    </nav>
  )
}
