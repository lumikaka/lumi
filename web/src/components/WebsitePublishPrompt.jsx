import { useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Check, Copy } from 'lucide-react'
import { listRecentProjects } from '../api/projects.js'
import { projectQueryKeys } from '../api/projectQueryKeys.js'
import { useI18n } from '../i18n/useI18n.js'

export default function WebsitePublishPrompt({ projectUuid, available, hasUpdates }) {
  const { t } = useI18n()
  const textarea = useRef(null)
  const [copy, setCopy] = useState(null)
  // Reuse the workspace's project directory query; no new endpoint or polling.
  const projects = useQuery({ queryKey: projectQueryKeys.recent(), queryFn: listRecentProjects })
  const root = projects.data?.items?.find((project) => project.uuid === projectUuid)?.root_path
  const source = root
    ? t('website.publish_prompt_source_path', { path: JSON.stringify(`${root.replace(/[\\/]+$/, '')}/website`) })
    : t('website.publish_prompt_source_upload')
  const prompt = t('website.publish_prompt_body', { source, token: '{site_url}' })
  const copied = copy?.text === prompt && copy.status === 'copied'
  const copying = copy?.text === prompt && copy.status === 'copying'
  const failed = copy?.text === prompt && copy.status === 'failed'
  async function copyPrompt() {
    setCopy({ text: prompt, status: 'copying' })
    try {
      await navigator.clipboard.writeText(prompt)
      setCopy({ text: prompt, status: 'copied' })
    } catch {
      textarea.current?.focus()
      textarea.current?.select()
      setCopy({ text: prompt, status: 'failed' })
    }
  }
  return (
    <section className="project-website__publish" aria-labelledby="website-publish-title">
      <div className="project-website__publish-header">
        <h3 id="website-publish-title">{t('website.publish_prompt_title')}</h3>
        <button type="button" className="button-secondary" onClick={copyPrompt} disabled={copying}>
          {copied ? <Check size={16} aria-hidden="true" /> : <Copy size={16} aria-hidden="true" />}
          {t(copied ? 'website.publish_prompt_copied' : 'website.publish_prompt_copy')}
        </button>
      </div>
      <p id="website-publish-hint">{t(!available ? 'website.publish_prompt_generate_first' : hasUpdates ? 'website.publish_prompt_update_first' : 'website.publish_prompt_hint')}</p>
      <textarea ref={textarea} readOnly rows={12} value={prompt} aria-labelledby="website-publish-title" aria-describedby="website-publish-hint" spellCheck={false} />
      <p className="project-website__copy-status" role="status" aria-live="polite">{failed ? t('website.publish_prompt_copy_failed') : copied ? t('website.publish_prompt_copy_success') : ''}</p>
    </section>
  )
}
