import { Link } from 'react-router-dom'
import AppPageShell from '../components/AppPageShell.jsx'
import { useI18n } from '../i18n/useI18n.js'

export default function NotFoundPage() {
  const { t } = useI18n()
  return (
    <AppPageShell title={t('common.not_found.page_title')}>
      <section className="not-found">
        <span>404</span>
        <h1>{t('common.not_found.title')}</h1>
        <p>{t('common.not_found.body')}</p>
        <Link className="button-link" to="/">{t('common.not_found.action')}</Link>
      </section>
    </AppPageShell>
  )
}
