import { createContext, useContext } from 'react'

import { DEFAULT_LOCALE, LANGUAGE_OPTIONS, RESOURCES } from './messages/index.js'
import { formatCountForLocale, formatDateTimeForLocale, formatNumberForLocale, translateMessage } from './runtime.js'

const defaultValue = Object.freeze({
  locale: DEFAULT_LOCALE,
  setLocale: () => {},
  t: (key, values) => translateMessage(RESOURCES, DEFAULT_LOCALE, key, values),
  formatDateTime: (value, options) => formatDateTimeForLocale(DEFAULT_LOCALE, value, options),
  formatNumber: (value, options) => formatNumberForLocale(DEFAULT_LOCALE, value, options),
  formatCount: (key, count, values) => formatCountForLocale(RESOURCES, DEFAULT_LOCALE, key, count, values),
  languageOptions: LANGUAGE_OPTIONS,
})

export const I18nContext = createContext(defaultValue)

export function useI18n() {
  return useContext(I18nContext)
}
