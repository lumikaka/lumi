import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useSearchParams, Link } from 'react-router-dom'
import { Plug } from 'lucide-react'
import { getMCPAuthorization, decideMCPAuthorization } from '../api/mcp.js'
import { ensureProjectOpen, listRecentProjects } from '../api/projects.js'
import { projectQueryKeys } from '../api/projectQueryKeys.js'
import AppPageShell from '../components/AppPageShell.jsx'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'

export default function MCPOAuthPage() {
  const { t, formatDateTime } = useI18n()
  const [params] = useSearchParams()
  const uuid = params.get('request_uuid') || ''
  const [projectUuid, setProjectUuid] = useState('')
  const [permission, setPermission] = useState('read')
  const [completed, setCompleted] = useState(null)
  const request = useQuery({ queryKey: ['mcp-authorization', uuid], queryFn: () => getMCPAuthorization(uuid), enabled: Boolean(uuid), retry: false })
  const projects = useQuery({ queryKey: projectQueryKeys.recent(), queryFn: listRecentProjects })
  const decide = useMutation({
    mutationFn: async (decision) => {
      if (decision === 'approve') await ensureProjectOpen(projectUuid)
      return decideMCPAuthorization(uuid, { decision, project_uuid: decision === 'approve' ? projectUuid : '', permission: decision === 'approve' ? permission : '' })
    },
    onSuccess: (value) => {
      setCompleted(value.redirect_url)
      window.location.assign(value.redirect_url)
    },
  })
  const value = request.data
  const pending = value?.status === 'pending' && !completed
  return <AppPageShell title={t('mcp.oauth.title')}>
    <div className="local-account-page">
      <header className="local-account-heading"><span className="local-account-heading__icon"><Plug size={22} aria-hidden="true" /></span><div><h1>{t('mcp.oauth.title')}</h1><p>{t('mcp.oauth.intro')}</p></div></header>
      <section className="overview-card">
        <LocalizedErrorMessage error={request.error || projects.error || decide.error} />
        {request.isLoading ? <p>{t('mcp.loading')}</p> : null}
        {!uuid ? <p>{t('mcp.oauth.unavailable')}</p> : null}
        {value ? <>
          <h2>{value.client_name}</h2><p>{t('mcp.oauth.client_notice')}</p>
          <p>{t('mcp.oauth.callback')} <code style={{ overflowWrap: 'anywhere' }}>{value.redirect_uri}</code></p>
          <p>{t('mcp.oauth.expiry')} {formatDateTime(value.expires_at, { dateStyle: 'medium', timeStyle: 'short' })}</p>
          {value.offline_access ? <p>{t('mcp.oauth.refresh')}</p> : null}
          {pending ? <form className="overview-edit-form" onSubmit={(event) => { event.preventDefault(); decide.mutate('approve') }}>
            <label>{t('settings.mcp.project')}<select required value={projectUuid} onChange={(event) => setProjectUuid(event.target.value)}><option value="">{t('settings.mcp.select_project')}</option>{(projects.data?.items || []).map((p) => <option value={p.uuid} key={p.uuid}>{p.name}</option>)}</select></label>
            <label>{t('mcp.permission')}<select value={permission} onChange={(event) => setPermission(event.target.value)}><option value="read">{t('mcp.read')}</option><option value="edit">{t('mcp.edit_generate')}</option></select></label>
            <p>{t('mcp.oauth.scope')}</p>
            <div className="overview-form-actions"><button type="button" className="button-secondary" disabled={decide.isPending} onClick={() => decide.mutate('reject')}>{t('mcp.oauth.reject')}</button><button type="submit" disabled={!projectUuid || decide.isPending}>{t('mcp.oauth.allow')}</button></div>
          </form> : <p>{t(completed ? 'mcp.oauth.completed' : 'mcp.oauth.unavailable')}</p>}
          {completed ? <a href={completed} rel="noreferrer">{t('mcp.oauth.return')}</a> : null}
        </> : null}
        <Link to="/settings/mcp">{t('settings.mcp')}</Link>
      </section>
    </div>
  </AppPageShell>
}
