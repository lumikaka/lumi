import { useCallback, useEffect, useMemo, useState } from 'react'

import { DEFAULT_LOCALE, LANGUAGE_OPTIONS, RESOURCES } from './messages/index.js'
import {
  formatCountForLocale,
  formatDateTimeForLocale,
  formatNumberForLocale,
  readLocalePreference,
  translateMessage,
  writeLocalePreference,
} from './runtime.js'
import { I18nContext } from './useI18n.js'

export { normalizeLocale } from './runtime.js'
export { useI18n } from './useI18n.js'

export function I18nProvider({ children }) {
  const [locale, setLocaleState] = useState(readInitialLocale)

  const setLocale = useCallback((nextLocale) => {
    const normalized = writeLocalePreference(window.localStorage, nextLocale)
    setLocaleState(normalized)
  }, [])

  const t = useCallback(
    (key, values) => translateMessage(RESOURCES, locale, key, values),
    [locale],
  )
  const formatDateTime = useCallback(
    (value, options) => formatDateTimeForLocale(locale, value, options),
    [locale],
  )
  const formatNumber = useCallback(
    (value, options) => formatNumberForLocale(locale, value, options),
    [locale],
  )
  const formatCount = useCallback(
    (key, count, values) => formatCountForLocale(RESOURCES, locale, key, count, values),
    [locale],
  )

  useEffect(() => {
    document.documentElement.lang = locale
    document.title = t('common.meta.title')
    const description = document.querySelector('meta[name="description"]')
    if (description) description.setAttribute('content', t('common.meta.description'))
  }, [locale, t])

  const value = useMemo(() => ({
    locale,
    setLocale,
    t,
    formatDateTime,
    formatNumber,
    formatCount,
    languageOptions: LANGUAGE_OPTIONS,
  }), [formatCount, formatDateTime, formatNumber, locale, setLocale, t])

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}

function readInitialLocale() {
  if (typeof window === 'undefined') return DEFAULT_LOCALE
  return readLocalePreference(window.localStorage)
}
