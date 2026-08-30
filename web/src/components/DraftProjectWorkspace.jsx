import { useI18n } from '../i18n/useI18n.js'

export default function DraftProjectWorkspace() {
  const { t } = useI18n()

  return (
    <section className="draft-project-workspace">
      <span aria-hidden="true">✦</span>
      <p className="eyebrow">{t('projects.draft.eyebrow')}</p>
      <h1>{t('projects.draft.title')}</h1>
      <p>{t('projects.draft.body')}</p>
      <small>{t('projects.draft.directory_hint')}</small>
    </section>
  )
}
