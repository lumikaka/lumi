import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Globe, ExternalLink, FolderOpen } from 'lucide-react'
import { cancelProductionTask, retryProductionTask, getProjectWebsite, generateProjectWebsite, openProjectWebsiteDirectory } from '../api/production.js'
import { useI18n } from '../i18n/useI18n.js'
import { statusLabel } from '../i18n/labels.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import WebsitePublishPrompt from './WebsitePublishPrompt.jsx'
import '../styles/project-website.sass'

export default function ProjectWebsiteCard({ projectUuid }) {
  const { t, formatDateTime } = useI18n()
  const client = useQueryClient()
  const [allowedMissing, setAllowedMissing] = useState(null)
  const query = useQuery({ queryKey: ['project-website', projectUuid], queryFn: () => getProjectWebsite(projectUuid) })
  const website = query.data
  const latest = website?.latest
  const readiness = website?.readiness
  const missing = readiness?.missing_sections || []
  const missingKey = JSON.stringify(missing.map((item) => item.uuid))
  const consent = allowedMissing === missingKey
  const active = ['queued', 'running'].includes(latest?.status)
  const retryable = ['failed', 'cancelled', 'interrupted'].includes(latest?.status)
  const invalidate = () => {
    client.invalidateQueries({ queryKey: ['project-website', projectUuid] })
    client.invalidateQueries({ queryKey: ['production-tasks', projectUuid] })
  }
  const generate = useMutation({
    mutationFn: () => generateProjectWebsite(projectUuid, { allow_missing_images: missing.length > 0 && consent, idempotency_key: crypto.randomUUID() }),
    onSettled: invalidate,
  })
  const cancel = useMutation({ mutationFn: () => cancelProductionTask(projectUuid, latest.task_uuid), onSettled: invalidate })
  const retry = useMutation({ mutationFn: () => retryProductionTask(projectUuid, latest.task_uuid), onSettled: invalidate })
  const open = useMutation({ mutationFn: () => openProjectWebsiteDirectory(projectUuid) })
  const pending = generate.isPending || cancel.isPending || retry.isPending
  return (
    <section className="overview-card project-website" aria-labelledby="website-title">
      <header className="overview-card__header">
        <div><h2 id="website-title"><Globe size={21} aria-hidden="true" /> {t('website.title')}</h2><p>{t('website.description')}</p></div>
        {website ? <span className="project-website__badge">{t(website.has_updates ? 'website.changed' : 'website.current')}</span> : null}
      </header>
      <LocalizedErrorMessage error={query.error || generate.error || cancel.error || retry.error || open.error} />
      <p>{t('website.last_success')} {website?.last_successful_at ? <time dateTime={website.last_successful_at}>{formatDateTime(website.last_successful_at)}</time> : t('website.never')}</p>
      {latest ? <div className="project-website__status" role="status" aria-live="polite">
        <span>{statusLabel(t, latest.status)}{active ? ` · ${latest.progress}%` : ''}</span>
        {active ? <progress max="100" value={latest.progress} aria-label={t('website.progress')} /> : null}
        {latest.status === 'completed' ? <span>{t('website.image_counts', { processed: latest.processed_images, reused: latest.reused_images })}</span> : null}
        {retryable ? <span>{t('website.failed_hint')}{latest.error_code ? ` (${latest.error_code})` : ''}</span> : null}
      </div> : null}
      {missing.length > 0 ? <div className="project-website__missing">
        <details><summary>{t('website.missing', { count: missing.length })}</summary><ul>{missing.map((item) => <li key={item.uuid}>{item.title || t('website.missing_page', { number: item.section_no })}</li>)}</ul></details>
        <label><input type="checkbox" checked={consent} disabled={active || pending} onChange={(event) => setAllowedMissing(event.target.checked ? missingKey : null)} />{t('website.allow_missing')}</label>
      </div> : null}
      {readiness && !readiness.can_export ? <p>{t('website.empty')}</p> : null}
      <div className="project-website__actions">
        <button type="button" disabled={active || pending || !readiness?.can_export || (missing.length > 0 && !consent)} onClick={() => generate.mutate()}>{t(website?.last_successful_at ? 'website.update' : 'website.generate')}</button>
        {active ? <button type="button" className="button-secondary" disabled={pending} onClick={() => cancel.mutate()}>{t('common.action.cancel')}</button> : null}
        {retryable ? <button type="button" className="button-secondary" disabled={pending} onClick={() => retry.mutate()}>{t('website.retry')}</button> : null}
        {website?.available ? <><a className="button-secondary" href={website.preview_url} target="_blank" rel="noopener noreferrer"><ExternalLink size={16} aria-hidden="true" />{t('website.preview')}</a><button type="button" className="button-secondary" disabled={open.isPending} onClick={() => open.mutate()}><FolderOpen size={16} aria-hidden="true" />{t('website.open_directory')}</button></> : null}
      </div>
      <p className="project-website__note">{t('website.managed_note')}</p>
      <p className="project-website__note">{t('website.rss_note', { token: '{site_url}' })}</p>
      <p className="project-website__note">{t('website.publish_note')}</p>
      <WebsitePublishPrompt projectUuid={projectUuid} available={website?.available} hasUpdates={website?.has_updates} />
    </section>
  )
}
