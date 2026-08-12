import { useEffect, useState } from 'react'
import { Languages, UserRound } from 'lucide-react'

import AppPageShell from '../components/AppPageShell.jsx'
import LocalAccountSettingsNav from '../components/LocalAccountSettingsNav.jsx'
import { useI18n } from '../i18n/useI18n.js'

export default function AccountSettingsPage() {
  const { languageOptions, locale, setLocale, t } = useI18n()
  const [selectedLocale, setSelectedLocale] = useState(locale)
  const [saved, setSaved] = useState(false)

  useEffect(() => setSelectedLocale(locale), [locale])

  const submitLanguage = (event) => {
    event.preventDefault()
    setLocale(selectedLocale)
    setSaved(true)
  }

  return (
    <AppPageShell title={t('settings.account')}>
      <div className="local-account-page">
        <header className="local-account-heading">
          <span className="local-account-heading__icon"><UserRound size={22} aria-hidden="true" /></span>
          <div>
            <p className="eyebrow">{t('settings.account.eyebrow')}</p>
            <h1>{t('settings.account.heading')}</h1>
            <p>{t('settings.account.description')}</p>
          </div>
        </header>

        <div className="local-account-layout">
          <LocalAccountSettingsNav />
          <section className="local-account-panel" id="language">
            <header>
              <Languages size={20} aria-hidden="true" />
              <div><h2>{t('settings.language')}</h2><p>{t('settings.language.project_note')}</p></div>
            </header>
            <form className="local-account-language-form" onSubmit={submitLanguage}>
              <label>
                <span>{t('settings.language.label')}</span>
                <select
                  value={selectedLocale}
                  onChange={(event) => { setSelectedLocale(event.target.value); setSaved(false) }}
                >
                  {languageOptions.map((option) => <option key={option.value} value={option.value} data-no-i18n>{option.nativeLabel}</option>)}
                </select>
              </label>
              {saved && selectedLocale === locale ? <p className="local-account-success" role="status">{t('settings.language.saved')}</p> : null}
              <button type="submit" disabled={selectedLocale === locale}>{t('settings.language.save')}</button>
            </form>
          </section>
        </div>
      </div>
    </AppPageShell>
  )
}
