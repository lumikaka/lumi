export const SUPPORTED_LOCALES = Object.freeze(['zh-Hans', 'en'])

const INTL_LOCALES = Object.freeze({
  'zh-Hans': 'zh-CN',
  en: 'en-US',
})

export function normalizeLocale(locale) {
  if (!locale || typeof locale !== 'string') return 'zh-Hans'
  const normalized = locale.trim().replace('_', '-').toLowerCase()
  if (normalized === 'en' || normalized.startsWith('en-')) return 'en'
  if (normalized === 'zh' || normalized.startsWith('zh-')) return 'zh-Hans'
  return 'zh-Hans'
}

export function translateMessage(resources, locale, key, values) {
  const normalized = normalizeLocale(locale)
  const template = resources[normalized]?.[key]
  if (typeof template !== 'string') return key
  return interpolate(template, values)
}

export function formatDateTimeForLocale(locale, value, options = {}) {
  if (!value) return '—'
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return new Intl.DateTimeFormat(INTL_LOCALES[normalizeLocale(locale)], options).format(date)
}

export function formatNumberForLocale(locale, value, options = {}) {
  return new Intl.NumberFormat(INTL_LOCALES[normalizeLocale(locale)], options).format(Number(value) || 0)
}

export function formatCountForLocale(resources, locale, key, count, values = {}) {
  const normalized = normalizeLocale(locale)
  const plural = new Intl.PluralRules(INTL_LOCALES[normalized]).select(Number(count) || 0)
  const candidate = `${key}.${plural}`
  const fallback = `${key}.other`
  const selected = Object.prototype.hasOwnProperty.call(resources[normalized] || {}, candidate) ? candidate : fallback
  return translateMessage(resources, normalized, selected, { ...values, count })
}

export function interpolationNames(template) {
  return [...String(template).matchAll(/\{([a-zA-Z0-9_]+)\}/g)].map((match) => match[1]).sort()
}

export function readLocalePreference(storage) {
  try {
    return normalizeLocale(storage?.getItem('lumi.locale'))
  } catch {
    return 'zh-Hans'
  }
}

export function writeLocalePreference(storage, locale) {
  const normalized = normalizeLocale(locale)
  try {
    storage?.setItem('lumi.locale', normalized)
  } catch {
    // A restricted browser can still keep the selected locale in React state.
  }
  return normalized
}

function interpolate(template, values) {
  if (!values) return template
  return Object.entries(values).reduce(
    (message, [name, value]) => message.replaceAll(`{${name}}`, value ?? ''),
    template,
  )
}
