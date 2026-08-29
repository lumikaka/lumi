export function formatProjectEditTime(value, locale, t, now = new Date()) {
  const editedAt = new Date(value || '')
  if (Number.isNaN(editedAt.getTime())) return ''
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const startOfEditedDay = new Date(editedAt.getFullYear(), editedAt.getMonth(), editedAt.getDate())
  const days = Math.max(0, Math.round((startOfToday - startOfEditedDay) / 86_400_000))
  if (days === 0) return t('projects.index.edited_just_now')
  if (days === 1) return t('projects.index.edited_yesterday')
  if (days < 7) return t('projects.index.edited_days_ago', { count: new Intl.NumberFormat(locale).format(days) })
  return t('projects.index.edited_on', { date: new Intl.DateTimeFormat(locale, { month: 'short', day: 'numeric' }).format(editedAt) })
}
