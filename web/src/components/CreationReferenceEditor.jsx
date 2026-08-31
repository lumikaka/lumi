import { useState } from 'react'
import { ChevronDown, Image } from 'lucide-react'

import { useI18n } from '../i18n/useI18n.js'

const referenceRoles = ['auto', 'character', 'scene', 'prop', 'style']

function referenceImageSource(projectUuid, reference) {
  if (reference.previewUrl) return reference.previewUrl
  const fileUuid = reference.fileUuid || reference.resource_uuid || reference.image_file_uuid
  return projectUuid && fileUuid
    ? `/media/projects/${encodeURIComponent(projectUuid)}/assets/${encodeURIComponent(fileUuid)}/content`
    : ''
}

export default function CreationReferenceEditor({ projectUuid = '', references = [], disabled = false, onChange }) {
  const { t } = useI18n()
  const [expanded, setExpanded] = useState(() => new Set())
  if (!references.length) return null

  const toggleExpanded = (key) => {
    setExpanded((current) => {
      const next = new Set(current)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  return (
    <section className="creation-reference-editor" aria-label={t('projects.conversation.reference.plan')}>
      <header>
        <div><strong>{t('projects.conversation.reference.plan')}</strong><p>{t('projects.conversation.reference.promise')}</p></div>
      </header>
      <div className="creation-reference-editor__items">
        {references.map((reference, index) => {
          const key = reference.localId || reference.referenceUuid || `creation-reference-${index + 1}`
          const isExpanded = expanded.has(key)
          const included = reference.includeInYolo !== false
          const imageSource = referenceImageSource(projectUuid, reference)
          return (
            <article className="creation-reference-editor__item" key={key}>
              <span className="creation-reference-editor__preview">{imageSource ? <img src={imageSource} alt="" /> : <Image size={17} aria-hidden="true" />}</span>
              <div className="creation-reference-editor__summary">
                <strong title={reference.filename}>{reference.planTitle || reference.filename}</strong>
                <select aria-label={t('projects.conversation.reference.role_label', { title: reference.planTitle || reference.filename })} value={reference.referenceRole || 'auto'} disabled={disabled} onChange={(event) => onChange(key, { referenceRole: event.target.value })}>
                  {referenceRoles.map((role) => <option value={role} key={role}>{t(`reference.role.${role}`)}</option>)}
                </select>
              </div>
              <button type="button" className="creation-reference-editor__include" aria-pressed={included} disabled={disabled} onClick={() => onChange(key, { includeInYolo: !included })}>{t(included ? 'projects.conversation.reference.included' : 'projects.conversation.reference.excluded')}</button>
              <button type="button" className="creation-reference-editor__expand" aria-expanded={isExpanded} aria-controls={`${key}-reference-fields`} onClick={() => toggleExpanded(key)}>{t('projects.conversation.reference.details')}<ChevronDown size={14} aria-hidden="true" /></button>
              {isExpanded ? <div className="creation-reference-editor__fields" id={`${key}-reference-fields`}>
                <label><span>{t('projects.conversation.reference.title')}</span><input value={reference.planTitle || ''} maxLength="160" disabled={disabled} onChange={(event) => onChange(key, { planTitle: event.target.value })} /></label>
                <label><span>{t('projects.conversation.reference.instruction')}</span><textarea rows="2" value={reference.instruction || ''} maxLength="2000" disabled={disabled} placeholder={t('projects.conversation.reference.instruction_placeholder')} onChange={(event) => onChange(key, { instruction: event.target.value })} /></label>
              </div> : null}
            </article>
          )
        })}
      </div>
      {disabled ? <p className="creation-reference-editor__locked">{t('projects.conversation.reference.locked')}</p> : null}
    </section>
  )
}
