import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ClipboardList, Save } from 'lucide-react'

import { getProjectSetup, updateProjectSetupReference } from '../api/projects.js'
import { projectQueryKeys } from '../api/projectQueryKeys.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'

const missingCopy = {
  project_name: 'chat.setup.field.project_name',
  generation_language: 'chat.setup.field.generation_language',
  overall_style: 'chat.setup.field.overall_style',
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

const setupReferenceRoles = ['auto', 'character', 'scene', 'prop', 'style']

function SetupReferenceItem({ projectUuid, revision, reference, editable }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState(() => ({
    reference_role: reference.reference_role,
    title: reference.title,
    instruction: reference.instruction || '',
    include_in_yolo: reference.include_in_yolo,
  }))
  useEffect(() => {
    setDraft({
      reference_role: reference.reference_role,
      title: reference.title,
      instruction: reference.instruction || '',
      include_in_yolo: reference.include_in_yolo,
    })
  }, [reference])
  const mutation = useMutation({
    mutationFn: () => updateProjectSetupReference(projectUuid, reference.uuid, { expected_revision: revision, ...draft }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: projectQueryKeys.setup(projectUuid) }),
    onError: () => queryClient.invalidateQueries({ queryKey: projectQueryKeys.setup(projectUuid) }),
  })
  const dirty = draft.reference_role !== reference.reference_role
    || draft.title !== reference.title
    || draft.instruction !== (reference.instruction || '')
    || draft.include_in_yolo !== reference.include_in_yolo
  const title = draft.title || reference.title
  return (
    <article className="project-setup-reference">
      <img src={reference.thumbnail_url} alt="" loading="lazy" />
      <div className="project-setup-reference__heading">
        <strong>{title}</strong>
        <small data-source={reference.plan_source}>{t(sourceCopy(reference.plan_source))}</small>
      </div>
      {editable ? <>
        <select aria-label={t('chat.setup.reference.role_label', { title })} value={draft.reference_role} disabled={mutation.isPending} onChange={(event) => setDraft((current) => ({ ...current, reference_role: event.target.value }))}>
          {setupReferenceRoles.map((role) => <option value={role} key={role}>{t(`reference.role.${role}`)}</option>)}
        </select>
        <button type="button" className="project-setup-reference__include" aria-pressed={draft.include_in_yolo} disabled={mutation.isPending} onClick={() => setDraft((current) => ({ ...current, include_in_yolo: !current.include_in_yolo }))}>{t(draft.include_in_yolo ? 'chat.setup.reference.included' : 'chat.setup.reference.excluded')}</button>
        <label><span>{t('chat.setup.reference.title')}</span><input value={draft.title} maxLength="160" disabled={mutation.isPending} onChange={(event) => setDraft((current) => ({ ...current, title: event.target.value }))} /></label>
        <label className="project-setup-reference__instruction"><span>{t('chat.setup.reference.instruction')}</span><textarea rows="2" value={draft.instruction} maxLength="2000" disabled={mutation.isPending} onChange={(event) => setDraft((current) => ({ ...current, instruction: event.target.value }))} /></label>
        <button type="button" className="project-setup-reference__save" disabled={!dirty || !draft.title.trim() || mutation.isPending} onClick={() => mutation.mutate()}><Save size={13} aria-hidden="true" />{t(mutation.isPending ? 'chat.setup.reference.saving' : 'chat.setup.reference.save')}</button>
        {mutation.isError ? <LocalizedErrorMessage error={mutation.error} className="chat-error project-setup-reference__error" compact /> : null}
      </> : <div className="project-setup-reference__readonly"><span>{t(`reference.role.${reference.reference_role}`)}</span><span>{t(reference.include_in_yolo ? 'chat.setup.reference.included' : 'chat.setup.reference.excluded')}</span>{reference.instruction ? <p>{reference.instruction}</p> : null}</div>}
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
        <header><div><strong id="project-setup-reference-title">{t('chat.setup.reference.title_section')}</strong><p>{t('chat.setup.reference.hint')}</p></div><span>{setup.reference_plan.items.filter((item) => item.include_in_yolo).length}/{setup.reference_plan.items.length}</span></header>
        <div>{setup.reference_plan.items.map((reference) => <SetupReferenceItem key={reference.uuid} projectUuid={projectUuid} revision={setup.revision} reference={reference} editable />)}</div>
      </section> : null}
      {setup.missing_information?.length ? <div className="project-setup-card__missing"><strong>{t('chat.setup.missing')}</strong><ul>{setup.missing_information.map((field) => <li key={field}>{t(missingCopy[field] || 'chat.setup.field.unknown')}</li>)}</ul></div> : null}
      <footer><span>{t('chat.setup.revision', { revision: setup.revision })}</span><span>{t('chat.setup.directory_hint')}</span></footer>
    </section>
  )
}
