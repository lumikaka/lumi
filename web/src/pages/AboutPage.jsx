import HealthCard from '../components/HealthCard.jsx'
import AppPageShell from '../components/AppPageShell.jsx'
import { useI18n } from '../i18n/useI18n.js'

export default function AboutPage() {
  const { t } = useI18n()
  return (
    <AppPageShell title={t('common.about.title')}>
      <div className="content-page">
        <p className="eyebrow">{t('common.about.eyebrow')}</p>
        <h1>{t('common.about.heading')}</h1>
        <p className="content-page__intro">{t('common.about.intro')}</p>
        <div className="principle-list">
          <article><b>01</b><div><h2>{t('common.about.entry.title')}</h2><p>{t('common.about.entry.body')}</p></div></article>
          <article><b>02</b><div><h2>{t('common.about.data.title')}</h2><p>{t('common.about.data.body')}</p></div></article>
          <article><b>03</b><div><h2>{t('common.about.boundary.title')}</h2><p>{t('common.about.boundary.body')}</p></div></article>
        </div>
        <HealthCard compact />
      </div>
    </AppPageShell>
  )
}
