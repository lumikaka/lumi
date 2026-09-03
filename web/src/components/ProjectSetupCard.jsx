import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ClipboardList } from 'lucide-react'

import { getProjectSetup } from '../api/projects.js'
import { projectQueryKeys } from '../api/projectQueryKeys.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'

const missingCopy = {
  project_name: 'chat.setup.field.project_name',
  generation_language: 'chat.setup.field.generation_language',
  overall_style: 'chat.setup.field.overall_style',
  generation_brief: 'chat.setup.field.generation_brief',
  'picture_book.format': 'chat.setup.field.format',
}

function sourceCopy(source) {
  return {
    system_default: 'chat.setup.source.system_default',
    agent_proposed: 'chat.setup.source.agent_proposed',
    user_confirmed: 'chat.setup.source.user_confirmed',
  }[source] || 'chat.setup.source.unknown'
}

function valueOrMissing(value, t) {
  return value === '' || value === null || value === undefined ? t('chat.setup.value.missing') : value
}

function pictureBookFields(draftValues, t) {
  const profile = draftValues?.picture_book
  if (!profile) return [{ key: 'format', sourceKey: 'format', label: t('chat.setup.field.format'), value: '' }]
  const fields = [
    { key: 'format', sourceKey: 'format', label: t('chat.setup.field.format'), value: t(`projects.picture_book.format.${profile.format}`) },
    { key: 'aspect_ratio', sourceKey: 'aspect_ratio', label: t('chat.setup.field.aspect_ratio'), value: `${profile.aspect_ratio?.width || '—'}:${profile.aspect_ratio?.height || '—'}` },
  ]
  if (profile.format === 'classic_picture_book') fields.push({ key: 'large_image_minimal_text', sourceKey: 'large_image_minimal_text', label: t('chat.setup.field.large_image_minimal_text'), value: t(profile.large_image_minimal_text ? 'common.answer.yes' : 'common.answer.no') })
  if (profile.format === 'interactive_picture_book') fields.push({ key: 'interaction_mode', sourceKey: 'interaction_mode', label: t('chat.setup.field.interaction_mode'), value: t(`projects.picture_book.interaction.${profile.interaction_mode}`) })
  if (profile.format === 'comic_story') fields.push({ key: 'comic_layout', sourceKey: 'comic_layout', label: t('chat.setup.field.comic_layout'), value: t(`projects.picture_book.comic_layout.${profile.comic_layout}`) })
  return fields
}

function SetupReferenceItem({ reference }) {
  return (
    <article className="project-setup-reference">
      <img src={reference.thumbnail_url} alt="" loading="lazy" />
      <div className="project-setup-reference__heading">
        <strong>{reference.title}</strong>
      </div>
    </article>
  )
}

export default function ProjectSetupCard({ projectUuid, enabled }) {
  const { t } = useI18n()
  const [activeField, setActiveField] = useState('')
  const setupQuery = useQuery({
    queryKey: projectQueryKeys.setup(projectUuid),
    queryFn: () => getProjectSetup(projectUuid),
    enabled: Boolean(enabled && projectUuid),
  })
  const setup = setupQuery.data
  if (setupQuery.isError) return <LocalizedErrorMessage error={setupQuery.error} className="chat-error project-setup-card__error" />
  if (!setup || setup.setup_status !== 'draft') return null

  const draftValues = setup.draft_values || {}
  const fields = [
    { key: 'project_name', sourceKey: 'project_name', label: t('chat.setup.field.project_name'), value: draftValues.project_name },
    { key: 'generation_language', sourceKey: 'generation_language', label: t('chat.setup.field.generation_language'), value: draftValues.generation_language ? t(`common.language.${draftValues.generation_language === 'zh-Hans' ? 'zh_hans' : 'en'}`) : '' },
    { key: 'overall_style', sourceKey: 'overall_style', label: t('chat.setup.field.overall_style'), value: draftValues.overall_style },
    { key: 'generation_brief', sourceKey: 'generation_brief', label: t('chat.setup.field.generation_brief'), value: draftValues.generation_brief },
    ...pictureBookFields(draftValues, t),
  ]
  const statusKey = setup.status
  return (
    <section className={`project-setup-card project-setup-card--${statusKey}`} aria-labelledby="project-setup-card-title" data-project-setup-status={setup.setup_status}>
      <header>
        <div className="project-setup-card__title"><span><ClipboardList size={17} aria-hidden="true" /></span><div><p>{t('chat.setup.eyebrow')}</p><h3 id="project-setup-card-title">{t('chat.setup.title')}</h3></div></div>
        <span className="project-setup-card__status">{t(`chat.setup.status.${statusKey}`)}</span>
      </header>
      <p className="project-setup-card__hint">{t('chat.setup.draft_hint')}</p>
      <div className="project-setup-card__fields">
        {fields.map((field) => {
          const source = setup.field_sources?.[field.sourceKey] || ''
          return <button key={field.key} type="button" className="project-setup-card__field" aria-pressed={activeField === field.key} onClick={() => setActiveField((current) => current === field.key ? '' : field.key)}><span>{field.label}</span><strong>{valueOrMissing(field.value, t)}</strong><small data-source={source}>{t(sourceCopy(source))}</small></button>
        })}
      </div>
      {setup.reference_plan?.items?.length ? <section className="project-setup-card__references" aria-labelledby="project-setup-reference-title">
        <header><div><strong id="project-setup-reference-title">{t('chat.setup.reference.title_section')}</strong><p>{t('chat.setup.reference.auto_hint', { count: setup.reference_plan.items.length })}</p></div></header>
        <div>{setup.reference_plan.items.map((reference) => <SetupReferenceItem key={reference.uuid} reference={reference} />)}</div>
      </section> : null}
      {setup.missing_information?.length ? <div className="project-setup-card__missing"><strong>{t('chat.setup.missing')}</strong><ul>{setup.missing_information.map((field) => <li key={field}>{t(missingCopy[field] || 'chat.setup.field.unknown')}</li>)}</ul></div> : null}
      <footer><span>{t('chat.setup.revision', { revision: setup.revision })}</span><span>{t('chat.setup.directory_hint')}</span></footer>
    </section>
  )
}
